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
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"gopkg.in/yaml.v3"
)

// Quality gate (milestone 3, stage 1; docs/quality-gate.md): checks one test
// run against rules kept in the application's repository. A run is the spans
// whose resource (or span) attribute troubleshooting.run names it; an
// execution is one entry span of the run and the spans below it in its trace.
// The evaluation reads canonical raw spans, so it works only within raw span
// retention. It is computed on request; with record it is also stored, with
// the rules and evidence snapshots, as the project's history
// (quality_history.go). Requirements (quality_requirements.go) group checks
// under a named intent with an owner and a reason.

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
	// MinPerExecution (db_calls, optional) is an instrumentation guard: an
	// execution with fewer recorded database calls is missing spans.
	MinPerExecution *int   `json:"min_per_execution,omitempty" yaml:"min_per_execution,omitempty"`
	TargetPer       *int   `json:"target_per_execution,omitempty" yaml:"target_per_execution,omitempty"`
	From            string `json:"from,omitempty" yaml:"from,omitempty"`
	To              string `json:"to,omitempty" yaml:"to,omitempty"`
	// declared is run.executions when it set MinExecutions (not in JSON):
	// fewer executions then mean lost executions, not a small run.
	declared int
	// countsRequired is run.child_counts: required (not in JSON).
	countsRequired bool
}

// QualityRun is what the test run declares about itself. Executions is the
// number of executions of each checked entry the tests make: every check
// then needs at least that many, and fewer means executions were lost (not
// exported, not tagged) rather than a smaller run.
type QualityRun struct {
	Executions int `json:"executions,omitempty" yaml:"executions,omitempty"`
	// ChildCounts "required": a db_calls or forbidden_call check can't pass
	// while an execution doesn't record how many child spans its spans
	// started (lost child spans couldn't be seen there). Default "" reports
	// the coverage without requiring it.
	ChildCounts string `json:"child_counts,omitempty" yaml:"child_counts,omitempty"`
}

// ChildCountsRequired is the value of run.child_counts that requires them.
const ChildCountsRequired = "required"

// MaxDeclaredExecutions bounds run.executions.
const MaxDeclaredExecutions = 1_000_000

// QualityRules is the rules file (quality.yaml), version 1.
type QualityRules struct {
	Version int `json:"version" yaml:"version"`
	// Project names the history the evaluations are recorded under (default:
	// the service of the entry).
	Project      string               `json:"project,omitempty" yaml:"project,omitempty"`
	Run          *QualityRun          `json:"run,omitempty" yaml:"run,omitempty"`
	Entry        *QualityEntry        `json:"entry,omitempty" yaml:"entry,omitempty"`
	Checks       []QualityCheck       `json:"checks" yaml:"checks"`
	Requirements []QualityRequirement `json:"requirements,omitempty" yaml:"requirements,omitempty"`
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
	// Record stores the evaluation as the project's history (it needs
	// rules_yaml, so the exact rules are kept); Source says what was tested.
	Record bool `json:"record,omitempty"`
	// Reevaluate records a new evaluation even when the same inputs were
	// recorded before (otherwise that record is the answer).
	Reevaluate bool           `json:"reevaluate,omitempty"`
	Source     *QualitySource `json:"source,omitempty"`
	// rulesText is rules_yaml as sent, kept for the record.
	rulesText string
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
		q.Rules, q.rulesText, q.RulesYAML = rules, q.RulesYAML, ""
	}
	if q.Reevaluate && !q.Record {
		return invalid("reevaluate needs record")
	}
	if q.Record && q.rulesText == "" {
		return invalid("record needs rules_yaml: the exact rules are kept with the evaluation")
	}
	if err := q.Source.validate(); err != nil {
		return err
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
	declared := 0
	if r.Run != nil {
		if r.Run.Executions < 0 || r.Run.Executions > MaxDeclaredExecutions {
			return invalid("run.executions must be between 0 and %d", MaxDeclaredExecutions)
		}
		declared = r.Run.Executions
		if r.Run.ChildCounts != "" && r.Run.ChildCounts != ChildCountsRequired {
			return invalid("run.child_counts must be %q or left out", ChildCountsRequired)
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
		if err := validateCheck(c, r.Entry, declared); err != nil {
			return invalid("check %s: %s", c.ID, err.Error())
		}
		c.countsRequired = r.Run != nil && r.Run.ChildCounts == ChildCountsRequired
	}
	if err := validateRequirements(r); err != nil {
		return err
	}
	return validateProject(r)
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

// foreignFields rejects a field of another check type (e.g. max_ms on a
// db_calls check): it would be accepted and never applied, so a limit the
// author believes in would silently not hold.
func foreignFields(c *QualityCheck) error {
	set := map[string]bool{
		"percentile": c.Percentile != 0, "max_ms": c.MaxMs != nil, "target_ms": c.TargetMs != nil,
		"max_per_execution": c.MaxPerExecution != nil, "target_per_execution": c.TargetPer != nil, "min_per_execution": c.MinPerExecution != nil,
		"from": c.From != "", "to": c.To != "",
	}
	own := map[string][]string{
		CheckLatency:   {"percentile", "max_ms", "target_ms"},
		CheckDBCalls:   {"max_per_execution", "target_per_execution", "min_per_execution"},
		CheckForbidden: {"from", "to"},
	}[c.Type]
	if own == nil {
		return nil // an unknown type is reported by the caller
	}
	for _, f := range own {
		delete(set, f)
	}
	var bad []string
	for f, ok := range set {
		if ok {
			bad = append(bad, f)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	verb := "doesn't"
	if len(bad) > 1 {
		verb = "don't"
	}
	return fmt.Errorf("%s %s apply to a %s check", strings.Join(bad, ", "), verb, c.Type)
}

// validateCheck checks one check and fills its defaults; declared is the
// run's declared executions (0: none), the default minimum of every check.
func validateCheck(c *QualityCheck, entry *QualityEntry, declared int) error {
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
	if err := foreignFields(c); err != nil {
		return err
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
		if c.MinExecutions == 0 && declared > 0 {
			c.MinExecutions, c.declared = max(declared, defaultMinLatency), declared
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
		if c.MinPerExecution != nil && (*c.MinPerExecution < 0 || *c.MinPerExecution > *c.MaxPerExecution) {
			return fmt.Errorf("min_per_execution must be between 0 and max_per_execution")
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
	if c.MinExecutions == 0 && declared > 0 {
		c.MinExecutions, c.declared = declared, declared
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
	// Snapshot keeps the trace's spans in a recorded evaluation, so its
	// history shows the evidence after the raw spans expired.
	Snapshot *QualitySnapshot `json:"snapshot,omitempty"`
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
	// Project and Requirements: the requirements of the rules with a status
	// from their checks (quality_requirements.go).
	// Assumptions say what a verdict rests on: what the recorded spans can
	// and can't show about lost spans.
	Assumptions []string `json:"assumptions"`
	// ChildCounts: in how many checked executions lost child spans could be
	// seen (nil when nothing was loaded).
	ChildCounts  *QualityChildCoverage      `json:"child_counts,omitempty"`
	Project      string                     `json:"project"`
	Requirements []QualityRequirementResult `json:"requirements"`
	Source       *QualitySource             `json:"source,omitempty"`
	// ID is set when the evaluation was recorded (record); RecordError when
	// recording failed (the result itself is still valid).
	ID          string `json:"id,omitempty"`
	RecordedAt  string `json:"recorded_at,omitempty"`
	RecordError string `json:"record_error,omitempty"`
	// RecordedBefore: the same evaluation was recorded before (a retry), and
	// this is that record, exactly as recorded. Reevaluated: recorded as a
	// deliberate re-evaluation of inputs recorded before.
	RecordedBefore bool `json:"recorded_before,omitempty"`
	// RecordUnconfirmed: the evaluation was sent to the history under ID, but
	// reading the stored record back failed, so this answer is the local
	// verdict and the stored one may differ (two instances recording at once).
	// RecordError says why.
	RecordUnconfirmed bool `json:"record_unconfirmed,omitempty"`
	Reevaluated       bool `json:"reevaluated,omitempty"`
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
	Status                  int8   // 0 unset, 1 error, 2 ok
	PeerService, DBSystem   string // for snapshots
	// Children is the number of child spans the span started in its own
	// process, as recorded by the sender (log-sender); 0 when unknown (OTLP
	// doesn't carry it).
	Children int32
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
	// PastRead: the entry ends at or after the end of the spans read (the
	// window end plus qualityLookahead), so spans at its end were not read
	// and the execution is incomplete.
	PastRead bool
	// Unrecorded is a span of the execution that started more child spans in
	// its own process than were recorded (only known when the sender records
	// child counts): children were lost. nil if there is none.
	Unrecorded *qSpan
	// Counted: the execution records how many child spans its spans started,
	// so lost child spans would show (Unrecorded). A recorded count of 0 and
	// no count at all (OTLP) look the same, so this needs the entry span to
	// report a count and every span with recorded children of its own
	// service to report one too; an execution whose spans start no children
	// can't establish it.
	Counted bool
}

// unrecordedChildren returns the first span (in start order) of the
// execution whose recorded child count is larger than the number of its
// children of the same service that were recorded. A sender's child count
// covers the children started in its own process, i.e. of the same service;
// children in other services (remote calls) are not counted by it.
func (ix spanIndex) unrecordedChildren(ex execution) *qSpan {
	var first *qSpan
	for _, s := range append([]*qSpan{ex.Entry}, ex.Subtree...) {
		if s.Children <= 0 {
			continue
		}
		local := int32(0)
		for _, c := range ix.children[s.TraceID+":"+s.SpanID] {
			if c.Service == s.Service {
				local++
			}
		}
		if local < s.Children && (first == nil || s.Start < first.Start) {
			first = s
		}
	}
	return first
}

// counted: see execution.Counted.
func (ix spanIndex) counted(ex execution) bool {
	if ex.Entry.Children <= 0 {
		return false
	}
	for _, s := range ex.Subtree {
		if s.Children > 0 {
			continue
		}
		for _, c := range ix.children[s.TraceID+":"+s.SpanID] {
			if c.Service == s.Service {
				return false
			}
		}
	}
	return true
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
// nothing below the entry. Neither do the entry's siblings under that same
// missing parent (the gateway's other calls): they are beside the entry, not
// below it.
func (ix spanIndex) missingBelow(entry *qSpan) *qSpan {
	var first *qSpan
	for _, s := range ix.orphans[entry.TraceID] {
		if s == entry || s.Parent == entry.Parent || s.Start < entry.Start || (entry.Duration >= 0 && s.Start > entry.Start+entry.Duration*1000) {
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
		if s.Parent == RootParentSpanID || s.Parent == "" {
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
	readEnd := to + int64(qualityLookahead)
	var out []execution
	for i := range spans {
		s := &spans[i]
		if s.Service != e.Service || s.Name != e.Operation || s.Run != run || s.Start < from || s.Start >= to {
			continue
		}
		if e.SpanKind != "" && s.Kind != e.SpanKind {
			continue
		}
		ex := execution{Entry: s, Missing: ix.missingBelow(s), PastRead: s.Duration >= 0 && s.Start+s.Duration*1000 >= readEnd}
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
		ex.Unrecorded = ix.unrecordedChildren(ex)
		ex.Counted = ix.counted(ex)
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
	if c.countsRequired && (c.Type == CheckDBCalls || c.Type == CheckForbidden) {
		requireChildCounts(exs, &r)
	}
	return r
}

// requireChildCounts (run.child_counts: required): a pass needs every
// execution to record its child counts; a failure stands.
func requireChildCounts(exs []execution, r *QualityCheckResult) {
	var first *execution
	n := 0
	for i := range exs {
		if !exs[i].Counted {
			n++
			if first == nil {
				first = &exs[i]
			}
		}
	}
	if n == 0 {
		return
	}
	r.Details = append(r.Details, fmt.Sprintf("%d of %d executions don't record how many child spans their spans started, so spans lost there can't be seen (run.child_counts: required)", n, len(exs)))
	if r.Status != CheckPass {
		return
	}
	r.Status, r.Reason, r.TargetStatus = CheckInsufficient, "unverified_child_counts", ""
	r.Summary = fmt.Sprintf("%d of %d executions don't record child counts, so the limit can't be shown to hold (%s)", n, len(exs), r.Summary)
	r.Evidence = append([]QualityEvidence{evidenceOf(*first, nil, "execution without child counts: lost child spans can't be seen in it")}, r.Evidence...)
}

// requireComplete: a call count or the absence of a call can't pass while an
// execution may be missing spans: a span of its trace names a parent that was
// not recorded, or the execution runs past the end of the spans read. The
// missing part could hold calls. A failure stands, since the calls it counted
// were recorded.
func requireComplete(exs []execution, r *QualityCheckResult) {
	missing, unrecorded, pastRead := 0, 0, 0
	var first *execution
	for i := range exs {
		switch {
		case exs[i].Missing != nil:
			missing++
		case exs[i].Unrecorded != nil:
			unrecorded++
		case exs[i].PastRead:
			pastRead++
		default:
			continue
		}
		if first == nil {
			first = &exs[i]
		}
	}
	incomplete := missing + unrecorded + pastRead
	if incomplete == 0 {
		return
	}
	if missing > 0 {
		r.Details = append(r.Details, fmt.Sprintf("%d of %d executions are incomplete: a span below the entry names a parent that was not recorded (lost, sampled out or not instrumented), so calls below it may be missing", missing, len(exs)))
	}
	if unrecorded > 0 {
		r.Details = append(r.Details, fmt.Sprintf("%d of %d executions are incomplete: a span started more child spans than were recorded (lost or not exported), so calls may be missing", unrecorded, len(exs)))
	}
	if pastRead > 0 {
		r.Details = append(r.Details, fmt.Sprintf("%d of %d executions are incomplete: they run past the spans read (%s after the window's end), so calls at their end were not checked; end the window later", pastRead, len(exs), qualityLookahead))
	}
	if r.Status != CheckPass {
		return
	}
	r.Status, r.Reason, r.TargetStatus = CheckInsufficient, "incomplete_executions", ""
	r.Summary = fmt.Sprintf("%d of %d executions are incomplete, so the limit can't be shown to hold (%s)", incomplete, len(exs), r.Summary)
	span, note := first.Missing, "incomplete execution: it runs past the spans read"
	switch {
	case first.Missing != nil:
		note = fmt.Sprintf("incomplete execution: %s names parent %s, which was not recorded", first.Missing.Name, first.Missing.Parent)
	case first.Unrecorded != nil:
		span = first.Unrecorded
		note = fmt.Sprintf("incomplete execution: %s started %d child spans in its service, fewer were recorded", first.Unrecorded.Name, first.Unrecorded.Children)
	}
	r.Evidence = append([]QualityEvidence{evidenceOf(*first, span, note)}, r.Evidence...)
}

func insufficientExecutions(c QualityCheck, r *QualityCheckResult, n int) bool {
	if n == 0 {
		r.Status, r.Reason = CheckInsufficient, "no_executions"
		r.Summary = "no executions of " + c.Entry.String() + " in this run and window"
		return true
	}
	if n < c.MinExecutions && c.declared > 0 && n < c.declared {
		r.Status, r.Reason = CheckInsufficient, "missing_executions"
		r.Summary = fmt.Sprintf("%d executions, the run declares %d: executions were lost (not exported, not tagged or outside the window)", n, c.declared)
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
	if len(ms) > 0 {
		sorted := append([]float64(nil), ms...)
		sort.Float64s(sorted)
		r.Value = fptr(nearestRank(sorted, c.Percentile))
	}
	label := "p" + fmtNum(c.Percentile)
	if len(ms) == 0 && len(exs) > 0 {
		r.Status, r.Reason = CheckInsufficient, "invalid_durations"
		r.Summary = fmt.Sprintf("none of the %d executions has a valid duration (the entry never ended, or ended before it started)", len(exs))
		r.Evidence = append(r.Evidence, evidenceOf(exs[0], nil, "execution without a valid duration"))
		return
	}
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
	requireDurations(exs, invalidN, label, r)
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
	if total == 0 && *c.MaxPerExecution > 0 {
		// No database call in the whole run is far more often missing
		// instrumentation than an operation that reads nothing: not a pass.
		r.Status, r.Reason, r.TargetStatus = CheckInsufficient, "no_database_calls", ""
		r.Summary = fmt.Sprintf("no database calls were recorded in %d executions: are they instrumented (CLIENT spans with db.system)? If %s really makes none, set max_per_execution: 0", len(exs), c.Entry.String())
	}
	requireMinCalls(c, exs, r)
}

// requireMinCalls: with min_per_execution, an execution with fewer recorded
// database calls is missing spans, so a pass can't stand on it; a failure
// stands, since the calls it counted were recorded.
func requireMinCalls(c QualityCheck, exs []execution, r *QualityCheckResult) {
	if c.MinPerExecution == nil || *c.MinPerExecution == 0 {
		return
	}
	below, fewest, fewestN := 0, -1, 0
	for i, ex := range exs {
		n := len(dbCalls(ex))
		if n < *c.MinPerExecution {
			below++
			if fewest < 0 || n < fewestN {
				fewest, fewestN = i, n
			}
		}
	}
	if below == 0 {
		return
	}
	r.Details = append(r.Details, fmt.Sprintf("%d of %d executions recorded fewer than %d database calls (min_per_execution): their database spans are missing", below, len(exs), *c.MinPerExecution))
	if r.Status != CheckPass {
		return
	}
	r.Status, r.Reason, r.TargetStatus = CheckInsufficient, "missing_database_calls", ""
	r.Summary = fmt.Sprintf("%d of %d executions recorded fewer than %d database calls, so their calls can't be counted (%s)", below, len(exs), *c.MinPerExecution, r.Summary)
	r.Evidence = append([]QualityEvidence{evidenceOf(exs[fewest], nil, fmt.Sprintf("execution with the fewest database calls: %d", fewestN))}, r.Evidence...)
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
// QualityChildCoverage says in how many of the checked executions lost child
// spans could be seen: Counted of Executions record their child counts.
// Status is "all", "partial" or "none".
type QualityChildCoverage struct {
	Executions int    `json:"executions"`
	Counted    int    `json:"counted"`
	Status     string `json:"status"`
}

// evaluateRun measures every check; the coverage is over the distinct
// executions of all checked entries (nil when nothing was loaded).
func evaluateRun(rules QualityRules, data runSpans, run string, from, to int64) ([]QualityCheckResult, *QualityChildCoverage) {
	out := make([]QualityCheckResult, 0, len(rules.Checks))
	if data.Problem != "" {
		for _, c := range rules.Checks {
			out = append(out, QualityCheckResult{ID: c.ID, Type: c.Type, Entry: *c.Entry, Advisory: c.Advisory,
				Status: CheckInsufficient, Reason: data.Problem, Summary: data.Detail,
				Details: []string{}, Evidence: []QualityEvidence{}})
		}
		return out, nil
	}
	ix := indexSpans(data.Spans)
	cache := map[QualityEntry][]execution{}
	cov := &QualityChildCoverage{}
	seen := map[*qSpan]bool{}
	for _, c := range rules.Checks {
		exs, ok := cache[*c.Entry]
		if !ok {
			exs = ix.executions(data.Spans, *c.Entry, run, from, to)
			cache[*c.Entry] = exs
			for _, ex := range exs {
				if !seen[ex.Entry] {
					seen[ex.Entry] = true
					cov.Executions++
					if ex.Counted {
						cov.Counted++
					}
				}
			}
		}
		out = append(out, measure(c, exs, ix))
	}
	switch {
	case cov.Executions > 0 && cov.Counted == cov.Executions:
		cov.Status = "all"
	case cov.Counted > 0:
		cov.Status = "partial"
	default:
		cov.Status = "none"
	}
	return out, cov
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
	var coverage *QualityChildCoverage
	res.Checks, coverage = evaluateRun(req.Rules, run, req.Run, from, to)
	if req.Baseline != "" {
		bFrom, bTo, _ := parseWindow(req.BaselineFrom, req.BaselineTo, "")
		base, _ := evaluateRun(req.Rules, baseline, req.Baseline, bFrom, bTo)
		for i := range res.Checks {
			b := base[i]
			res.Checks[i].Baseline = &QualityBaseline{Status: b.Status, Value: b.Value, Executions: b.Executions, Summary: b.Summary}
		}
	}
	res.Gate = gateOf(res.Checks)
	res.Assumptions = assumptionsOf(coverage)
	res.ChildCounts = coverage
	res.Project = req.Rules.Project
	res.Requirements = requirementResults(req.Rules.Requirements, res.Checks)
	setRequirementCriteria(res.Requirements, req.Rules)
	res.Source = req.Source
	if req.Record {
		addSnapshots(&res, run)
	}
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
		` + runAttr + ` AS "Run", "StatusCode",
		"SpanAttributes"['peer.service'] AS "PeerService",
		if("SpanAttributes"['db.system'] != '', "SpanAttributes"['db.system'], "SpanAttributes"['db.system.name']) AS "DBSystem", "ChildSpanCount"
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
		if err = rows.Scan(&s.TraceID, &s.SpanID, &s.Parent, &s.Service, &s.Name, &s.Kind, &s.Start, &s.Duration, &s.DB, &s.Peer, &s.Run, &s.Status, &s.PeerService, &s.DBSystem, &s.Children); err != nil {
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
	// The run and the baseline are read at the same time: both are bounded by
	// the one request deadline.
	var run, base runSpans
	var wg sync.WaitGroup
	if req.Baseline != "" {
		bFrom, bTo, _ := parseWindow(req.BaselineFrom, req.BaselineTo, "")
		wg.Add(1)
		go func() {
			defer wg.Done()
			base = loadRun(ctx, req.Rules, req.Baseline, bFrom, bTo)
		}()
	}
	run = loadRun(ctx, req.Rules, req.Run, from, to)
	wg.Wait()
	return buildQualityResult(req, run, base, clock.Now())
}

// requireDurations: executions without a valid duration (their entry never
// ended, or ended before it started) are left out of the percentile. They are
// often the slowest ones (timeouts, crashes), so a pass can't stand on the
// rest; a failure stands, since the measured executions already broke the
// limit.
func requireDurations(exs []execution, invalidN int, label string, r *QualityCheckResult) {
	if invalidN == 0 {
		return
	}
	r.Details = append(r.Details, fmt.Sprintf("%d of %d executions have no valid duration (the entry never ended, or ended before it started) and are left out", invalidN, len(exs)))
	if r.Status != CheckPass {
		return
	}
	r.Status, r.Reason, r.TargetStatus = CheckInsufficient, "invalid_durations", ""
	r.Summary = fmt.Sprintf("%d of %d executions have no valid duration, so the %s can't be shown to hold (%s)", invalidN, len(exs), label, r.Summary)
	for _, ex := range exs {
		if ex.Entry.Duration < 0 {
			r.Evidence = append([]QualityEvidence{evidenceOf(ex, nil, "execution without a valid duration")}, r.Evidence...)
			break
		}
	}
}

// Assumptions of every verdict (quality-gate-v1).
const (
	AssumptionRecorded = "Verdicts hold for the recorded spans: every call is assumed to be instrumented, not sampled out and exported before the check."
	AssumptionCaught   = "Caught as not enough data: fewer executions than run.executions, a span naming a parent that was not recorded, executions past the spans read, and fewer database calls than min_per_execution."
	AssumptionChildren = "Lost child spans are detected too: these spans record how many children they started."
	AssumptionNoCounts = "Child spans lost without a trace can't be detected: these spans don't record how many children they started (log-sender records it; OTLP doesn't carry it)."
	// AssumptionPartialCounts takes the counted and checked executions.
	AssumptionPartialCounts = "Lost child spans are detected in %d of %d executions only: the others don't record how many children their spans started (log-sender records it; OTLP doesn't carry it), so spans lost there can't be detected. run.child_counts: required makes that unresolved."
)

// assumptionsOf: whether lost child spans can be detected is decided per
// checked execution (execution.Counted), never by one span for the whole run.
func assumptionsOf(cov *QualityChildCoverage) []string {
	out := []string{AssumptionRecorded, AssumptionCaught}
	switch {
	case cov == nil || cov.Executions == 0:
		return out
	case cov.Status == "all":
		return append(out, AssumptionChildren)
	case cov.Status == "partial":
		return append(out, fmt.Sprintf(AssumptionPartialCounts, cov.Counted, cov.Executions))
	}
	return append(out, AssumptionNoCounts)
}
