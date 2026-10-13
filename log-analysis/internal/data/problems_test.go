package data

import (
	"strings"
	"testing"
)

func calls(n uint64) *uint64 { return &n }

// PW-GRP-002/004: affected calls come from distinct recorded spans; records
// without a span are listed with "Call attribution unavailable" and never
// satisfy a call threshold.
func TestErrorProblemAttribution(t *testing.T) {
	rule := ErrorRule{Enabled: true, MinAffectedCalls: 5, Grouping: GroupingErrorsV1}
	base := ErrorGroup{SourceKind: SourceLog, Service: "worker", Operation: "upload", Fingerprint: "f", Signature: "Storage returned 403 AccessDenied", Occurrences: 240, AffectedTraces: 100}

	g := base
	g.AffectedCalls = calls(100)
	p := errorProblem(g, rule, 3, "complete")
	if p == nil || !p.Qualified || p.Attribution != "" || p.Explanation.RuleRevision != 3 {
		t.Fatalf("100 calls ≥ 5 must qualify: %+v", p)
	}
	if !strings.Contains(p.Explanation.Observed, "100 affected call(s)") || !strings.Contains(p.Explanation.Observed, "240 occurrence(s)") {
		t.Errorf("observed must name calls and occurrences separately: %s", p.Explanation.Observed)
	}

	g.AffectedCalls = calls(4)
	if p := errorProblem(g, rule, 3, ""); p != nil {
		t.Errorf("4 attributable calls < 5 must not open a problem")
	}

	g.AffectedCalls = calls(0)
	g.Unlinked = 3
	p = errorProblem(g, rule, 3, "")
	if p == nil || p.Qualified || !strings.Contains(p.Attribution, "Call attribution unavailable") {
		t.Fatalf("records without a span stay discoverable but unqualified: %+v", p)
	}
	if !strings.Contains(p.Explanation.Observed, "3 record(s) without a TraceId") {
		t.Errorf("unlinked records must be named: %s", p.Explanation.Observed)
	}

	g.AffectedCalls = nil
	p = errorProblem(g, rule, 3, "")
	if p == nil || p.Qualified || !strings.Contains(p.Attribution, "before call counts were recorded") {
		t.Fatalf("unknown call counts are not zero: %+v", p)
	}
}

// PW-GRP-006: the version is not part of a problem's identity; the source
// kind is (span errors and logs stay separate problems).
func TestProblemIdentity(t *testing.T) {
	rule := ErrorRule{Enabled: true, MinAffectedCalls: 1, Grouping: GroupingErrorsV1}
	g := ErrorGroup{SourceKind: SourceLog, Service: "worker", Operation: "upload", Environment: "prod", Fingerprint: "f", AffectedCalls: calls(2),
		Versions: []VersionCount{{"1.1.0", 1}}}
	a := errorProblem(g, rule, 1, "")
	g.Versions = []VersionCount{{"1.1.1", 1}}
	b := errorProblem(g, rule, 2, "")
	if a.ID != b.ID {
		t.Errorf("another version or rule revision must keep the id: %s vs %s", a.ID, b.ID)
	}
	g.SourceKind = SourceSpanStatus
	if c := errorProblem(g, rule, 1, ""); c.ID == a.ID {
		t.Errorf("another source kind is another problem")
	}
}

func comparison(id string, bv, cv string, abs, rel *float64, class string) Analysis {
	b, c := bv, cv
	return Analysis{Id: id, Contracts: map[string]string{"comparison": ComparisonContract},
		Baseline:  PopulationStats{Population: Population{Service: "worker", Version: &b}, Label: "worker / version " + bv},
		Candidate: PopulationStats{Population: Population{Service: "worker", Version: &c}, Label: "worker / version " + cv},
		Rows: []ComparisonRow{{Operation: "upload", SpanKind: "consumer",
			Baseline: &CallStats{Calls: 1000, ValidDurations: 1000}, Candidate: &CallStats{Calls: 1000, ValidDurations: 1000},
			Metrics: []MetricComparison{{Metric: "p95", Baseline: f(100), Candidate: f(160), Absolute: abs, Relative: rel, Classification: class}}}}}
}

// PW-RUL-007, PW-CMP-005: regressions come only from generated comparisons
// with an eligible p95; both thresholds apply, the absolute one alone when
// the baseline is 0; the newest comparison of a pair wins.
func TestRegressionProblems(t *testing.T) {
	rule := RegressionRule{Enabled: true, P95Relative: 20, P95Absolute: 20}
	ps, eligible := regressionProblems([]Analysis{
		comparison("new", "1.0.0", "1.1.0", f(60), f(60), ClassRegression),
		comparison("old", "1.0.0", "1.1.0", f(5), f(5), ClassWithin),
	}, rule, 2)
	if eligible != 2 || len(ps) != 1 || ps[0].Regression.AnalysisID != "new" {
		t.Fatalf("one problem from the newest comparison of the pair: %+v", ps)
	}
	if ps[0].Explanation.RuleRevision != 2 || ps[0].Regression.CandidateCalls != 1000 {
		t.Errorf("explanation and evaluated calls: %+v", ps[0])
	}

	if ps, _ := regressionProblems([]Analysis{comparison("a", "1.0.0", "1.1.0", f(30), f(15), ClassRegression)}, rule, 1); len(ps) != 0 {
		t.Errorf("+15 %% is below the relative threshold")
	}
	if ps, _ := regressionProblems([]Analysis{comparison("a", "1.0.0", "1.1.0", f(30), nil, ClassRegression)}, rule, 1); len(ps) != 1 {
		t.Errorf("a zero baseline (no relative change) is decided by the absolute threshold")
	}
	ps, eligible = regressionProblems([]Analysis{comparison("a", "1.0.0", "1.1.0", f(60), f(60), ClassInsufficient)}, rule, 1)
	if len(ps) != 0 || eligible != 0 {
		t.Errorf("insufficient evidence is not eligible: %d problems, %d eligible", len(ps), eligible)
	}
	// A rule looser than comparison-v1 still uses only eligible metrics.
	loose := RegressionRule{Enabled: true, P95Relative: 5, P95Absolute: 5}
	if ps, _ := regressionProblems([]Analysis{comparison("a", "1.0.0", "1.1.1", f(5), f(5), ClassWithin)}, loose, 1); len(ps) != 1 {
		t.Errorf("an eligible +5 ms change meets a 5 ms / 5 %% rule")
	}
}

// PW-GRP-008: empty only after every enabled source answered completely.
func TestListStatus(t *testing.T) {
	ok := ProblemSource{Status: StatsOK}
	cases := []struct {
		sources map[string]ProblemSource
		n       int
		want    string
	}{
		{map[string]ProblemSource{ProblemSourceErrors: ok, ProblemSourceSlow: ok, ProblemSourceComparisons: ok}, 0, "empty"},
		{map[string]ProblemSource{ProblemSourceErrors: ok, ProblemSourceSlow: ok, ProblemSourceComparisons: ok}, 2, "available"},
		{map[string]ProblemSource{ProblemSourceErrors: {Status: StatsUnavailable}, ProblemSourceSlow: ok, ProblemSourceComparisons: ok}, 0, "partial"},
		{map[string]ProblemSource{ProblemSourceErrors: {Status: StatsUnavailable}, ProblemSourceSlow: {Status: "timeout"}, ProblemSourceComparisons: {Status: "disabled"}}, 0, "unavailable"},
		{map[string]ProblemSource{ProblemSourceErrors: {Status: StatsOutsideRetention}, ProblemSourceSlow: {Status: StatsOutsideRetention}, ProblemSourceComparisons: {Status: "disabled"}}, 0, "expired"},
	}
	for i, c := range cases {
		if got, _ := listStatus(c.sources, c.n); got != c.want {
			t.Errorf("case %d: got %s, want %s", i, got, c.want)
		}
	}
}

// PW-RUL-002: enabled templates are validated; disabled ones keep their
// values unchecked.
func TestDetectionRulesValidate(t *testing.T) {
	r := DefaultDetectionRules
	if errs := r.Validate(); len(errs) != 0 {
		t.Fatalf("defaults must be valid: %+v", errs)
	}
	r.Slow.DurationMs = -5
	errs := r.Validate()
	if len(errs) != 1 || errs[0].Field != "slow.durationMs" {
		t.Fatalf("a negative duration must be rejected: %+v", errs)
	}
	r.Slow.Enabled = false
	if errs := r.Validate(); len(errs) != 0 {
		t.Errorf("a disabled template keeps its values unchecked: %+v", errs)
	}
	r.Errors.MinAffectedCalls = 0
	if errs := r.Validate(); len(errs) != 1 || errs[0].Field != "errors.minAffectedCalls" {
		t.Errorf("0 calls is not a threshold: %+v", errs)
	}
}
