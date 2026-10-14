package data

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// side builds a complete, comparable population side with the given values.
func side(calls, errors uint64, p95 float64, samples uint64) Side {
	s := CallStats{Calls: calls, Errors: errors, ValidDurations: samples}
	s.finish(time.Hour, []float32{float32(p95), float32(p95), float32(p95)})
	return Side{Stats: &s, Available: true, Coverage: CoverageComplete, Sampling: "unsampled"}
}

func metric(t *testing.T, ms []MetricComparison, name string) MetricComparison {
	t.Helper()
	for _, m := range ms {
		if m.Metric == name {
			return m
		}
	}
	t.Fatalf("no metric %s", name)
	return MetricComparison{}
}

func approx(t *testing.T, what string, got *float64, want float64) {
	t.Helper()
	if got == nil || math.Abs(*got-want) > 1e-9 {
		t.Errorf("%s = %v, want %g", what, got, want)
	}
}

var hourWindow = [2]float64{3600, 3600}

// M2-CMP-001
func TestRegressionSignals(t *testing.T) {
	ms := compareMetrics(side(1000, 10, 100, 1000), side(1000, 50, 160, 1000), hourWindow, DefaultRules, nil)
	p95 := metric(t, ms, "p95")
	approx(t, "p95 difference", p95.Absolute, 60)
	approx(t, "p95 relative", p95.Relative, 60)
	ef := metric(t, ms, "error_fraction")
	approx(t, "error fraction difference", ef.Absolute, 4)
	approx(t, "error fraction relative", ef.Relative, 400)
	for _, m := range []MetricComparison{p95, ef} {
		if m.Classification != ClassRegression {
			t.Errorf("%s: %s, want regression", m.Metric, m.Classification)
		}
	}
	if ef.DeltaUnit != "percentage points" {
		t.Errorf("error fraction difference unit %q", ef.DeltaUnit)
	}
	if c := metric(t, ms, "calls"); c.Classification != ClassDescriptive {
		t.Errorf("calls classified %s", c.Classification)
	}
}

// M2-CMP-002
func TestImprovementAndWithinThreshold(t *testing.T) {
	ms := compareMetrics(side(1000, 50, 160, 1000), side(1000, 10, 105, 1000), hourWindow, DefaultRules, nil)
	p95 := metric(t, ms, "p95")
	approx(t, "p95 relative", p95.Relative, -34.375)
	if p95.Classification != ClassImprovement || metric(t, ms, "error_fraction").Classification != ClassImprovement {
		t.Errorf("want improvement signals")
	}
	ms = compareMetrics(side(1000, 10, 100, 1000), side(1000, 10, 105, 1000), hourWindow, DefaultRules, nil)
	if metric(t, ms, "p95").Classification != ClassWithin || metric(t, ms, "error_fraction").Classification != ClassWithin {
		t.Errorf("1.1.1 against 1.0.0 should be within thresholds")
	}
}

// M2-CMP-003: a zero baseline has no relative change.
func TestZeroBaseline(t *testing.T) {
	ms := compareMetrics(side(1000, 0, 0, 1000), side(1000, 20, 100, 1000), hourWindow, DefaultRules, nil)
	p95 := metric(t, ms, "p95")
	if p95.Relative != nil || p95.RelativeNote == "" || p95.Classification != ClassRegression {
		t.Errorf("p95 0 -> 100: relative %v, note %q, %s", p95.Relative, p95.RelativeNote, p95.Classification)
	}
	ef := metric(t, ms, "error_fraction")
	approx(t, "0 -> 2 %", ef.Absolute, 2)
	if ef.Relative != nil || ef.Classification != ClassRegression {
		t.Errorf("error fraction 0 -> 2: relative %v, %s", ef.Relative, ef.Classification)
	}
	ms = compareMetrics(side(1000, 0, 50, 1000), side(1000, 0, 50, 1000), hourWindow, DefaultRules, nil)
	ef = metric(t, ms, "error_fraction")
	if ef.Relative != nil || ef.Classification != ClassWithin {
		t.Errorf("0 -> 0: relative %v, %s; want unavailable and within threshold", ef.Relative, ef.Classification)
	}
}

// M2-CMP-005: mixed results stay separate.
func TestMixedResults(t *testing.T) {
	ms := compareMetrics(side(1000, 10, 200, 1000), side(1000, 50, 100, 1000), hourWindow, DefaultRules, nil)
	if metric(t, ms, "p95").Classification != ClassImprovement || metric(t, ms, "error_fraction").Classification != ClassRegression {
		t.Errorf("want a latency improvement and an error regression")
	}
}

// M2-CMP-006: a population mismatch makes metrics incomparable.
func TestMismatchIsIncomparable(t *testing.T) {
	ms := compareMetrics(side(1000, 10, 100, 1000), side(1000, 50, 160, 1000), hourWindow, DefaultRules, []string{"environment"})
	for _, name := range []string{"p95", "error_fraction"} {
		if m := metric(t, ms, name); m.Classification != ClassIncomparable || m.Baseline == nil {
			t.Errorf("%s: %s with baseline %v; want incomparable with the value shown", name, m.Classification, m.Baseline)
		}
	}
	prod, test := "production", "test"
	b := Population{Service: "s", Environment: &prod}
	c := Population{Service: "s", Environment: &test}
	mm := populationMismatches(b, c, "unsampled", "unsampled", map[string]string{"metrics": "metrics-v1", "errors": "errors-v1"}, map[string]string{"metrics": "metrics-v1", "errors": "errors-v2"})
	got := map[string][]string{}
	for _, m := range mm {
		got[m.Dimension] = m.Affects
	}
	if len(got["environment"]) != 2 || strings.Join(got["error grouping rule version"], ",") != "error_groups" {
		t.Errorf("mismatches %v: a grouping-rule mismatch must block only error groups", got)
	}
}

// M2-CMP-007: each limitation withholds the classification.
func TestInsufficientEvidence(t *testing.T) {
	cases := map[string]func(b, c *Side){
		"50 duration samples":     func(b, c *Side) { *c = side(1000, 10, 160, 50) },
		"50 calls":                func(b, c *Side) { *c = side(50, 5, 160, 1000) },
		"unknown sampling policy": func(b, c *Side) { c.Sampling = "" },
		"collection gap":          func(b, c *Side) { c.Coverage = CoverageGap },
		"unavailable baseline":    func(b, c *Side) { b.Available, b.Stats, b.Reason = false, nil, "statistics query timed out" },
		"conflicting payloads":    func(b, c *Side) { c.Stats.Conflicts = 1 },
	}
	for name, change := range cases {
		b, c := side(1000, 10, 100, 1000), side(1000, 50, 160, 1000)
		change(&b, &c)
		ms := compareMetrics(b, c, hourWindow, DefaultRules, nil)
		m := metric(t, ms, "p95")
		if name == "50 calls" {
			m = metric(t, ms, "error_fraction")
		}
		if m.Classification != ClassInsufficient || len(m.Reasons) == 0 {
			t.Errorf("%s: %s with reasons %v, want insufficient evidence with the limitation", name, m.Classification, m.Reasons)
		}
		if name == "unavailable baseline" && m.Baseline != nil {
			t.Errorf("an unavailable baseline is shown as %v", *m.Baseline)
		}
	}
}

// M2-CMP-008: no candidate calls.
func TestNoCandidateCalls(t *testing.T) {
	c := Side{Available: true, Coverage: CoverageComplete, Sampling: "unsampled"}
	ms := compareMetrics(side(1000, 10, 100, 1000), c, hourWindow, DefaultRules, nil)
	approx(t, "candidate calls", metric(t, ms, "calls").Candidate, 0)
	if m := metric(t, ms, "p95"); m.Candidate != nil || m.Classification != ClassInsufficient {
		t.Errorf("p95 without candidate calls: %v %s", m.Candidate, m.Classification)
	}
	if m := metric(t, ms, "error_fraction"); m.Candidate != nil {
		t.Errorf("error fraction without candidate calls: %v", *m.Candidate)
	}
}

// M2-CMP-004: rates over different window lengths.
func TestRatesOverDifferentWindows(t *testing.T) {
	ms := compareMetrics(side(2000, 0, 50, 2000), side(4000, 0, 50, 4000), [2]float64{3600, 7200}, DefaultRules, nil)
	rate := metric(t, ms, "rate")
	if math.Abs(*rate.Baseline-33.333) > 0.01 || math.Abs(*rate.Candidate-33.333) > 0.01 {
		t.Errorf("rates %v / %v", *rate.Baseline, *rate.Candidate)
	}
	approx(t, "call difference", metric(t, ms, "calls").Absolute, 2000)
}

// M2-LAT-004: sample thresholds per percentile.
func TestPercentileSampleThresholds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		samples uint64
		want    string
	}{{"p50", 19, PercentileInsufficient}, {"p50", 20, PercentileAvailable}, {"p95", 199, PercentileInsufficient},
		{"p95", 200, PercentileAvailable}, {"p99", 999, PercentileInsufficient}, {"p99", 1000, PercentileAvailable}, {"p95", 0, PercentileUnavailable}} {
		s := CallStats{Calls: tc.samples, ValidDurations: tc.samples}
		s.finish(time.Hour, []float32{1, 1, 1})
		if got := s.Percentiles[tc.name]; got.Status != tc.want || got.Minimum != PercentileMinimum[tc.name] {
			t.Errorf("%s with %d samples: %s (minimum %d), want %s", tc.name, tc.samples, got.Status, got.Minimum, tc.want)
		}
	}
	empty := CallStats{}
	empty.finish(time.Hour, nil)
	if empty.ErrorFraction != nil || empty.Percentiles["p95"].ValueMs != nil || empty.RatePerMinute != 0 {
		t.Errorf("no calls: fraction %v, p95 %v; want unavailable, rate 0", empty.ErrorFraction, empty.Percentiles["p95"].ValueMs)
	}
}

func groups(source string, counts map[string]uint64) ErrorGroups {
	eg := ErrorGroups{Sources: map[string]SourceResult{
		ErrorSourceLogs:  {Status: StatsOK, Coverage: CoverageAssessment{Status: CoverageComplete}},
		ErrorSourceSpans: {Status: StatsOK, Coverage: CoverageAssessment{Status: CoverageComplete}},
	}}
	if source != "" {
		eg.Sources[source] = SourceResult{Status: StatsUnavailable, Error: "unreachable"}
	}
	for key, n := range counts {
		eg.Groups = append(eg.Groups, ErrorGroup{Key: key, SourceKind: SourceLog, Occurrences: n, FirstSeen: "1", LastSeen: "2"})
	}
	return eg
}

// M2-ERR-005
func TestGroupLabels(t *testing.T) {
	b := groups("", map[string]uint64{"A": 10, "C": 4})
	c := groups("", map[string]uint64{"A": 3, "B": 2})
	labels := map[string]string{}
	for _, g := range compareGroups(b, c, nil) {
		labels[g.Key] = g.Label
	}
	want := map[string]string{"A": LabelRecurring, "B": LabelNew, "C": LabelNotObserved}
	for k, v := range want {
		if labels[k] != v {
			t.Errorf("group %s: %s, want %s", k, labels[k], v)
		}
	}
	// Regenerated while baseline log evidence is unavailable: B's novelty is
	// undetermined, and no zero is reused.
	b = groups(ErrorSourceLogs, nil)
	for _, g := range compareGroups(b, c, nil) {
		if g.Key == "B" && (g.Label != LabelUndetermined || g.Baseline.Occurrences != nil) {
			t.Errorf("B with unavailable baseline: %s, baseline %v", g.Label, g.Baseline.Occurrences)
		}
	}
	// A grouping-rule mismatch blocks group conclusions.
	for _, g := range compareGroups(groups("", map[string]uint64{"A": 1}), groups("", map[string]uint64{"A": 1}), []string{"error grouping rule version"}) {
		if g.Label != LabelIncomparable {
			t.Errorf("mismatched rules: %s", g.Label)
		}
	}
}

// M2-ERR-002: the configured identifiers are normalized, status codes kept.
func TestNormalizeMessage(t *testing.T) {
	a := NormalizeMessage("Request request-A failed with status 403")
	b := NormalizeMessage("Request request-B failed with status 403")
	c := NormalizeMessage("Request request-C failed with status 500")
	if a != b || a == c || !strings.Contains(a, "403") {
		t.Errorf("normalized: %q %q %q", a, b, c)
	}
	if got := NormalizeMessage("trace 4bf92f3577b34da6a3ce929d0e0e4736 for user=42  failed"); got != "trace <hex> for user-<id> failed" {
		t.Errorf("got %q", got)
	}
	// Ids after an identifier word and long numbers are identifiers; status
	// codes, durations, counts and ordinary words after "user" are not.
	for in, want := range map[string]string{
		"bank-gateway POST /v2/authorize failed for order 3684958: timeout after 51 ms": "bank-gateway POST /v2/authorize failed for order <id>: timeout after 51 ms",
		"Order 8688224 declined with status 403":                                        "Order <id> declined with status 403",
		"payment 12345678 rejected after 3000 ms (attempt 3)":                           "payment <num> rejected after 3000 ms (attempt 3)",
		"user agent not allowed":                                                        "user agent not allowed",
		"session 9f8e7d6c-1a2b expired":                                                 "session <id> expired",
	} {
		if got := NormalizeMessage(in); got != want {
			t.Errorf("NormalizeMessage(%q) = %q, want %q", in, got, want)
		}
	}
	// The SQL twin uses the same rules.
	sql := normalizedMessage(`"Message"`)
	for _, rule := range messageRules {
		if !strings.Contains(sql, quoteString(rule.pattern)) {
			t.Errorf("SQL lacks rule %s", rule.pattern)
		}
	}
}

// RET-007, RET-009: one representation per bucket, no prorating.
func TestPlanWindow(t *testing.T) {
	h := hourNs
	t0 := int64(1000) * h
	revisions := map[int64]Revision{t0: {Revision: 7, State: StateFinal}, t0 + h: {Revision: 8, State: StateProvisional}}
	// Both hours whole, raw retained: stored revisions only.
	plan := planWindow(t0, t0+2*h, revisions, t0, 0)
	if len(plan) != 2 || plan[0].Source != SourceStored || plan[1].Source != SourceStored {
		t.Fatalf("plan %+v", plan)
	}
	// Half an hour: raw records, the whole bucket next to it stored.
	plan = planWindow(t0, t0+h+h/2, revisions, t0, 0)
	if plan[1].Source != SourceRaw || plan[1].WholeBucket {
		t.Errorf("partial bucket %+v, want raw", plan[1])
	}
	// Raw expired: a partial bucket is unavailable, with a whole-bucket alternative.
	plan = planWindow(t0+h/6, t0+h+h/6, revisions, t0+10*h, 0)
	if plan[0].Source != SourceUnavailable || !strings.HasPrefix(plan[0].Reason, "exact bounds") {
		t.Errorf("expired partial bucket %+v", plan[0])
	}
	if from, to, ok := wholeBucketAlternative(t0+h/6, t0+h+h/6, revisions, 0); !ok || from != t0 || to != t0+2*h {
		t.Errorf("alternative [%d, %d) %v", from, to, ok)
	}
	// Statistics expired too: outside retained coverage, never zero.
	plan = planWindow(t0, t0+h, revisions, t0+10*h, t0+5*h)
	if plan[0].Source != SourceUnavailable || !strings.HasPrefix(plan[0].Reason, "outside retained coverage") {
		t.Errorf("expired bucket %+v", plan[0])
	}
}

// CAL-006, CAL-008: coverage from heartbeats and declarations.
func TestCoverageAssessment(t *testing.T) {
	from := int64(1_000_000) * 60 * 1e9
	to := from + int64(time.Hour)
	later := time.Unix(0, to).Add(time.Hour)
	full := coverageEvidence{minutes: map[int64]bool{}}
	for m := from / 1e9; m < to/1e9; m += 60 {
		full.minutes[m] = true
	}
	if a := full.assess(from, to, "traces", later); a.Status != CoverageComplete {
		t.Errorf("full heartbeats: %s", a.Status)
	}
	partial := coverageEvidence{minutes: map[int64]bool{from / 1e9: true}}
	if a := partial.assess(from, to, "traces", later); a.Status != CoverageGap {
		t.Errorf("one heartbeat: %s, want gap", a.Status)
	}
	if a := (coverageEvidence{minutes: map[int64]bool{}}).assess(from, to, "traces", later); a.Status != CoverageUnknown {
		t.Errorf("nothing known: %s, want unknown", a.Status)
	}
	declared := coverageEvidence{minutes: map[int64]bool{}, declarations: []declared{{from: from, to: to, source: "all", status: CoverageComplete, reason: "fixture"}}}
	if a := declared.assess(from, to, "logs", later); a.Status != CoverageComplete {
		t.Errorf("declared complete: %s", a.Status)
	}
	declared.declarations = append(declared.declarations, declared.declarations[0])
	declared.declarations[1].status = CoverageGap
	declared.declarations[1].source = "logs"
	if a := declared.assess(from, to, "logs", later); a.Status != CoverageGap {
		t.Errorf("declared gap: %s", a.Status)
	}
	if a := declared.assess(from, to, "traces", later); a.Status != CoverageComplete {
		t.Errorf("a log gap must not affect traces: %s", a.Status)
	}
	if a := full.assess(from, to, "traces", time.Unix(0, from+int64(10*time.Minute))); a.Status != CoverageOpen {
		t.Errorf("window not over: %s, want open", a.Status)
	}
}

func TestSamplingOf(t *testing.T) {
	rows := func(policies ...string) []CallStats {
		var out []CallStats
		for _, p := range policies {
			out = append(out, CallStats{Calls: 1, Sampling: []string{p}})
		}
		return out
	}
	if got := samplingOf(rows("unsampled", "unsampled")); got != "unsampled" {
		t.Errorf("got %q", got)
	}
	if got := samplingOf(rows("unsampled", "head:0.2")); got != "mixed" {
		t.Errorf("got %q", got)
	}
	if got := samplingOf(rows("")); got != "" {
		t.Errorf("undeclared without default: %q, want unknown", got)
	}
	statsConfig.DefaultSampling = "unsampled"
	defer func() { statsConfig.DefaultSampling = "" }()
	if got := samplingOf(rows("")); got != "unsampled" {
		t.Errorf("undeclared with default: %q", got)
	}
}

func TestEligibilityUsesBucketDeadline(t *testing.T) {
	cond := eligibleCondition(5, 24*time.Hour)
	if !strings.Contains(cond, `"ReceivedTime" < least(5, intDiv("UnixTime", 3600000000000) * 3600000000000 + 90000000000000)`) {
		t.Errorf("eligibility %s", cond)
	}
}

// Every placeholder of the statistics queries is bound.
func TestStatsPlaceholdersAreBound(t *testing.T) {
	op, kind, env := "o", "server", ""
	pop := Population{Service: "s", Operation: &op, SpanKind: &kind, Environment: &env, TimeFrom: "0", TimeTo: "1"}
	conds, params := pop.callConditions("")
	params = append(params, namedParam("rawservice", "s"))
	plan := []PlanPart{{from: 0, to: hourNs, Source: SourceRaw}}
	query := callsUnion(plan, 1, `"ServiceName" = {rawservice:String}`) + whereClause(conds)
	checkBound(t, query, params)

	conds, params = pop.errorConditions("")
	query = exceptionOccurrences([]interval{{0, 1}}, 1, time.Hour, `"ServiceName" = {service:String}`) + whereClause(conds)
	checkBound(t, query, params)
}

func checkBound(t *testing.T, query string, params []any) {
	t.Helper()
	bound := map[string]bool{}
	for _, param := range params {
		bound[param.(driver.NamedValue).Name] = true
	}
	for _, m := range namedPlaceholder.FindAllStringSubmatch(query, -1) {
		if !bound[m[1]] {
			t.Errorf("placeholder %q is used but not bound", m[1])
		}
	}
}
