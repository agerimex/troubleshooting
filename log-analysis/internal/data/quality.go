package data

import (
	"context"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"gopkg.in/yaml.v3"
)

// Quality gate (milestone 3, stage 1; docs/quality-gate.md): checks one test
// run against rules kept in the application's repository. A run is the spans
// whose resource (or span) attribute troubleshooting.run names it; an
// execution is one entry span of the run and the spans below it in its trace.
// The evaluation reads canonical raw spans, so it works only within raw span
// retention, and it is computed on request (nothing is stored).

const (
	QualityContract = "quality-gate-v1"
	// AttrRun names the test run a span belongs to. Set it on the resource,
	// e.g. OTEL_RESOURCE_ATTRIBUTES=troubleshooting.run=pr-42-1.
	AttrRun = "troubleshooting.run"
	// QualityPercentileMethod names the latency calculation: exact over all
	// executions, nearest rank (the smallest duration that p% of the
	// executions do not exceed).
	QualityPercentileMethod = "exact nearest-rank percentile of entry span durations"
)

// Check types.
const (
	CheckLatency   = "latency"
	CheckDBCalls   = "db_calls"
	CheckForbidden = "forbidden_call"
)

// Check statuses, target statuses and the gate.
const (
	CheckPass         = "pass"
	CheckFail         = "fail"
	CheckInsufficient = "insufficient"

	TargetMet  = "met"
	TargetOpen = "open"

	GatePassed     = "passed"
	GateFailed     = "failed"
	GateUnresolved = "unresolved"
)

// Limits of one evaluation.
const (
	MaxQualityWindow  = 24 * time.Hour
	MaxQualityChecks  = 50
	MaxQualitySpans   = 200_000
	qualityLookahead  = 5 * time.Minute
	defaultMinLatency = 20
	maxQualityDetails = 5
	rootParent        = "0000000000000000"
)

// QualityEntry is the operation an execution starts with.
type QualityEntry struct {
	Service   string `json:"service" yaml:"service"`
	Operation string `json:"operation" yaml:"operation"`
	// SpanKind is optional (server, consumer, internal, ...).
	SpanKind string `json:"span_kind,omitempty" yaml:"span_kind,omitempty"`
}

func (e QualityEntry) String() string {
	s := e.Service + " / " + e.Operation
	if e.SpanKind != "" {
		s += " (" + e.SpanKind + ")"
	}
	return s
}

// QualityCheck is one rule. Fields apply by type:
//   - latency: the percentile (default 95) of entry durations <= max_ms
//     (target_ms is advisory), with at least min_executions (default 20);
//   - db_calls: every execution has at most max_per_execution recorded
//     database calls (CLIENT spans with db.system or db.system.name);
//   - forbidden_call: no execution calls service "to" (from "from", if set).
type QualityCheck struct {
	ID              string        `json:"id" yaml:"id"`
	Type            string        `json:"type" yaml:"type"`
	Entry           *QualityEntry `json:"entry,omitempty" yaml:"entry,omitempty"`
	Advisory        bool          `json:"advisory,omitempty" yaml:"advisory,omitempty"`
	MinExecutions   int           `json:"min_executions,omitempty" yaml:"min_executions,omitempty"`
	Percentile      float64       `json:"percentile,omitempty" yaml:"percentile,omitempty"`
	MaxMs           *float64      `json:"max_ms,omitempty" yaml:"max_ms,omitempty"`
	TargetMs        *float64      `json:"target_ms,omitempty" yaml:"target_ms,omitempty"`
	MaxPerExecution *int          `json:"max_per_execution,omitempty" yaml:"max_per_execution,omitempty"`
	TargetPer       *int          `json:"target_per_execution,omitempty" yaml:"target_per_execution,omitempty"`
	From            string        `json:"from,omitempty" yaml:"from,omitempty"`
	To              string        `json:"to,omitempty" yaml:"to,omitempty"`
}

// QualityRules is the rules file (quality.yaml), version 1.
type QualityRules struct {
	Version int            `json:"version" yaml:"version"`
	Entry   *QualityEntry  `json:"entry,omitempty" yaml:"entry,omitempty"`
	Checks  []QualityCheck `json:"checks" yaml:"checks"`
}

// QualityRequest evaluates Run in the window [TimeFrom, TimeTo) of entry
// start times and, for comparison only, Baseline in its own window (by
// default the same one).
type QualityRequest struct {
	Rules QualityRules `json:"rules"`
	// RulesYAML is the rules file as text (the UI sends it as typed); it is
	// parsed strictly like the CLI does: unknown and duplicate keys are errors.
	// A request has either rules or rules_yaml.
	RulesYAML    string `json:"rules_yaml,omitempty"`
	Run          string `json:"run"`
	TimeFrom     string `json:"time_from"`
	TimeTo       string `json:"time_to"`
	Baseline     string `json:"baseline,omitempty"`
	BaselineFrom string `json:"baseline_time_from,omitempty"`
	BaselineTo   string `json:"baseline_time_to,omitempty"`
}

var checkIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func validRunID(s string) bool {
	return s != "" && len(s) <= 128 && !strings.ContainsAny(s, "\x00\n\r")
}

func finitePositive(v *float64) bool {
	return v != nil && !math.IsNaN(*v) && !math.IsInf(*v, 0) && *v > 0
}

// MaxQualityRulesBytes bounds a rules file.
const MaxQualityRulesBytes = 64 << 10

// ParseQualityRules reads a rules file (YAML; JSON is YAML too). Unknown keys,
// duplicate keys and more than one document are errors, so a misspelled
// limit is never silently ignored. The result still needs Validate.
func ParseQualityRules(text string) (QualityRules, error) {
	if len(text) > MaxQualityRulesBytes {
		return QualityRules{}, invalid("the rules are larger than %d bytes", MaxQualityRulesBytes)
	}
	dec := yaml.NewDecoder(strings.NewReader(text))
	dec.KnownFields(true)
	var r QualityRules
	if err := dec.Decode(&r); err != nil {
		return QualityRules{}, invalid("rules: %s", err.Error())
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return QualityRules{}, invalid("rules: only one YAML document is allowed")
	}
	return r, nil
}

func parseWindow(fromS, toS, what string) (int64, int64, error) {
	from, err := strconv.ParseInt(fromS, 10, 64)
	if err != nil {
		return 0, 0, invalid("%stime_from must be Unix nanoseconds", what)
	}
	to, err := strconv.ParseInt(toS, 10, 64)
	if err != nil {
		return 0, 0, invalid("%stime_to must be Unix nanoseconds", what)
	}
	if to <= from {
		return 0, 0, invalid("%stime_to must be after %stime_from (the window is half-open)", what, what)
	}
	if time.Duration(to-from) > MaxQualityWindow {
		return 0, 0, invalid("the %swindow is longer than %s", what, MaxQualityWindow)
	}
	return from, to, nil
}

// Validate checks the request and fills defaults (percentile, minimum
// executions, each check's entry, the baseline window). Errors name the check.
func (q *QualityRequest) Validate() error {
	if _, _, err := parseWindow(q.TimeFrom, q.TimeTo, ""); err != nil {
		return err
	}
	if strings.TrimSpace(q.RulesYAML) != "" {
		if q.Rules.Version != 0 || q.Rules.Entry != nil || len(q.Rules.Checks) > 0 {
			return invalid("send rules or rules_yaml, not both")
		}
		rules, err := ParseQualityRules(q.RulesYAML)
		if err != nil {
			return err
		}
		q.Rules, q.RulesYAML = rules, ""
	}
	q.Run = strings.TrimSpace(q.Run)
	q.Baseline = strings.TrimSpace(q.Baseline)
	if !validRunID(q.Run) {
		return invalid("run is required (the value of %s, at most 128 characters)", AttrRun)
	}
	if q.Baseline == "" && (q.BaselineFrom != "" || q.BaselineTo != "") {
		return invalid("baseline_time_from and baseline_time_to need a baseline")
	}
	if q.Baseline != "" {
		if !validRunID(q.Baseline) {
			return invalid("baseline must be a run id of at most 128 characters")
		}
		if q.Baseline == q.Run {
			return invalid("baseline must be a different run than run")
		}
		if q.BaselineFrom == "" && q.BaselineTo == "" {
			q.BaselineFrom, q.BaselineTo = q.TimeFrom, q.TimeTo
		}
		if _, _, err := parseWindow(q.BaselineFrom, q.BaselineTo, "baseline_"); err != nil {
			return err
		}
	}
	r := &q.Rules
	if r.Version != 1 {
		return invalid("rules version must be 1")
	}
	if len(r.Checks) == 0 {
		return invalid("rules have no checks")
	}
	if len(r.Checks) > MaxQualityChecks {
		return invalid("rules have more than %d checks", MaxQualityChecks)
	}
	if r.Entry != nil {
		if err := normalizeEntry(r.Entry); err != nil {
			return invalid("entry: %s", err.Error())
		}
	}
	seen := map[string]bool{}
	for i := range r.Checks {
		c := &r.Checks[i]
		if !checkIDPattern.MatchString(c.ID) {
			return invalid("check %d: id must be 1-64 letters, digits, '_', '.' or '-'", i+1)
		}
		if seen[c.ID] {
			return invalid("check %s: duplicate id", c.ID)
		}
		seen[c.ID] = true
		if err := validateCheck(c, r.Entry); err != nil {
			return invalid("check %s: %s", c.ID, err.Error())
		}
	}
	return nil
}

func normalizeEntry(e *QualityEntry) error {
	e.Service = strings.TrimSpace(e.Service)
	e.Operation = strings.TrimSpace(e.Operation)
	e.SpanKind = strings.ToLower(strings.TrimSpace(e.SpanKind))
	if e.Service == "" || e.Operation == "" {
		return fmt.Errorf("service and operation are required")
	}
	return nil
}

func validateCheck(c *QualityCheck, entry *QualityEntry) error {
	if c.Entry == nil {
		if entry == nil {
			return fmt.Errorf("no entry: set entry on the check or at the top of the rules")
		}
		e := *entry
		c.Entry = &e
	} else if err := normalizeEntry(c.Entry); err != nil {
		return fmt.Errorf("entry: %s", err.Error())
	}
	if c.MinExecutions < 0 {
		return fmt.Errorf("min_executions must not be negative")
	}
	switch c.Type {
	case CheckLatency:
		if c.Percentile == 0 {
			c.Percentile = 95
		}
		if math.IsNaN(c.Percentile) || c.Percentile <= 0 || c.Percentile > 100 {
			return fmt.Errorf("percentile must be in (0, 100]")
		}
		if !finitePositive(c.MaxMs) {
			return fmt.Errorf("max_ms must be a positive number")
		}
		if c.TargetMs != nil && (!finitePositive(c.TargetMs) || *c.TargetMs > *c.MaxMs) {
			return fmt.Errorf("target_ms must be a positive number no larger than max_ms")
		}
		if c.MinExecutions == 0 {
			c.MinExecutions = defaultMinLatency
		}
	case CheckDBCalls:
		if c.MaxPerExecution == nil || *c.MaxPerExecution < 0 {
			return fmt.Errorf("max_per_execution must be a number >= 0")
		}
		if c.TargetPer != nil && (*c.TargetPer < 0 || *c.TargetPer > *c.MaxPerExecution) {
			return fmt.Errorf("target_per_execution must be between 0 and max_per_execution")
		}
	case CheckForbidden:
		c.From, c.To = strings.TrimSpace(c.From), strings.TrimSpace(c.To)
		if c.To == "" {
			return fmt.Errorf("to (the forbidden service) is required")
		}
		if c.From == c.To {
			return fmt.Errorf("from and to must differ")
		}
	default:
		return fmt.Errorf("type must be %s, %s or %s", CheckLatency, CheckDBCalls, CheckForbidden)
	}
	if c.MinExecutions == 0 {
		c.MinExecutions = 1
	}
	return nil
}

// QualityEvidence points to one execution; the window opens it in the
// execution viewer.
type QualityEvidence struct {
	TraceID  string `json:"trace_id"`
	SpanID   string `json:"span_id"`
	TimeFrom string `json:"time_from"`
	TimeTo   string `json:"time_to"`
	Note     string `json:"note"`
}

// QualityBaseline is the same check measured on the baseline run, for
// comparison only: it never changes the run's status or the gate.
type QualityBaseline struct {
	Status     string   `json:"status"`
	Value      *float64 `json:"value"`
	Executions int      `json:"executions"`
	Summary    string   `json:"summary"`
}

// QualityCheckResult is the outcome of one check on the run.
type QualityCheckResult struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Entry        QualityEntry      `json:"entry"`
	Advisory     bool              `json:"advisory"`
	Status       string            `json:"status"`
	Reason       string            `json:"reason"`
	Summary      string            `json:"summary"`
	Unit         string            `json:"unit"`
	Value        *float64          `json:"value"`
	Limit        *float64          `json:"limit"`
	Target       *float64          `json:"target"`
	TargetStatus string            `json:"target_status,omitempty"`
	Executions   int               `json:"executions"`
	Violations   int               `json:"violations"`
	Details      []string          `json:"details"`
	Evidence     []QualityEvidence `json:"evidence"`
	Baseline     *QualityBaseline  `json:"baseline,omitempty"`
}

// QualityResult is the whole evaluation.
type QualityResult struct {
	Contract     string               `json:"contract"`
	Method       string               `json:"method"`
	Run          string               `json:"run"`
	TimeFrom     string               `json:"time_from"`
	TimeTo       string               `json:"time_to"`
	Baseline     string               `json:"baseline,omitempty"`
	BaselineFrom string               `json:"baseline_time_from,omitempty"`
	BaselineTo   string               `json:"baseline_time_to,omitempty"`
	Gate         string               `json:"gate"`
	Checks       []QualityCheckResult `json:"checks"`
	EvaluatedAt  string               `json:"evaluated_at"`
}

// qSpan is the part of a canonical span the evaluator needs.
type qSpan struct {
	TraceID, SpanID, Parent string
	Service, Name, Kind     string
	Start                   int64 // Unix ns
	Duration                int64 // µs; negative = invalid
	DB                      bool
	Peer                    string // peer.service, else server.address
	Run                     string
}

// runSpans is what was loaded for one run. Problem (a reason code) makes
// every check of the run insufficient.
type runSpans struct {
	Spans   []qSpan
	Problem string
	Detail  string
}

// execution is one entry span and the spans below it.
type execution struct {
	Entry   *qSpan
	Subtree []*qSpan
	// Missing is a span of the trace whose parent was not recorded and that
	// starts within the entry's duration, so it may belong below the entry:
	// the execution is incomplete. nil if there is none.
	Missing *qSpan
}

type spanIndex struct {
	children map[string][]*qSpan // TraceId + ":" + parent SpanId
	byID     map[string]*qSpan
	orphans  map[string][]*qSpan // per trace id: spans whose parent was not recorded
}

// missingBelow returns the first span (in start order) of the entry's trace
// whose parent was not recorded and that starts within the entry's duration
// (from its start on, if the duration is invalid). The entry's own missing
// parent (e.g. an uninstrumented gateway above it) doesn't count: it hides
// nothing below the entry.
func (ix spanIndex) missingBelow(entry *qSpan) *qSpan {
	var first *qSpan
	for _, s := range ix.orphans[entry.TraceID] {
		if s == entry || s.Start < entry.Start || (entry.Duration >= 0 && s.Start > entry.Start+entry.Duration*1000) {
			continue
		}
		if first == nil || s.Start < first.Start || (s.Start == first.Start && s.SpanID < first.SpanID) {
			first = s
		}
	}
	return first
}

func indexSpans(spans []qSpan) spanIndex {
	ix := spanIndex{children: map[string][]*qSpan{}, byID: map[string]*qSpan{}, orphans: map[string][]*qSpan{}}
	for i := range spans {
		s := &spans[i]
		ix.byID[s.TraceID+":"+s.SpanID] = s
	}
	for i := range spans {
		s := &spans[i]
		if s.Parent == rootParent || s.Parent == "" {
			continue
		}
		key := s.TraceID + ":" + s.Parent
		ix.children[key] = append(ix.children[key], s)
		if ix.byID[key] == nil {
			ix.orphans[s.TraceID] = append(ix.orphans[s.TraceID], s)
		}
	}
	return ix
}

// executions returns the run's entry spans that start in [from, to), in start
// order, each with its subtree (by recorded parentage in the same trace).
func (ix spanIndex) executions(spans []qSpan, e QualityEntry, run string, from, to int64) []execution {
	var out []execution
	for i := range spans {
		s := &spans[i]
		if s.Service != e.Service || s.Name != e.Operation || s.Run != run || s.Start < from || s.Start >= to {
			continue
		}
		if e.SpanKind != "" && s.Kind != e.SpanKind {
			continue
		}
		ex := execution{Entry: s, Missing: ix.missingBelow(s)}
		stack := []*qSpan{s}
		seen := map[*qSpan]bool{s: true}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, c := range ix.children[n.TraceID+":"+n.SpanID] {
				if !seen[c] {
					seen[c] = true
					ex.Subtree = append(ex.Subtree, c)
					stack = append(stack, c)
				}
			}
		}
		sort.Slice(ex.Subtree, func(i, j int) bool {
			if ex.Subtree[i].Start != ex.Subtree[j].Start {
				return ex.Subtree[i].Start < ex.Subtree[j].Start
			}
			return ex.Subtree[i].SpanID < ex.Subtree[j].SpanID
		})
		out = append(out, ex)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Entry.Start != out[j].Entry.Start {
			return out[i].Entry.Start < out[j].Entry.Start
		}
		return out[i].Entry.TraceID < out[j].Entry.TraceID
	})
	return out
}

func evidenceOf(ex execution, span *qSpan, note string) QualityEvidence {
	if span == nil {
		span = ex.Entry
	}
	end := ex.Entry.Start
	if ex.Entry.Duration > 0 {
		end += ex.Entry.Duration * 1000
	}
	return QualityEvidence{TraceID: ex.Entry.TraceID, SpanID: span.SpanID,
		TimeFrom: num(ex.Entry.Start - int64(time.Minute)), TimeTo: num(end + int64(time.Minute)), Note: note}
}

func fptr(v float64) *float64 { return &v }

func fmtNum(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// fmtMs shows milliseconds with at most one decimal.
func fmtMs(v float64) string { return fmtNum(math.Round(v*10)/10) + " ms" }

// nearestRank is the p-th percentile (0 < p <= 100) of sorted values.
func nearestRank(sorted []float64, p float64) float64 {
	k := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	return sorted[max(k, 0)]
}

// measure evaluates one check over the executions of one run.
func measure(c QualityCheck, exs []execution, ix spanIndex) QualityCheckResult {
	r := QualityCheckResult{ID: c.ID, Type: c.Type, Entry: *c.Entry, Advisory: c.Advisory,
		Executions: len(exs), Details: []string{}, Evidence: []QualityEvidence{}}
	switch c.Type {
	case CheckLatency:
		measureLatency(c, exs, &r) // the entry's own duration: complete even if spans below it are missing
	case CheckDBCalls:
		measureDBCalls(c, exs, &r)
		requireComplete(exs, &r)
	case CheckForbidden:
		measureForbidden(c, exs, &r, ix)
		requireComplete(exs, &r)
	}
	return r
}

// requireComplete: a call count or the absence of a call can't pass while an
// execution may be missing spans (a span of its trace names a parent that was
// not recorded): the missing part could hold calls. A failure stands, since
// the calls it counted were recorded.
func requireComplete(exs []execution, r *QualityCheckResult) {
	incomplete := 0
	var first *execution
	for i := range exs {
		if exs[i].Missing != nil {
			incomplete++
			if first == nil {
				first = &exs[i]
			}
		}
	}
	if incomplete == 0 {
		return
	}
	r.Details = append(r.Details, fmt.Sprintf("%d of %d executions are incomplete: a span below the entry names a parent that was not recorded (lost, sampled out or not instrumented), so calls below it may be missing", incomplete, len(exs)))
	if r.Status != CheckPass {
		return
	}
	r.Status, r.Reason, r.TargetStatus = CheckInsufficient, "incomplete_executions", ""
	r.Summary = fmt.Sprintf("%d of %d executions are incomplete, so the limit can't be shown to hold (%s)", incomplete, len(exs), r.Summary)
	r.Evidence = append([]QualityEvidence{evidenceOf(*first, first.Missing,
		fmt.Sprintf("incomplete execution: %s names parent %s, which was not recorded", first.Missing.Name, first.Missing.Parent))}, r.Evidence...)
}

func insufficientExecutions(c QualityCheck, r *QualityCheckResult, n int) bool {
	if n == 0 {
		r.Status, r.Reason = CheckInsufficient, "no_executions"
		r.Summary = "no executions of " + c.Entry.String() + " in this run and window"
		return true
	}
	if n < c.MinExecutions {
		r.Status, r.Reason = CheckInsufficient, "too_few_executions"
		r.Summary = fmt.Sprintf("%d executions, at least %d needed", n, c.MinExecutions)
		return true
	}
	return false
}

func measureLatency(c QualityCheck, exs []execution, r *QualityCheckResult) {
	r.Unit = "ms"
	r.Limit, r.Target = fptr(*c.MaxMs), c.TargetMs
	var ms []float64
	invalidN := 0
	for _, ex := range exs {
		if ex.Entry.Duration < 0 {
			invalidN++
			continue
		}
		ms = append(ms, float64(ex.Entry.Duration)/1000)
	}
	if invalidN > 0 {
		r.Details = append(r.Details, fmt.Sprintf("%d executions have no valid duration and are left out", invalidN))
	}
	if len(ms) > 0 {
		sorted := append([]float64(nil), ms...)
		sort.Float64s(sorted)
		r.Value = fptr(nearestRank(sorted, c.Percentile))
	}
	label := "p" + fmtNum(c.Percentile)
	if insufficientExecutions(c, r, len(ms)) {
		if r.Value != nil {
			r.Summary += fmt.Sprintf(" (%s so far %s)", label, fmtMs(*r.Value))
		}
		return
	}
	v := *r.Value
	for _, x := range ms {
		if x > *c.MaxMs {
			r.Violations++
		}
	}
	if v <= *c.MaxMs {
		r.Status, r.Reason = CheckPass, "within_limit"
	} else {
		r.Status, r.Reason = CheckFail, "limit_exceeded"
	}
	r.Summary = fmt.Sprintf("%s %s, limit %s, %d executions", label, fmtMs(v), fmtMs(*c.MaxMs), len(ms))
	if c.TargetMs != nil {
		if v <= *c.TargetMs {
			r.TargetStatus = TargetMet
		} else {
			r.TargetStatus = TargetOpen
			r.Details = append(r.Details, "target "+fmtMs(*c.TargetMs)+" not met yet (advisory)")
		}
	}
	worst := -1
	for i, ex := range exs {
		if ex.Entry.Duration >= 0 && (worst < 0 || ex.Entry.Duration > exs[worst].Entry.Duration) {
			worst = i
		}
	}
	r.Evidence = append(r.Evidence, evidenceOf(exs[worst], nil, "slowest execution: "+fmtMs(float64(exs[worst].Entry.Duration)/1000)))
}

// dbCalls are the recorded database calls of an execution: CLIENT spans
// with db.system or db.system.name, the entry included.
func dbCalls(ex execution) []*qSpan {
	var out []*qSpan
	for _, s := range append([]*qSpan{ex.Entry}, ex.Subtree...) {
		if s.DB && s.Kind == "client" {
			out = append(out, s)
		}
	}
	return out
}

// repeatedNames lists span names with their counts, most frequent first.
func repeatedNames(spans []*qSpan) []string {
	counts := map[string]int{}
	for _, s := range spans {
		counts[s.Name]++
	}
	names := make([]string, 0, len(counts))
	for n := range counts {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	out := []string{}
	for i, n := range names {
		if i == maxQualityDetails {
			out = append(out, fmt.Sprintf("… %d more operations", len(names)-i))
			break
		}
		out = append(out, fmt.Sprintf("%s × %d", n, counts[n]))
	}
	return out
}

func measureDBCalls(c QualityCheck, exs []execution, r *QualityCheckResult) {
	r.Unit = "calls per execution"
	r.Limit = fptr(float64(*c.MaxPerExecution))
	if c.TargetPer != nil {
		r.Target = fptr(float64(*c.TargetPer))
	}
	if insufficientExecutions(c, r, len(exs)) {
		return
	}
	worst, worstN, total := 0, -1, 0
	for i, ex := range exs {
		n := len(dbCalls(ex))
		total += n
		if n > *c.MaxPerExecution {
			r.Violations++
		}
		if n > worstN {
			worst, worstN = i, n
		}
	}
	r.Value = fptr(float64(worstN))
	if worstN <= *c.MaxPerExecution {
		r.Status, r.Reason = CheckPass, "within_limit"
	} else {
		r.Status, r.Reason = CheckFail, "limit_exceeded"
	}
	r.Summary = fmt.Sprintf("up to %d database calls per execution, limit %d, %d executions", worstN, *c.MaxPerExecution, len(exs))
	if r.Violations > 0 {
		r.Summary += fmt.Sprintf(", %d over the limit", r.Violations)
	}
	if total == 0 {
		r.Details = append(r.Details, "no database calls were recorded: check that database calls are instrumented (CLIENT spans with db.system)")
	}
	if c.TargetPer != nil {
		if worstN <= *c.TargetPer {
			r.TargetStatus = TargetMet
		} else {
			r.TargetStatus = TargetOpen
			r.Details = append(r.Details, fmt.Sprintf("target %d per execution not met yet (advisory)", *c.TargetPer))
		}
	}
	if worstN > 0 {
		r.Details = append(r.Details, repeatedNames(dbCalls(exs[worst]))...)
		r.Evidence = append(r.Evidence, evidenceOf(exs[worst], nil, fmt.Sprintf("execution with the most database calls: %d", worstN)))
	}
}

// forbiddenCall finds the first call (in start order) from the execution to
// service c.To: a span of service To whose parent belongs to another service,
// or a CLIENT span naming To as its peer (peer.service or server.address) for
// a callee that is not instrumented. The caller is the parent's service or the
// CLIENT's own service; From, if set, must match it. It returns the calling
// span (the CLIENT, or the parent of the callee's span) and the caller.
func forbiddenCall(c QualityCheck, ex execution, ix spanIndex) (*qSpan, string) {
	for _, s := range ex.Subtree {
		if s.Kind == "client" && s.Peer == c.To && s.Service != c.To && (c.From == "" || s.Service == c.From) {
			return s, s.Service
		}
		if s.Service != c.To {
			continue
		}
		p := ix.byID[s.TraceID+":"+s.Parent]
		if p == nil || p.Service == c.To {
			continue
		}
		if c.From == "" || p.Service == c.From {
			return p, p.Service
		}
	}
	return nil, ""
}

func measureForbidden(c QualityCheck, exs []execution, r *QualityCheckResult, ix spanIndex) {
	r.Unit = "executions with the call"
	r.Limit = fptr(0)
	if insufficientExecutions(c, r, len(exs)) {
		return
	}
	callers := map[string]int{}
	for _, ex := range exs {
		span, caller := forbiddenCall(c, ex, ix)
		if span == nil {
			continue
		}
		r.Violations++
		callers[caller]++
		if len(r.Evidence) == 0 {
			r.Evidence = append(r.Evidence, evidenceOf(ex, span, fmt.Sprintf("%s → %s (%s)", caller, c.To, span.Name)))
		}
	}
	r.Value = fptr(float64(r.Violations))
	edge := c.To
	if c.From != "" {
		edge = c.From + " → " + c.To
	}
	if r.Violations == 0 {
		r.Status, r.Reason = CheckPass, "not_observed"
		r.Summary = fmt.Sprintf("no call to %s in %d executions", edge, len(exs))
		r.Details = append(r.Details, "not observed in this run; it does not prove the call can never happen")
		return
	}
	r.Status, r.Reason = CheckFail, "forbidden_call"
	r.Summary = fmt.Sprintf("call to %s in %d of %d executions", edge, r.Violations, len(exs))
	names := make([]string, 0, len(callers))
	for n := range callers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		r.Details = append(r.Details, fmt.Sprintf("%s → %s in %d executions", n, c.To, callers[n]))
	}
}

// evaluateRun measures every check on one run's spans.
func evaluateRun(rules QualityRules, data runSpans, run string, from, to int64) []QualityCheckResult {
	out := make([]QualityCheckResult, 0, len(rules.Checks))
	if data.Problem != "" {
		for _, c := range rules.Checks {
			out = append(out, QualityCheckResult{ID: c.ID, Type: c.Type, Entry: *c.Entry, Advisory: c.Advisory,
				Status: CheckInsufficient, Reason: data.Problem, Summary: data.Detail,
				Details: []string{}, Evidence: []QualityEvidence{}})
		}
		return out
	}
	ix := indexSpans(data.Spans)
	cache := map[QualityEntry][]execution{}
	for _, c := range rules.Checks {
		exs, ok := cache[*c.Entry]
		if !ok {
			exs = ix.executions(data.Spans, *c.Entry, run, from, to)
			cache[*c.Entry] = exs
		}
		out = append(out, measure(c, exs, ix))
	}
	return out
}

// gateOf: a blocking failure fails the gate; otherwise a blocking check
// without enough data leaves it unresolved. Advisory checks never decide it.
func gateOf(checks []QualityCheckResult) string {
	gate := GatePassed
	for _, c := range checks {
		if c.Advisory {
			continue
		}
		switch c.Status {
		case CheckFail:
			return GateFailed
		case CheckInsufficient:
			gate = GateUnresolved
		}
	}
	return gate
}

// buildQualityResult combines the run's results with the baseline's. The
// request must be validated.
func buildQualityResult(req QualityRequest, run, baseline runSpans, now time.Time) QualityResult {
	from, to, _ := parseWindow(req.TimeFrom, req.TimeTo, "")
	res := QualityResult{Contract: QualityContract, Method: QualityPercentileMethod, Run: req.Run,
		TimeFrom: req.TimeFrom, TimeTo: req.TimeTo,
		Baseline: req.Baseline, BaselineFrom: req.BaselineFrom, BaselineTo: req.BaselineTo,
		EvaluatedAt: now.UTC().Format(time.RFC3339)}
	res.Checks = evaluateRun(req.Rules, run, req.Run, from, to)
	if req.Baseline != "" {
		bFrom, bTo, _ := parseWindow(req.BaselineFrom, req.BaselineTo, "")
		base := evaluateRun(req.Rules, baseline, req.Baseline, bFrom, bTo)
		for i := range res.Checks {
			b := base[i]
			res.Checks[i].Baseline = &QualityBaseline{Status: b.Status, Value: b.Value, Executions: b.Executions, Summary: b.Summary}
		}
	}
	res.Gate = gateOf(res.Checks)
	return res
}

// qualityQuery reads one canonical row per (TraceId, SpanId) for every trace
// that has an entry span of the run starting in [from, to), up to the
// lookahead after the window (an execution may end after it). It reads one row
// more than MaxQualitySpans to detect a run that is too large.
func qualityQuery(entries []QualityEntry, from, to int64) string {
	pairs := make([][2]string, 0, len(entries))
	seen := map[[2]string]bool{}
	for _, e := range entries {
		p := [2]string{e.Service, e.Operation}
		if !seen[p] {
			seen[p] = true
			pairs = append(pairs, p)
		}
	}
	runAttr := spanAttr(AttrRun)
	return `SELECT "TraceId", "SpanId", "ParentSpanId", "ServiceName", "SpanName", "SpanKind", "UnixTime", "Duration",
		("SpanAttributes"['db.system'] != '' OR "SpanAttributes"['db.system.name'] != '') AS "IsDB",
		if("SpanAttributes"['peer.service'] != '', "SpanAttributes"['peer.service'], "SpanAttributes"['server.address']) AS "Peer",
		` + runAttr + ` AS "Run"
	FROM otel_traces
	WHERE "UnixTime" >= ` + num(from) + ` AND "UnixTime" < ` + num(to+int64(qualityLookahead)) + `
	AND "TraceId" IN (SELECT DISTINCT "TraceId" FROM otel_traces
		WHERE "UnixTime" >= ` + num(from) + ` AND "UnixTime" < ` + num(to) + `
		AND ` + runAttr + ` = {qualityRun:String}
		AND ("ServiceName", "SpanName") IN ` + pairsLiteral(pairs) + `)
	ORDER BY "TraceId", "SpanId", ` + winnerOrder + `
	LIMIT 1 BY "TraceId", "SpanId"
	LIMIT ` + num(MaxQualitySpans+1)
}

func queryProblem(ctx context.Context, err error) runSpans {
	status, msg := classify(ctx, err)
	return runSpans{Problem: "query_" + status, Detail: msg}
}

// loadRun reads one run's spans; a failure becomes the reason of every check.
func loadRun(ctx context.Context, rules QualityRules, run string, from, to int64) runSpans {
	pol := policyFor(ctx)
	if from < rawHorizon(clock.Now(), pol.TracesSeconds) {
		return runSpans{Problem: "outside_retention", Detail: "part of the window is outside raw span retention"}
	}
	entries := make([]QualityEntry, 0, len(rules.Checks))
	for _, c := range rules.Checks {
		entries = append(entries, *c.Entry)
	}
	query := qualityQuery(entries, from, to)
	params := []any{clickhouse.Named("qualityRun", run)}
	ctx, span := startQuerySpan(ctx, "SELECT quality run", query, params)
	var err error
	defer func() { endQuerySpan(span, err) }()
	rows, err := clickhouseDB.Query(withLimits(ctx), query, params...)
	if err != nil {
		return queryProblem(ctx, err)
	}
	defer rows.Close()
	var out runSpans
	for rows.Next() {
		var s qSpan
		if err = rows.Scan(&s.TraceID, &s.SpanID, &s.Parent, &s.Service, &s.Name, &s.Kind, &s.Start, &s.Duration, &s.DB, &s.Peer, &s.Run); err != nil {
			return queryProblem(ctx, err)
		}
		out.Spans = append(out.Spans, s)
	}
	if err = rows.Err(); err != nil {
		return queryProblem(ctx, err)
	}
	if len(out.Spans) > MaxQualitySpans {
		return runSpans{Problem: "too_many_spans", Detail: fmt.Sprintf("the run has more than %d spans in the window; narrow the window", MaxQualitySpans)}
	}
	return out
}

// EvaluateQuality evaluates a validated request.
func EvaluateQuality(ctx context.Context, req QualityRequest) QualityResult {
	from, to, _ := parseWindow(req.TimeFrom, req.TimeTo, "")
	run := loadRun(ctx, req.Rules, req.Run, from, to)
	var base runSpans
	if req.Baseline != "" {
		bFrom, bTo, _ := parseWindow(req.BaselineFrom, req.BaselineTo, "")
		base = loadRun(ctx, req.Rules, req.Baseline, bFrom, bTo)
	}
	return buildQualityResult(req, run, base, clock.Now())
}
