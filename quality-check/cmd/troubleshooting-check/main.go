// Command troubleshooting-check checks a test run against quality rules kept
// in your repository (quality.yaml) and exits with the gate's result:
//
//	0 passed, 1 failed, 2 unresolved (not enough data), 3 error (no result).
//
// Usage:
//
//	troubleshooting-check --url http://localhost:8095 --rules quality.yaml \
//	    --run pr-42-1 [--baseline main-118] [--since 1h] [--json result.json] [--html report.html] [--markdown summary.md]
//
// The run is the value of the troubleshooting.run attribute on the spans of
// the test run (e.g. OTEL_RESOURCE_ATTRIBUTES=troubleshooting.run=pr-42-1).
// See docs/quality-gate.md.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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
	From            string   `yaml:"from,omitempty" json:"from,omitempty"`
	To              string   `yaml:"to,omitempty" json:"to,omitempty"`
}

type Rules struct {
	Version int     `yaml:"version" json:"version"`
	Entry   *Entry  `yaml:"entry,omitempty" json:"entry,omitempty"`
	Checks  []Check `yaml:"checks" json:"checks"`
}

type request struct {
	Rules        Rules  `json:"rules"`
	Run          string `json:"run"`
	TimeFrom     string `json:"time_from"`
	TimeTo       string `json:"time_to"`
	Baseline     string `json:"baseline,omitempty"`
	BaselineFrom string `json:"baseline_time_from,omitempty"`
	BaselineTo   string `json:"baseline_time_to,omitempty"`
}

type Evidence struct {
	TraceID  string `json:"trace_id"`
	SpanID   string `json:"span_id"`
	TimeFrom string `json:"time_from"`
	TimeTo   string `json:"time_to"`
	Note     string `json:"note"`
	Link     string `json:"link,omitempty"`
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
}

type options struct {
	url, ui, rules, run, baseline, jsonPath, htmlPath, mdPath string
	from, to, baselineFrom, baselineTo                        string
	since, baselineSince                                      time.Duration
	timeout                                                   time.Duration
	basicAuth                                                 string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, time.Now))
}

func run(args []string, stdout, stderr io.Writer, now func() time.Time) int {
	fs := flag.NewFlagSet("troubleshooting-check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o options
	fs.StringVar(&o.url, "url", envOr("TROUBLESHOOTING_URL", "http://localhost:8095"), "Troubleshooting URL (the UI origin, which serves /api; or the API itself)")
	fs.StringVar(&o.ui, "ui", "", "UI URL for evidence links (default: --url)")
	fs.StringVar(&o.rules, "rules", "quality.yaml", "rules file (YAML or JSON)")
	fs.StringVar(&o.run, "run", "", "run to check: the value of troubleshooting.run on its spans (required)")
	fs.StringVar(&o.baseline, "baseline", "", "run to compare with (shown next to each value; never changes the result)")
	fs.DurationVar(&o.since, "since", time.Hour, "window: executions that started this long before now")
	fs.StringVar(&o.from, "from", "", "window start, RFC 3339 (instead of --since)")
	fs.StringVar(&o.to, "to", "", "window end, RFC 3339 (default now)")
	fs.DurationVar(&o.baselineSince, "baseline-since", 0, "baseline window (default: the run's window)")
	fs.StringVar(&o.baselineFrom, "baseline-from", "", "baseline window start, RFC 3339")
	fs.StringVar(&o.baselineTo, "baseline-to", "", "baseline window end, RFC 3339 (default now)")
	fs.StringVar(&o.jsonPath, "json", "", "write the evaluation as JSON to this file")
	fs.StringVar(&o.htmlPath, "html", "", "write a self-contained HTML report to this file")
	fs.StringVar(&o.mdPath, "markdown", "", "write a Markdown summary (for a CI job summary or a PR comment) to this file")
	fs.DurationVar(&o.timeout, "timeout", 2*time.Minute, "request timeout")
	o.basicAuth = os.Getenv("TROUBLESHOOTING_BASIC_AUTH")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if o.ui == "" {
		o.ui = o.url
	}
	req, err := buildRequest(o, now())
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitError
	}
	ev, err := evaluate(o, req)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitError
	}
	addLinks(&ev, o.ui)
	b, _ := json.MarshalIndent(ev, "", "  ")
	for _, f := range []struct{ path, body string }{
		{o.jsonPath, string(b) + "\n"}, {o.htmlPath, htmlReport(ev)}, {o.mdPath, markdownReport(ev)},
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
func loadRules(path string) (Rules, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Rules{}, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var r Rules
	if err := dec.Decode(&r); err != nil {
		return Rules{}, fmt.Errorf("%s: %w", path, err)
	}
	return r, nil
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
	rules, err := loadRules(o.rules)
	if err != nil {
		return request{}, err
	}
	req := request{Rules: rules, Run: o.run, Baseline: o.baseline}
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

func evaluate(o options, req request) (Evaluation, error) {
	body, _ := json.Marshal(req)
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	endpoint := strings.TrimRight(o.url, "/") + "/api/v1/quality/evaluate"
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Evaluation{}, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	if user, pass, ok := strings.Cut(o.basicAuth, ":"); ok {
		hreq.SetBasicAuth(user, pass)
	}
	resp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		return Evaluation{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
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
		return Evaluation{}, fmt.Errorf("%s: HTTP %d, not a Troubleshooting API answer", endpoint, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK || out.Error {
		return Evaluation{}, fmt.Errorf("%s: HTTP %d: %s", endpoint, resp.StatusCode, out.Message)
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
			fmt.Fprintf(&b, "        evidence: %s\n        %s\n", e.Note, e.Link)
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
