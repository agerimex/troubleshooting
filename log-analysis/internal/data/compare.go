package data

import (
	"fmt"
	"math"
	"strings"
)

// comparison-v1: explainable change signals between two explicitly selected
// populations. They are product heuristics, not hypothesis tests: a signal
// supports further investigation, it doesn't establish a cause.

// Classifications of one metric.
const (
	ClassRegression   = "regression_signal"
	ClassImprovement  = "improvement_signal"
	ClassWithin       = "within_threshold"
	ClassInsufficient = "insufficient_evidence"
	ClassIncomparable = "incomparable"
	ClassDescriptive  = "descriptive" // call counts and rates: no signal
)

// Rules are the comparison-v1 thresholds; they are stored with each analysis.
type Rules struct {
	Version              string            `json:"version"`
	LatencyAbsoluteMs    float64           `json:"latencyAbsoluteMs"`
	LatencyRelativePct   float64           `json:"latencyRelativePct"`
	PercentileMinimum    map[string]uint64 `json:"percentileMinimum"`
	ErrorFractionMinimum uint64            `json:"errorFractionMinimumCalls"`
	ErrorFractionPp      float64           `json:"errorFractionAbsolutePp"`
	Description          string            `json:"description"`
}

// DefaultRules are comparison-v1.
var DefaultRules = Rules{
	Version:              ComparisonContract,
	LatencyAbsoluteMs:    20,
	LatencyRelativePct:   20,
	PercentileMinimum:    PercentileMinimum,
	ErrorFractionMinimum: 1000,
	ErrorFractionPp:      1,
	Description: "Latency percentile: both populations meet the percentile's sample minimum, and the change is at least 20 ms and at least 20 % of the baseline (20 ms alone when the baseline is 0). " +
		"Recorded error fraction: at least 1000 calls in each population and a change of at least 1 percentage point. " +
		"Increases are regression signals, decreases improvement signals. Thresholds are product heuristics, not statistical tests.",
}

// MetricComparison is one metric of baseline and candidate.
type MetricComparison struct {
	Metric         string   `json:"metric"` // calls, rate, error_fraction, p50, p95, p99
	Unit           string   `json:"unit"`
	DeltaUnit      string   `json:"deltaUnit"`
	Baseline       *float64 `json:"baseline"`
	Candidate      *float64 `json:"candidate"`
	Absolute       *float64 `json:"absolute"`
	Relative       *float64 `json:"relative"` // percent; nil when the baseline is 0 or a value is missing
	RelativeNote   string   `json:"relativeNote,omitempty"`
	Classification string   `json:"classification"`
	Reasons        []string `json:"reasons"`
}

// Side is what the rules know about one population.
type Side struct {
	Stats       *CallStats // nil when the population has no row (no calls) or failed
	Available   bool       // the query succeeded and covers the window
	Reason      string     // why not available
	Coverage    string
	Sampling    string // "", "mixed" or a policy
	Definitions string
}

// Mismatch is a known difference between the populations besides the
// compared version or window.
type Mismatch struct {
	Dimension string   `json:"dimension"`
	Baseline  string   `json:"baseline"`
	Candidate string   `json:"candidate"`
	Affects   []string `json:"affects"` // metrics, error_groups
}

func f(v float64) *float64 { return &v }

// compareMetrics applies comparison-v1 to one (operation, span kind) row.
// mismatched are dimensions that make the call/latency metrics incomparable.
func compareMetrics(b, c Side, window [2]float64, rules Rules, mismatched []string) []MetricComparison {
	var out []MetricComparison
	calls := func(s Side) *float64 {
		if !s.Available {
			return nil
		}
		if s.Stats == nil {
			return f(0)
		}
		return f(float64(s.Stats.Calls))
	}
	rate := func(s Side, minutes float64) *float64 {
		n := calls(s)
		if n == nil || minutes <= 0 {
			return nil
		}
		return f(*n / minutes)
	}
	out = append(out, descriptive("calls", "calls", calls(b), calls(c)))
	out = append(out, descriptive("rate", "calls per minute", rate(b, window[0]/60), rate(c, window[1]/60)))

	common := commonLimitations(b, c, mismatched)

	// Recorded error fraction, in percent of calls.
	ef := MetricComparison{Metric: "error_fraction", Unit: "percent of recorded calls", DeltaUnit: "percentage points", Reasons: []string{}}
	fraction := func(s Side) *float64 {
		if !s.Available || s.Stats == nil || s.Stats.ErrorFraction == nil {
			return nil
		}
		return f(*s.Stats.ErrorFraction * 100)
	}
	ef.Baseline, ef.Candidate = fraction(b), fraction(c)
	differences(&ef)
	ef.Reasons = append(ef.Reasons, common...)
	for _, s := range []struct {
		name string
		side Side
	}{{"baseline", b}, {"candidate", c}} {
		switch {
		case !s.side.Available:
		case s.side.Stats == nil || s.side.Stats.Calls == 0:
			ef.Reasons = append(ef.Reasons, "no "+s.name+" calls were observed: the error fraction is unavailable")
		case s.side.Stats.Calls < rules.ErrorFractionMinimum:
			ef.Reasons = append(ef.Reasons, fmt.Sprintf("only %d %s calls (the rule needs %d)", s.side.Stats.Calls, s.name, rules.ErrorFractionMinimum))
		}
	}
	ef.Classification = classify2(ef, len(mismatched) > 0, func(abs, _ float64) bool { return math.Abs(abs) >= rules.ErrorFractionPp })
	out = append(out, ef)

	for _, name := range percentileNames {
		m := MetricComparison{Metric: name, Unit: "milliseconds", DeltaUnit: "milliseconds", Reasons: []string{}}
		value := func(s Side) *float64 {
			if !s.Available || s.Stats == nil {
				return nil
			}
			return s.Stats.Percentiles[name].ValueMs
		}
		m.Baseline, m.Candidate = value(b), value(c)
		differences(&m)
		m.Reasons = append(m.Reasons, common...)
		for _, s := range []struct {
			name string
			side Side
		}{{"baseline", b}, {"candidate", c}} {
			switch {
			case !s.side.Available:
			case s.side.Stats == nil || s.side.Stats.ValidDurations == 0:
				m.Reasons = append(m.Reasons, "no valid "+s.name+" durations: "+name+" is unavailable")
			case s.side.Stats.ValidDurations < rules.PercentileMinimum[name]:
				m.Reasons = append(m.Reasons, fmt.Sprintf("only %d %s duration samples for %s (the rule needs %d)",
					s.side.Stats.ValidDurations, s.name, name, rules.PercentileMinimum[name]))
			}
		}
		m.Classification = classify2(m, len(mismatched) > 0, func(abs, base float64) bool {
			if math.Abs(abs) < rules.LatencyAbsoluteMs {
				return false
			}
			if base == 0 {
				return true // a zero baseline has no relative change: the absolute threshold alone
			}
			return math.Abs(100*abs/base) >= rules.LatencyRelativePct
		})
		out = append(out, m)
	}
	return out
}

// commonLimitations are reasons that withhold every signal of a row.
func commonLimitations(b, c Side, mismatched []string) []string {
	var reasons []string
	for _, s := range []struct {
		name string
		side Side
	}{{"baseline", b}, {"candidate", c}} {
		switch {
		case !s.side.Available:
			reasons = append(reasons, "the "+s.name+" statistics are unavailable: "+s.side.Reason)
			continue
		case s.side.Coverage != CoverageComplete:
			reasons = append(reasons, "the "+s.name+" window has "+coverageWords(s.side.Coverage)+" collection coverage")
		}
		switch s.side.Sampling {
		case "":
			reasons = append(reasons, "the "+s.name+" sampling policy is unknown")
		case "mixed":
			reasons = append(reasons, "the "+s.name+" calls declare different sampling policies")
		}
		if s.side.Stats != nil && s.side.Stats.Conflicts > 0 {
			reasons = append(reasons, fmt.Sprintf("%d %s call(s) have conflicting payloads", s.side.Stats.Conflicts, s.name))
		}
	}
	for _, d := range mismatched {
		reasons = append(reasons, "the populations differ in "+d)
	}
	return reasons
}

func coverageWords(status string) string {
	switch status {
	case CoverageGap:
		return "a gap in its"
	case CoverageOpen:
		return "incomplete (still open)"
	}
	return status
}

// differences fills Absolute and Relative from Baseline and Candidate.
func differences(m *MetricComparison) {
	if m.Baseline == nil || m.Candidate == nil {
		return
	}
	m.Absolute = f(*m.Candidate - *m.Baseline)
	if *m.Baseline == 0 {
		m.RelativeNote = "relative change unavailable: the baseline is zero"
		return
	}
	m.Relative = f(100 * (*m.Candidate - *m.Baseline) / *m.Baseline)
}

// classify2 returns the classification of a measured metric: incomparable
// for a population mismatch, insufficient evidence when any reason withholds
// it, otherwise a signal in the direction of the change (more is worse) or
// within threshold.
func classify2(m MetricComparison, mismatch bool, exceeds func(abs, base float64) bool) string {
	switch {
	case mismatch:
		return ClassIncomparable
	case len(m.Reasons) > 0 || m.Absolute == nil:
		return ClassInsufficient
	case !exceeds(*m.Absolute, *m.Baseline):
		return ClassWithin
	case *m.Absolute > 0:
		return ClassRegression
	default:
		return ClassImprovement
	}
}

func descriptive(metric, unit string, b, c *float64) MetricComparison {
	m := MetricComparison{Metric: metric, Unit: unit, DeltaUnit: unit, Baseline: b, Candidate: c, Classification: ClassDescriptive, Reasons: []string{}}
	differences(&m)
	if metric == "calls" {
		m.Reasons = append(m.Reasons, "a change in recorded call volume alone is not a latency or reliability signal")
	}
	return m
}

// populationMismatches lists differences of the definitions other than
// version and window, which are what a comparison compares. A difference in
// operation, span kind, environment or segment blocks every metric; a
// sampling difference blocks counts-based conclusions too.
func populationMismatches(b, c Population, bSampling, cSampling string, bDefs, cDefs map[string]string) []Mismatch {
	var out []Mismatch
	str := func(p *string) string {
		switch {
		case p == nil:
			return "any"
		case *p == "":
			return "not recorded"
		}
		return *p
	}
	all := []string{"metrics", "error_groups"}
	if b.Service != c.Service {
		out = append(out, Mismatch{"service", b.Service, c.Service, all})
	}
	bd, cd := b.dimensions(), c.dimensions()
	for i := range bd {
		if bd[i].column == "Version" {
			continue
		}
		if str(bd[i].value) != str(cd[i].value) {
			affects := all
			if bd[i].column == "Operation" || bd[i].column == "SpanKind" {
				affects = []string{"metrics"}
			}
			out = append(out, Mismatch{bd[i].name, str(bd[i].value), str(cd[i].value), affects})
		}
	}
	if bSampling != cSampling && bSampling != "" && cSampling != "" {
		out = append(out, Mismatch{"sampling policy", bSampling, cSampling, []string{"metrics", "error_groups"}})
	}
	if bDefs["metrics"] != cDefs["metrics"] {
		out = append(out, Mismatch{"metric definition version", bDefs["metrics"], cDefs["metrics"], []string{"metrics"}})
	}
	if bDefs["errors"] != cDefs["errors"] {
		out = append(out, Mismatch{"error grouping rule version", bDefs["errors"], cDefs["errors"], []string{"error_groups"}})
	}
	return out
}

func mismatchedFor(mismatches []Mismatch, affected string) []string {
	var out []string
	for _, m := range mismatches {
		for _, a := range m.Affects {
			if a == affected {
				out = append(out, m.Dimension)
			}
		}
	}
	return out
}

// Error group labels within a comparison. They are scoped to the selected
// populations: "not observed in candidate" doesn't mean resolved, "newly
// observed" doesn't mean first ever.
const (
	LabelRecurring    = "recurring"
	LabelNew          = "newly_observed_in_candidate"
	LabelNotObserved  = "not_observed_in_candidate"
	LabelUndetermined = "undetermined"
	LabelIncomparable = "incomparable"
)

// GroupComparison is one error group in baseline and candidate.
type GroupComparison struct {
	ErrorGroup
	Baseline           *ScopeOccurrences `json:"baseline"`
	Candidate          *ScopeOccurrences `json:"candidate"`
	Label              string            `json:"label"`
	Reason             string            `json:"reason,omitempty"`
	AbsoluteChange     *float64          `json:"absoluteChange"`
	RelativeChange     *float64          `json:"relativeChange"`
	RelativeChangeNote string            `json:"relativeChangeNote,omitempty"`
}

// ScopeOccurrences are a group's occurrences in one population; nil counts
// mean the source was unavailable.
type ScopeOccurrences struct {
	Occurrences    *uint64 `json:"occurrences"`
	AffectedTraces *uint64 `json:"affectedTraces"`
	Unlinked       *uint64 `json:"unlinked"`
	FirstSeen      string  `json:"firstSeen,omitempty"`
	LastSeen       string  `json:"lastSeen,omitempty"`
}

// sourceOf maps a group's source kind to the evidence source it comes from.
func sourceOf(kind string) string {
	if kind == SourceLog {
		return ErrorSourceLogs
	}
	return ErrorSourceSpans
}

// compareGroups labels each group seen in either population. A source that is
// unavailable, or whose coverage is not complete, makes novelty and absence
// undetermined: an earlier zero is never reused.
func compareGroups(b, c ErrorGroups, mismatched []string) []GroupComparison {
	byKey := map[string]*GroupComparison{}
	var order []string
	add := func(g ErrorGroup) *GroupComparison {
		if gc, ok := byKey[g.Key]; ok {
			return gc
		}
		gc := &GroupComparison{ErrorGroup: g}
		gc.Versions = nil
		byKey[g.Key] = gc
		order = append(order, g.Key)
		return gc
	}
	scope := func(g ErrorGroup) *ScopeOccurrences {
		return &ScopeOccurrences{Occurrences: u(g.Occurrences), AffectedTraces: u(g.AffectedTraces), Unlinked: u(g.Unlinked), FirstSeen: g.FirstSeen, LastSeen: g.LastSeen}
	}
	for _, g := range b.Groups {
		add(g).Baseline = scope(g)
	}
	for _, g := range c.Groups {
		gc := add(g)
		gc.Candidate = scope(g)
		gc.ErrorGroup.Occurrences = g.Occurrences
	}
	known := func(eg ErrorGroups, kind string) (bool, string) {
		s, ok := eg.Sources[sourceOf(kind)]
		switch {
		case !ok || s.Status != StatsOK:
			return false, "unavailable"
		case s.Coverage.Status != CoverageComplete:
			return false, "coverage " + s.Coverage.Status
		}
		return true, ""
	}
	out := make([]GroupComparison, 0, len(order))
	for _, key := range order {
		gc := byKey[key]
		bKnown, bWhy := known(b, gc.SourceKind)
		cKnown, cWhy := known(c, gc.SourceKind)
		if gc.Baseline == nil && bKnown {
			gc.Baseline = &ScopeOccurrences{Occurrences: u(0), AffectedTraces: u(0), Unlinked: u(0)}
		}
		if gc.Candidate == nil && cKnown {
			gc.Candidate = &ScopeOccurrences{Occurrences: u(0), AffectedTraces: u(0), Unlinked: u(0)}
		}
		if gc.Baseline == nil {
			gc.Baseline = &ScopeOccurrences{}
		}
		if gc.Candidate == nil {
			gc.Candidate = &ScopeOccurrences{}
		}
		bn, cn := gc.Baseline.Occurrences, gc.Candidate.Occurrences
		if bn != nil && cn != nil {
			gc.AbsoluteChange = f(float64(*cn) - float64(*bn))
			if *bn > 0 {
				gc.RelativeChange = f(100 * (float64(*cn) - float64(*bn)) / float64(*bn))
			} else {
				gc.RelativeChangeNote = "relative change unavailable: the baseline is zero"
			}
		}
		switch {
		case len(mismatched) > 0:
			gc.Label, gc.Reason = LabelIncomparable, "the populations differ in "+strings.Join(mismatched, ", ")
		case bn != nil && cn != nil && *bn > 0 && *cn > 0:
			gc.Label = LabelRecurring
		case cn != nil && *cn > 0 && (bn == nil || *bn == 0):
			if bKnown {
				gc.Label = LabelNew
			} else {
				gc.Label, gc.Reason = LabelUndetermined, "novelty undetermined: baseline "+sourceOf(gc.SourceKind)+" evidence is "+bWhy
			}
		case bn != nil && *bn > 0 && (cn == nil || *cn == 0):
			if cKnown {
				gc.Label = LabelNotObserved
			} else {
				gc.Label, gc.Reason = LabelUndetermined, "candidate "+sourceOf(gc.SourceKind)+" evidence is "+cWhy
			}
		default:
			gc.Label = LabelUndetermined
		}
		out = append(out, *gc)
	}
	return out
}

func u(n uint64) *uint64 { return &n }
