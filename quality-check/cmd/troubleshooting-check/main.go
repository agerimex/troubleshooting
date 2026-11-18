// Command troubleshooting-check checks a test run against quality rules kept
// in your repository (quality.yaml) and exits with the gate's result:
//
//	0 passed, 1 failed, 2 unresolved (not enough data), 3 error (no result).
//
// Usage:
//
//	troubleshooting-check --url http://localhost:8095 --rules quality.yaml \
//	    --run pr-42-1 [--baseline main-118] [--since 1h] [--json result.json] [--html report.html] [--markdown summary.md] [--record]
//
//	troubleshooting-check comment --markdown summary.md --repo owner/name --pr 42 [--commit sha]
//
// posts or updates the summary as one pull request comment (comment.go).
//
// The run is the value of the troubleshooting.run attribute on the spans of
// the test run (e.g. OTEL_RESOURCE_ATTRIBUTES=troubleshooting.run=pr-42-1).
// See docs/quality-gate.md.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	exitPassed     = 0
	exitFailed     = 1
	exitUnresolved = 2
	exitError      = 3
)

// Rules mirror the API's quality-gate-v1 rules. Unknown keys are an error,
// so a misspelled limit is never silently ignored.
type Entry struct {
	Service   string `yaml:"service" json:"service"`
	Operation string `yaml:"operation" json:"operation"`
	SpanKind  string `yaml:"span_kind,omitempty" json:"span_kind,omitempty"`
}

type Check struct {
	ID              string   `yaml:"id" json:"id"`
	Type            string   `yaml:"type" json:"type"`
	Entry           *Entry   `yaml:"entry,omitempty" json:"entry,omitempty"`
	Advisory        bool     `yaml:"advisory,omitempty" json:"advisory,omitempty"`
	MinExecutions   int      `yaml:"min_executions,omitempty" json:"min_executions,omitempty"`
	Percentile      float64  `yaml:"percentile,omitempty" json:"percentile,omitempty"`
	MaxMs           *float64 `yaml:"max_ms,omitempty" json:"max_ms,omitempty"`
	TargetMs        *float64 `yaml:"target_ms,omitempty" json:"target_ms,omitempty"`
	MaxPerExecution *int     `yaml:"max_per_execution,omitempty" json:"max_per_execution,omitempty"`
	TargetPer       *int     `yaml:"target_per_execution,omitempty" json:"target_per_execution,omitempty"`
	MinPerExecution *int     `yaml:"min_per_execution,omitempty" json:"min_per_execution,omitempty"`
	From            string   `yaml:"from,omitempty" json:"from,omitempty"`
	To              string   `yaml:"to,omitempty" json:"to,omitempty"`
}

// RunDecl is what the test run declares about itself: run.executions in
// quality.yaml (docs/quality-gate.md).
type RunDecl struct {
	Executions int `yaml:"executions,omitempty" json:"executions,omitempty"`
	// ChildCounts "required": db_calls and forbidden_call can't pass while an
	// execution doesn't record its child counts.
	ChildCounts string `yaml:"child_counts,omitempty" json:"child_counts,omitempty"`
}

// Requirement names an intent, its owner and reason, the checks that assess
// it and the guardrails that support it without deciding its status
// (docs/quality-gate.md, "Requirements").
type Requirement struct {
	ID         string   `yaml:"id" json:"id"`
	Title      string   `yaml:"title" json:"title"`
	Owner      string   `yaml:"owner,omitempty" json:"owner,omitempty"`
	Why        string   `yaml:"why,omitempty" json:"why,omitempty"`
	Checks     []string `yaml:"checks,omitempty" json:"checks,omitempty"`
	Guardrails []string `yaml:"guardrails,omitempty" json:"guardrails,omitempty"`
}

type Rules struct {
	Version      int           `yaml:"version" json:"version"`
	Project      string        `yaml:"project,omitempty" json:"project,omitempty"`
	Run          *RunDecl      `yaml:"run,omitempty" json:"run,omitempty"`
	Entry        *Entry        `yaml:"entry,omitempty" json:"entry,omitempty"`
	Checks       []Check       `yaml:"checks" json:"checks"`
	Requirements []Requirement `yaml:"requirements,omitempty" json:"requirements,omitempty"`
}

// request sends the rules file as text: the API reads it as strictly as
// loadRules, and a recorded evaluation keeps exactly that text.
type request struct {
	RulesYAML    string  `json:"rules_yaml"`
	Record       bool    `json:"record,omitempty"`
	Reevaluate   bool    `json:"reevaluate,omitempty"`
	Source       *Source `json:"source,omitempty"`
	Run          string  `json:"run"`
	TimeFrom     string  `json:"time_from"`
	TimeTo       string  `json:"time_to"`
	Baseline     string  `json:"baseline,omitempty"`
	BaselineFrom string  `json:"baseline_time_from,omitempty"`
	BaselineTo   string  `json:"baseline_time_to,omitempty"`
}

type Evidence struct {
	TraceID  string    `json:"trace_id"`
	SpanID   string    `json:"span_id"`
	TimeFrom string    `json:"time_from"`
	TimeTo   string    `json:"time_to"`
	Note     string    `json:"note"`
	Link     string    `json:"link,omitempty"`
	Snapshot *Snapshot `json:"snapshot,omitempty"` // the evidence trace's spans when checked (snapshot.go)
}

type Baseline struct {
	Status     string   `json:"status"`
	Value      *float64 `json:"value"`
	Executions int      `json:"executions"`
	Summary    string   `json:"summary"`
}

type CheckResult struct {
	ID           string     `json:"id"`
	Type         string     `json:"type"`
	Entry        Entry      `json:"entry"`
	Advisory     bool       `json:"advisory"`
	Status       string     `json:"status"`
	Reason       string     `json:"reason"`
	Summary      string     `json:"summary"`
	Unit         string     `json:"unit"`
	Value        *float64   `json:"value"`
	Limit        *float64   `json:"limit"`
	Target       *float64   `json:"target"`
	TargetStatus string     `json:"target_status,omitempty"`
	Executions   int        `json:"executions"`
	Violations   int        `json:"violations"`
	Details      []string   `json:"details"`
	Evidence     []Evidence `json:"evidence"`
	Baseline     *Baseline  `json:"baseline,omitempty"`
}

type Evaluation struct {
	Contract     string        `json:"contract"`
	Method       string        `json:"method"`
	Run          string        `json:"run"`
	TimeFrom     string        `json:"time_from"`
	TimeTo       string        `json:"time_to"`
	Baseline     string        `json:"baseline,omitempty"`
	BaselineFrom string        `json:"baseline_time_from,omitempty"`
	BaselineTo   string        `json:"baseline_time_to,omitempty"`
	Gate         string        `json:"gate"`
	Checks       []CheckResult `json:"checks"`
	EvaluatedAt  string        `json:"evaluated_at"`
	Project      string        `json:"project,omitempty"`
	// Requirements: the rules' requirements with a status from their checks.
	Requirements []RequirementResult `json:"requirements,omitempty"`
	// ID and RecordedAt: set when the API recorded the evaluation (--record).
	ID          string `json:"id,omitempty"`
	RecordedAt  string `json:"recorded_at,omitempty"`
	RecordError string `json:"record_error,omitempty"`
	// RecordedBefore: the same evaluation was recorded before, and the API
	// answered with that record; Reevaluated: a new record of inputs recorded
	// before (--reevaluate).
	RecordedBefore bool `json:"recorded_before,omitempty"`
	Reevaluated    bool `json:"reevaluated,omitempty"`
	// RecordUnconfirmed: sent to the history under ID, but the API couldn't
	// read the stored record back; the verdict shown is the local one.
	RecordUnconfirmed bool `json:"record_unconfirmed,omitempty"`
	// WindowFromRun: the window is the run's own, found from its spans (the
	// request left it out); BaselineFromRun the same for the baseline.
	WindowFromRun   bool `json:"window_from_run,omitempty"`
	BaselineFromRun bool `json:"baseline_window_from_run,omitempty"`
	// Assumptions say what the verdicts rest on (which lost spans can be seen).
	Assumptions []string `json:"assumptions,omitempty"`
	// ChildCounts: in how many checked executions lost child spans could be seen.
	ChildCounts *ChildCoverage `json:"child_counts,omitempty"`
	// Source is added by the CLI (the API doesn't know it): what was tested
	// and which rules decided, so a result can't be mistaken for one of
	// another commit or of other rules.
	Source *Source `json:"source,omitempty"`
}

// ChildCoverage is the API's child_counts.
type ChildCoverage struct {
	Executions int    `json:"executions"`
	Counted    int    `json:"counted"`
	Status     string `json:"status"`
}

// guardrailDefinition is printed once wherever guardrails appear: readers
// meet the word first in a result.
const guardrailDefinition = "(A guardrail is a check listed with a requirement: it decides the gate like every check, but never the requirement's status.)"

func hasGuardrails(reqs []RequirementResult) bool {
	for _, r := range reqs {
		if len(r.Guardrails) > 0 || r.GuardrailSummary != "" {
			return true
		}
	}
	return false
}

type RequirementResult struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Owner   string   `json:"owner,omitempty"`
	Why     string   `json:"why,omitempty"`
	Checks  []string `json:"checks"`
	Status  string   `json:"status"`
	Summary string   `json:"summary"`
	// Guardrails support the requirement; they decide the gate, never its status.
	Guardrails       []string `json:"guardrails,omitempty"`
	GuardrailSummary string   `json:"guardrail_summary,omitempty"`
	Criteria         string   `json:"criteria_sha256,omitempty"`
}

type Source struct {
	Commit      string `json:"commit,omitempty"`    // the tested commit: what was checked out (a PR's merge commit by default)
	PRHead      string `json:"pr_head,omitempty"`   // the pull request's head commit, if the tested commit merges it
	Ref         string `json:"ref,omitempty"`       // its branch or tag
	CIRun       string `json:"ci_run,omitempty"`    // link to the CI run
	Rules       string `json:"rules"`               // the rules file as given
	RulesRef    string `json:"rules_ref,omitempty"` // where the rules were read from, e.g. "origin/main:quality.yaml"
	RulesSHA256 string `json:"rules_sha256"`        // of the file's bytes
}

type options struct {
	url, ui, rules, run, baseline, jsonPath, htmlPath, mdPath string
	from, to, baselineFrom, baselineTo                        string
	since, baselineSince                                      time.Duration
	timeout                                                   time.Duration
	basicAuth                                                 string
	commit, prHead, ref, ciRun, rulesRef                      string
	noSnapshot, record, reevaluate                            bool
	// windowSet: --from, --to or --since was given. Without them the API
	// takes the run's own window (its first to its last entry span).
	windowSet bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, time.Now))
}

func run(args []string, stdout, stderr io.Writer, now func() time.Time) int {
	if len(args) > 0 && args[0] == "privacy" {
		return runPrivacy(args[1:], stdout, stderr, now)
	}
	if len(args) > 0 && args[0] == "comment" {
		return runComment(args[1:], stdout, stderr)
	}
	fs := flag.NewFlagSet("troubleshooting-check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o options
	fs.StringVar(&o.url, "url", envOr("TROUBLESHOOTING_URL", "http://localhost:8095"), "Troubleshooting URL (the UI origin, which serves /api; or the API itself)")
	fs.StringVar(&o.ui, "ui", "", "UI URL for evidence links (default: --url); \"none\" for no links, e.g. when the Troubleshooting is gone after the CI job")
	fs.StringVar(&o.rules, "rules", "quality.yaml", "rules file (YAML or JSON)")
	fs.StringVar(&o.run, "run", "", "run to check: the value of troubleshooting.run on its spans (required)")
	fs.StringVar(&o.baseline, "baseline", "", "run to compare with (shown next to each value; never changes the result)")
	fs.DurationVar(&o.since, "since", time.Hour, "window: executions that started this long before now (default: the run's own window, found from its spans)")
	fs.StringVar(&o.from, "from", "", "window start, RFC 3339 (default: the run's own window)")
	fs.StringVar(&o.to, "to", "", "window end, RFC 3339 (default now)")
	fs.DurationVar(&o.baselineSince, "baseline-since", 0, "baseline window (default: the run's window)")
	fs.StringVar(&o.baselineFrom, "baseline-from", "", "baseline window start, RFC 3339")
	fs.StringVar(&o.baselineTo, "baseline-to", "", "baseline window end, RFC 3339 (default now)")
	fs.StringVar(&o.jsonPath, "json", "", "write the evaluation as JSON to this file")
	fs.StringVar(&o.htmlPath, "html", "", "write a self-contained HTML report to this file")
	fs.StringVar(&o.mdPath, "markdown", "", "write a Markdown summary (for a CI job summary or a PR comment) to this file")
	fs.DurationVar(&o.timeout, "timeout", 2*time.Minute, "request timeout")
	fs.StringVar(&o.commit, "commit", "", "the tested commit (what was checked out), recorded in the result")
	fs.StringVar(&o.prHead, "pr-head", "", "the pull request's head commit, recorded in the result if it differs from --commit")
	fs.StringVar(&o.ref, "ref", "", "the tested branch or tag, recorded in the result")
	fs.StringVar(&o.ciRun, "ci-run", "", "link to the CI run, recorded in the result")
	fs.StringVar(&o.rulesRef, "rules-ref", "", "where the rules file was read from (e.g. origin/main:quality.yaml), recorded in the result")
	fs.BoolVar(&o.noSnapshot, "no-snapshot", false, "don't keep the spans of each evidence execution in --json and --html")
	fs.BoolVar(&o.record, "record", false, "record the evaluation in Troubleshooting as the project's history (requirements and their history on the Quality page)")
	fs.BoolVar(&o.reevaluate, "reevaluate", false, "with --record: record a new evaluation even if the same run, window, rules and commit were recorded before (otherwise that record is the answer)")
	o.basicAuth = os.Getenv("TROUBLESHOOTING_BASIC_AUTH")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "from", "to", "since":
			o.windowSet = true
		}
	})
	if o.ui == "" {
		o.ui = o.url
	}
	req, err := buildRequest(o, now())
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitError
	}
	ev, err := evaluate(o, req)
	if err != nil && !o.windowSet && strings.Contains(err.Error(), "time_from must be") {
		// A Troubleshooting from before windows from the run needs one: the
		// old default, the last --since.
		fmt.Fprintf(stderr, "note: this Troubleshooting needs a window; checking executions of the last %s (--since)\n", o.since)
		o.windowSet = true
		if req, err = buildRequest(o, now()); err == nil {
			ev, err = evaluate(o, req)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitError
	}
	if !o.noSnapshot && (o.jsonPath != "" || o.htmlPath != "") {
		addSnapshots(o, &ev) // a recorded evaluation already has them
	}
	// A recorded evaluation names what it recorded; a fresh one what was sent.
	if ev.ID == "" || ev.Source == nil {
		ev.Source = req.Source
	}
	if o.record {
		if ev.RecordError != "" {
			fmt.Fprintln(stderr, "warning:", ev.RecordError)
		} else if ev.ID == "" {
			fmt.Fprintln(stderr, "warning: the API did not record the evaluation (an older Troubleshooting?)")
		}
	}
	if o.ui != "none" {
		addLinks(&ev, o.ui)
	}
	b, _ := json.MarshalIndent(ev, "", "  ")
	for _, f := range []struct{ path, body string }{
		{o.jsonPath, string(b) + "\n"}, {o.htmlPath, htmlReport(ev)}, {o.mdPath, markdownReport(ev, htmlName(o.htmlPath))},
	} {
		if f.path == "" {
			continue
		}
		if err := os.WriteFile(f.path, []byte(f.body), 0o644); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return exitError
		}
	}
	fmt.Fprint(stdout, report(ev))
	if ev.ID != "" {
		// An id with a record error (an API before record_unconfirmed sends
		// only the error) is not a confirmed record.
		if ev.RecordUnconfirmed || ev.RecordError != "" {
			fmt.Fprintf(stdout, "\nNot confirmed: the evaluation was sent to the history as %s, but the API couldn't verify what is stored; the verdict above is this run's, and the recorded one may differ%s\n",
				ev.ID, recordLink(o.ui, ev.ID))
		} else if ev.RecordedBefore {
			fmt.Fprintf(stdout, "\nAlready recorded as evaluation %s (%s): this is that record, not a new evaluation; --reevaluate records a new one%s\n",
				ev.ID, ev.RecordedAt, recordLink(o.ui, ev.ID))
		} else {
			fmt.Fprintf(stdout, "\nRecorded as evaluation %s%s\n", ev.ID, recordLink(o.ui, ev.ID))
		}
	}
	return exitCode(ev.Gate)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func exitCode(gate string) int {
	switch gate {
	case "passed":
		return exitPassed
	case "failed":
		return exitFailed
	case "unresolved":
		return exitUnresolved
	}
	return exitError
}

// loadRules parses YAML (JSON is YAML too), rejecting unknown keys and
// duplicate keys.
func loadRules(path string) (Rules, string, error) {
	r, _, digest, err := readRules(path)
	return r, digest, err
}

// readRules is loadRules with the file's text.
func readRules(path string) (Rules, string, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Rules{}, "", "", err
	}
	// The same limits as the API's ParseQualityRules, so the CLI and the
	// Quality page accept the same files.
	if len(b) > maxRulesBytes {
		return Rules{}, "", "", fmt.Errorf("%s: the rules are larger than %d bytes", path, maxRulesBytes)
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var r Rules
	if err := dec.Decode(&r); err != nil {
		return Rules{}, "", "", fmt.Errorf("%s: %w", path, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return Rules{}, "", "", fmt.Errorf("%s: only one YAML document is allowed", path)
	}
	sum := sha256.Sum256(b)
	return r, string(b), hex.EncodeToString(sum[:]), nil
}

// window resolves --from/--to or --since into Unix nanoseconds.
func window(from, to string, since time.Duration, now time.Time) (string, string, error) {
	end := now
	if to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err != nil {
			return "", "", fmt.Errorf("--to: %w", err)
		}
		end = t
	}
	start := end.Add(-since)
	if from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err != nil {
			return "", "", fmt.Errorf("--from: %w", err)
		}
		start = t
	}
	if !end.After(start) {
		return "", "", errors.New("the window is empty")
	}
	return strconv.FormatInt(start.UnixNano(), 10), strconv.FormatInt(end.UnixNano(), 10), nil
}

func buildRequest(o options, now time.Time) (request, error) {
	if strings.TrimSpace(o.run) == "" {
		return request{}, errors.New("--run is required")
	}
	_, text, digest, err := readRules(o.rules)
	if err != nil {
		return request{}, err
	}
	req := request{RulesYAML: text, Record: o.record, Reevaluate: o.reevaluate && o.record, Run: o.run, Baseline: o.baseline,
		Source: &Source{Commit: o.commit, PRHead: prHead(o.commit, o.prHead), Ref: o.ref, CIRun: o.ciRun, Rules: o.rules, RulesRef: o.rulesRef, RulesSHA256: digest}}
	if !o.windowSet {
		// The API takes the run's own window, and the baseline's own unless
		// --baseline-* says otherwise.
		if o.baseline != "" && (o.baselineSince != 0 || o.baselineFrom != "" || o.baselineTo != "") {
			since := o.baselineSince
			if since == 0 {
				since = o.since
			}
			if req.BaselineFrom, req.BaselineTo, err = window(o.baselineFrom, o.baselineTo, since, now); err != nil {
				return request{}, fmt.Errorf("baseline %w", err)
			}
		}
		return req, nil
	}
	if req.TimeFrom, req.TimeTo, err = window(o.from, o.to, o.since, now); err != nil {
		return request{}, err
	}
	if o.baseline != "" && (o.baselineSince != 0 || o.baselineFrom != "" || o.baselineTo != "") {
		since := o.baselineSince
		if since == 0 {
			since = o.since
		}
		if req.BaselineFrom, req.BaselineTo, err = window(o.baselineFrom, o.baselineTo, since, now); err != nil {
			return request{}, fmt.Errorf("baseline %w", err)
		}
	}
	return req, nil
}

// maxRulesBytes is the API's MaxQualityRulesBytes.
const maxRulesBytes = 64 << 10

// postJSON posts payload to an API path (with --basic-auth and --timeout) and
// returns the HTTP status and at most limit bytes of the answer.
func postJSON(o options, path string, payload any, limit int64) (int, []byte, error) {
	body, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(o.url, "/")+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	if user, pass, ok := strings.Cut(o.basicAuth, ":"); ok {
		hreq.SetBasicAuth(user, pass)
	}
	resp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	return resp.StatusCode, raw, err
}

func evaluate(o options, req request) (Evaluation, error) {
	endpoint := strings.TrimRight(o.url, "/") + "/api/v1/quality/evaluate"
	status, raw, err := postJSON(o, "/api/v1/quality/evaluate", req, 32<<20)
	if err != nil {
		return Evaluation{}, err
	}
	var out struct {
		Error   bool   `json:"error"`
		Message string `json:"message"`
		Data    struct {
			Evaluation *Evaluation `json:"Evaluation"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return Evaluation{}, fmt.Errorf("%s: HTTP %d, not a Troubleshooting API answer", endpoint, status)
	}
	if status != http.StatusOK || out.Error {
		return Evaluation{}, fmt.Errorf("%s: HTTP %d: %s", endpoint, status, out.Message)
	}
	if out.Data.Evaluation == nil {
		return Evaluation{}, fmt.Errorf("%s: the answer has no evaluation", endpoint)
	}
	return *out.Data.Evaluation, nil
}

// addLinks adds an execution-viewer link to every piece of evidence: the
// timeline for latency and database calls, the sequence for calls between
// services.
func addLinks(ev *Evaluation, ui string) {
	base := strings.TrimRight(strings.SplitN(ui, "#", 2)[0], "/")
	for i := range ev.Checks {
		view := "timeline"
		if ev.Checks[i].Type == "forbidden_call" {
			view = "sequence"
		}
		for j := range ev.Checks[i].Evidence {
			e := &ev.Checks[i].Evidence[j]
			q := url.Values{}
			q.Set("trace", e.TraceID)
			q.Set("from", e.TimeFrom)
			q.Set("to", e.TimeTo)
			q.Set("span", e.SpanID)
			q.Set("view", view)
			e.Link = base + "/#/executions?" + q.Encode()
		}
	}
}

func fmtValue(v *float64, unit string) string {
	if v == nil {
		return "-"
	}
	s := strconv.FormatFloat(*v, 'f', -1, 64)
	if unit == "ms" {
		s = strconv.FormatFloat(float64(int64(*v*10+0.5))/10, 'f', -1, 64) + " ms"
	}
	return s
}

var statusWord = map[string]string{"pass": "PASS", "fail": "FAIL", "insufficient": "NO DATA"}

// report is the human-readable result.
func report(ev Evaluation) string {
	var b strings.Builder
	head := map[string]string{"passed": "PASSED", "failed": "FAILED", "unresolved": "UNRESOLVED (not enough data)"}[ev.Gate]
	if head == "" {
		head = strings.ToUpper(ev.Gate)
	}
	fmt.Fprintf(&b, "Quality gate: %s\nrun %s", head, ev.Run)
	if ev.Baseline != "" {
		fmt.Fprintf(&b, ", baseline %s", ev.Baseline)
	}
	fmt.Fprintf(&b, ", %d checks\n", len(ev.Checks))
	if s := sourceText(ev.Source); s != "" {
		fmt.Fprintf(&b, "%s\n", s)
	}
	if len(ev.Requirements) > 0 {
		fmt.Fprintf(&b, "\nRequirements: %s\n", requirementCounts(ev.Requirements))
		for _, r := range ev.Requirements {
			owner := ""
			if r.Owner != "" {
				owner = " (owner " + r.Owner + ")"
			}
			fmt.Fprintf(&b, "%-14s %s  %s%s\n%-14s %s\n", titleOf(requirementWord, r.Status), r.ID, r.Title, owner, "", r.Summary)
			if r.GuardrailSummary != "" {
				fmt.Fprintf(&b, "%-14s %s\n", "", r.GuardrailSummary)
			}
		}
		if hasGuardrails(ev.Requirements) {
			fmt.Fprintf(&b, "%-14s %s\n", "", guardrailDefinition)
		}
	}
	for _, c := range ev.Checks {
		word := statusWord[c.Status]
		if word == "" {
			word = strings.ToUpper(c.Status)
		}
		advisory := ""
		if c.Advisory {
			advisory = " (advisory)"
		}
		fmt.Fprintf(&b, "\n%-7s %s%s  %s\n", word, c.ID, advisory, entryText(c.Entry))
		fmt.Fprintf(&b, "        %s\n", c.Summary)
		if c.Baseline != nil {
			if c.Baseline.Status == "insufficient" {
				fmt.Fprintf(&b, "        baseline: no data (%s)\n", c.Baseline.Summary)
			} else {
				fmt.Fprintf(&b, "        baseline: %s (%d executions)\n", fmtValue(c.Baseline.Value, c.Unit), c.Baseline.Executions)
			}
		}
		for _, d := range c.Details {
			fmt.Fprintf(&b, "        %s\n", d)
		}
		for _, e := range c.Evidence {
			fmt.Fprintf(&b, "        evidence: %s\n", e.Note)
			if e.Link != "" {
				fmt.Fprintf(&b, "        %s\n", e.Link)
			} else {
				fmt.Fprintf(&b, "        trace %s, span %s\n", e.TraceID, e.SpanID)
			}
		}
	}
	if len(ev.Assumptions) > 0 {
		b.WriteString("\n")
		for _, a := range ev.Assumptions {
			fmt.Fprintf(&b, "Note: %s\n", a)
		}
	}
	return b.String()
}

func entryText(e Entry) string {
	s := e.Service + " / " + e.Operation
	if e.SpanKind != "" {
		s += " (" + e.SpanKind + ")"
	}
	return s
}

// htmlName is the file name of the --html report, "" if none is written.
func htmlName(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}

// recordLink is ": <ui>/#/quality?evaluation=<id>", or "" without a UI.
func recordLink(ui, id string) string {
	if ui == "none" || ui == "" {
		return ""
	}
	return ": " + strings.TrimRight(strings.SplitN(ui, "#", 2)[0], "/") + "/#/quality?evaluation=" + url.QueryEscape(id)
}

// prHead is the pull request's head when it differs from the tested commit
// (a merge commit tests the head merged into its base), else "".
func prHead(commit, head string) string {
	if head == "" || strings.EqualFold(head, commit) {
		return ""
	}
	return head
}
