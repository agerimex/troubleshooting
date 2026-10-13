package data

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// The problem workspace lists recurring symptoms of one service as problems:
// error groups (from the prepared error statistics), slow calls (canonical
// spans above the slow-call duration) and release regressions (generated
// comparisons whose p95 change meets the rule). Problems are evaluated when
// the list is requested, with the current detection rules; they are not
// stored. Counts of different problems or source kinds are never added.

// Problem types.
const (
	ProblemError      = "error"
	ProblemSlowCalls  = "slow_calls"
	ProblemRegression = "release_regression"
)

// Problem list sources.
const (
	ProblemSourceErrors      = "errors"
	ProblemSourceSlow        = "slow_calls"
	ProblemSourceComparisons = "comparisons"
)

// ProblemPageSize is the default and largest page of problems.
const ProblemPageSize = 50

// ProblemsRequest is the body of problems.
type ProblemsRequest struct {
	Service     string  `json:"service"`
	Environment *string `json:"environment"`
	TimeFrom    string  `json:"time_from"`
	TimeTo      string  `json:"time_to"`
	Limit       int     `json:"limit"`
	// Cursor continues a list: the offset returned as Next. It belongs to the
	// same scope and rules revision.
	Cursor string `json:"cursor"`
	// RulesRevision is the revision the cursor was produced with.
	RulesRevision *int64 `json:"rules_revision"`
}

func (r *ProblemsRequest) population() Population {
	return Population{Service: r.Service, Environment: r.Environment, TimeFrom: r.TimeFrom, TimeTo: r.TimeTo}
}

// Validate checks the request.
func (r *ProblemsRequest) Validate() error {
	r.Service = strings.TrimSpace(r.Service)
	if r.Service == "" {
		return invalid("service is required: problems are listed for one service")
	}
	pop := r.population()
	if err := pop.Validate(); err != nil {
		return err
	}
	if r.Limit <= 0 || r.Limit > ProblemPageSize {
		r.Limit = ProblemPageSize
	}
	if r.Cursor != "" {
		if n, err := strconv.Atoi(r.Cursor); err != nil || n < 0 {
			return invalid("cursor must be a value returned as next")
		}
	}
	return nil
}

// SlowCalls describes a slow-call problem.
type SlowCalls struct {
	ThresholdMs float64        `json:"thresholdMs"`
	Calls       uint64         `json:"calls"` // distinct canonical calls above the threshold
	Versions    []VersionCount `json:"versions"`
}

// Regression describes a release regression from a stored comparison.
type Regression struct {
	AnalysisID       string           `json:"analysisId"`
	GeneratedAt      string           `json:"generatedAt"`
	Baseline         Population       `json:"baseline"`
	Candidate        Population       `json:"candidate"`
	BaselineLabel    string           `json:"baselineLabel"`
	CandidateLabel   string           `json:"candidateLabel"`
	Metric           MetricComparison `json:"metric"`
	BaselineCalls    uint64           `json:"baselineCalls"`
	CandidateCalls   uint64           `json:"candidateCalls"`
	BaselineSamples  uint64           `json:"baselineSamples"`
	CandidateSamples uint64           `json:"candidateSamples"`
}

// Explanation says why a problem is listed, with the rule revision used.
type Explanation struct {
	RuleRevision int64  `json:"ruleRevision"`
	Rule         string `json:"rule"`
	Observed     string `json:"observed"`
	Threshold    string `json:"threshold"`
	Basis        string `json:"basis"`
	Scope        string `json:"scope"`
	Coverage     string `json:"coverage"`
}

// Problem is one listed problem.
type Problem struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Service     string `json:"service"`
	Operation   string `json:"operation"` // "" = not recorded
	SpanKind    string `json:"spanKind,omitempty"`
	Environment string `json:"environment"`
	// Qualified: the problem meets its rule. An error group whose calls can't
	// be attributed is listed but not qualified (Attribution says why).
	Qualified   bool        `json:"qualified"`
	Attribution string      `json:"attribution,omitempty"`
	FirstSeen   string      `json:"firstSeen,omitempty"`
	LastSeen    string      `json:"lastSeen,omitempty"`
	Error       *ErrorGroup `json:"error,omitempty"`
	Slow        *SlowCalls  `json:"slow,omitempty"`
	Regression  *Regression `json:"regression,omitempty"`
	Explanation Explanation `json:"explanation"`
}

// ProblemSource is the status of one source of the list.
type ProblemSource struct {
	Status    string `json:"status"` // ok, unavailable, timeout, error, outside_retention, disabled
	Condition string `json:"condition"`
}

// ProblemList answers problems.
type ProblemList struct {
	Scope       Population               `json:"scope"`
	Rules       DetectionRules           `json:"rules"`
	Status      string                   `json:"status"` // available, empty, partial, unavailable, expired
	Condition   string                   `json:"condition"`
	Sources     map[string]ProblemSource `json:"sources"`
	Problems    []Problem                `json:"problems"`
	Total       int                      `json:"total"`
	Next        string                   `json:"next,omitempty"`
	Waiting     string                   `json:"waiting,omitempty"` // release regressions without comparable data
	GeneratedAt string                   `json:"generatedAt"`
	Notices     []Notice                 `json:"notices"`
}

// problemID is a stable id of a problem's identity (not of its thresholds).
func problemID(parts ...string) string {
	sum := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return "PRB-" + hex.EncodeToString(sum[:5])
}

func shortText(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// errorTitle names the recorded symptom, never a cause.
func errorTitle(g ErrorGroup) string {
	switch {
	case g.ExceptionType != "" && g.Frame != "":
		return g.ExceptionType + " at " + g.Frame
	case g.ExceptionType != "":
		return g.ExceptionType
	case g.Signature != "":
		return shortText(g.Signature, 90)
	}
	return "Error without a recorded message"
}

var sourceKindWords = map[string]string{
	SourceSpanStatus: "ERROR-status spans",
	SourceException:  "exception events",
	SourceLog:        "ERROR/FATAL logs",
}

// errorProblem turns an error group into a problem, or nil when the group
// has attributable calls below the rule's threshold.
func errorProblem(g ErrorGroup, rule ErrorRule, revision int64, coverage string) *Problem {
	p := &Problem{
		ID:   problemID(ProblemError, g.SourceKind, g.Service, g.Operation, g.Environment, g.Fingerprint),
		Type: ProblemError, Title: errorTitle(g), Service: g.Service, Operation: g.Operation, Environment: g.Environment,
		FirstSeen: g.FirstSeen, LastSeen: g.LastSeen, Error: &g,
	}
	threshold := fmt.Sprintf("at least %d affected call(s) in the selected window", rule.MinAffectedCalls)
	observed := fmt.Sprintf("%d occurrence(s) in %s, %d affected trace(s)", g.Occurrences, sourceKindWords[g.SourceKind], g.AffectedTraces)
	switch {
	case g.AffectedCalls == nil:
		p.Attribution = "Call attribution unavailable: part of the window was summarized before call counts were recorded."
	case *g.AffectedCalls == 0:
		p.Attribution = "Call attribution unavailable: the records name no span."
	default:
		calls := *g.AffectedCalls
		observed = fmt.Sprintf("%d affected call(s), %s", calls, observed)
		if calls < uint64(rule.MinAffectedCalls) {
			return nil
		}
		p.Qualified = true
	}
	if g.Unlinked > 0 {
		observed += fmt.Sprintf("; %d record(s) without a TraceId", g.Unlinked)
	}
	p.Explanation = Explanation{
		RuleRevision: revision,
		Rule:         "Error signals: an error group with enough affected calls opens a problem; groups whose calls can't be attributed are listed without meeting the threshold.",
		Observed:     observed,
		Threshold:    threshold,
		Basis:        "errors-v1: " + map[string]string{MethodException: "exception type + application frame", MethodMessage: "normalized message"}[g.Method] + ", within source kind, service, operation and environment; the version is not part of the group",
		Coverage:     coverage,
	}
	return p
}

// regressionProblems evaluates stored comparisons (newest first) with the
// rule: one problem per (service, operation, kind, environment, baseline
// version, candidate version), from its newest comparison.
func regressionProblems(analyses []Analysis, rule RegressionRule, revision int64) (problems []Problem, eligible int) {
	seen := map[string]bool{}
	for _, a := range analyses {
		bv, cv := deref(a.Baseline.Population.Version), deref(a.Candidate.Population.Version)
		for _, row := range a.Rows {
			for _, m := range row.Metrics {
				if m.Metric != "p95" {
					continue
				}
				switch m.Classification {
				case ClassRegression, ClassImprovement, ClassWithin:
					eligible++
				default:
					continue
				}
				env := deref(a.Candidate.Population.Environment)
				id := problemID(ProblemRegression, a.Candidate.Population.Service, row.Operation, row.SpanKind, env, bv, cv, "p95")
				if seen[id] {
					continue
				}
				seen[id] = true
				if m.Absolute == nil || *m.Absolute <= 0 || *m.Absolute < rule.P95Absolute ||
					(m.Relative != nil && *m.Relative < rule.P95Relative) {
					continue
				}
				r := &Regression{AnalysisID: a.Id, GeneratedAt: a.GeneratedAt, Baseline: a.Baseline.Population, Candidate: a.Candidate.Population,
					BaselineLabel: a.Baseline.Label, CandidateLabel: a.Candidate.Label, Metric: m}
				if row.Baseline != nil {
					r.BaselineCalls, r.BaselineSamples = row.Baseline.Calls, row.Baseline.ValidDurations
				}
				if row.Candidate != nil {
					r.CandidateCalls, r.CandidateSamples = row.Candidate.Calls, row.Candidate.ValidDurations
				}
				title := row.Operation + " became slower"
				if bv != "" || cv != "" {
					title += " in " + versionWord(cv) + " (against " + versionWord(bv) + ")"
				}
				rel := "baseline is 0"
				if m.Relative != nil {
					rel = fmt.Sprintf("%+.1f %%", *m.Relative)
				}
				problems = append(problems, Problem{
					ID: id, Type: ProblemRegression, Title: title, Service: a.Candidate.Population.Service,
					Operation: row.Operation, SpanKind: row.SpanKind, Environment: env, Qualified: true, Regression: r,
					LastSeen: a.GeneratedAt,
					Explanation: Explanation{
						RuleRevision: revision,
						Rule:         "Release regressions: an eligible p95 change of a generated comparison that meets both thresholds.",
						Observed:     fmt.Sprintf("p95 %s → %s (%+.1f ms, %s), comparison %s", msText(m.Baseline), msText(m.Candidate), *m.Absolute, rel, a.Id),
						Threshold:    fmt.Sprintf("at least +%g %% and +%g ms", rule.P95Relative, rule.P95Absolute),
						Basis:        "the stored comparison's p95 (" + a.Contracts["comparison"] + "), merged distributions of each population",
						Scope:        a.Baseline.Label + " → " + a.Candidate.Label,
					},
				})
			}
		}
	}
	return problems, eligible
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func versionWord(v string) string {
	if v == "" {
		return "an unrecorded version"
	}
	return "version " + v
}

func msText(v *float64) string {
	if v == nil {
		return "unavailable"
	}
	return fmt.Sprintf("%.1f ms", *v)
}

// sortProblems: qualified first, then by the size of the headline measure,
// then the latest observation.
func sortProblems(ps []Problem) {
	weight := func(p Problem) float64 {
		switch {
		case p.Error != nil && p.Error.AffectedCalls != nil:
			return float64(*p.Error.AffectedCalls)
		case p.Error != nil:
			return float64(p.Error.Occurrences)
		case p.Slow != nil:
			return float64(p.Slow.Calls)
		case p.Regression != nil && p.Regression.Metric.Relative != nil:
			return *p.Regression.Metric.Relative
		}
		return 0
	}
	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].Qualified != ps[j].Qualified {
			return ps[i].Qualified
		}
		if wi, wj := weight(ps[i]), weight(ps[j]); wi != wj {
			return wi > wj
		}
		return ps[i].ID < ps[j].ID
	})
}

// ListProblems evaluates the problems of one service in the window.
func ListProblems(ctx context.Context, req ProblemsRequest) ProblemList {
	res := ProblemList{Scope: req.population(), Sources: map[string]ProblemSource{}, Problems: []Problem{},
		Notices: []Notice{}, GeneratedAt: nanos(clock.Now())}
	rules, err := LoadDetectionRules(ctx)
	if err != nil {
		status, msg := statsStatus(ctx, err)
		res.Status, res.Condition = "unavailable", "The detection rules could not be read ("+msg+"); problems were not evaluated."
		res.Sources["rules"] = ProblemSource{Status: status, Condition: msg}
		return res
	}
	res.Rules = rules
	if req.Cursor != "" && req.RulesRevision != nil && *req.RulesRevision != rules.Revision {
		res.Status, res.Condition = "unavailable", "The detection rules changed since the first page; reload the list."
		return res
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	var all []Problem
	add := func(source string, s ProblemSource, ps []Problem) {
		mu.Lock()
		defer mu.Unlock()
		res.Sources[source] = s
		all = append(all, ps...)
	}
	run := func(f func()) {
		wg.Add(1)
		go func() { defer wg.Done(); f() }()
	}
	pop := req.population()
	if rules.Errors.Enabled {
		run(func() {
			groups := ComputeErrorGroups(ctx, pop)
			var ps []Problem
			coverage := map[string]string{}
			for name, s := range groups.Sources {
				coverage[name] = s.Coverage.Status
			}
			for _, g := range groups.Groups {
				source := ErrorSourceSpans
				if g.SourceKind == SourceLog {
					source = ErrorSourceLogs
				}
				if p := errorProblem(g, rules.Errors, rules.Revision, "collection coverage of "+source+": "+coverage[source]); p != nil {
					p.Explanation.Scope = pop.Label()
					ps = append(ps, *p)
				}
			}
			s := ProblemSource{Status: StatsOK, Condition: groups.Condition}
			if groups.Status != StatsOK {
				s.Status = groups.Status
			} else {
				for _, name := range []string{ErrorSourceSpans, ErrorSourceLogs} {
					if src, ok := groups.Sources[name]; ok && src.Status != StatsOK {
						s.Status, s.Condition = "partial", "error evidence from "+name+" is unavailable ("+src.Error+"): its groups are missing, not zero"
					}
				}
			}
			add(ProblemSourceErrors, s, ps)
		})
	} else {
		res.Sources[ProblemSourceErrors] = ProblemSource{Status: "disabled", Condition: "error signals are disabled in the detection rules"}
	}
	if rules.Slow.Enabled {
		run(func() {
			ps, s := slowCallProblems(ctx, pop, rules.Slow, rules.Revision)
			add(ProblemSourceSlow, s, ps)
		})
	} else {
		res.Sources[ProblemSourceSlow] = ProblemSource{Status: "disabled", Condition: "slow calls are disabled in the detection rules"}
	}
	if rules.Regressions.Enabled {
		run(func() {
			analyses, err := comparisonsFor(ctx, pop)
			if err != nil {
				status, msg := statsStatus(ctx, err)
				add(ProblemSourceComparisons, ProblemSource{Status: status, Condition: msg}, nil)
				return
			}
			ps, eligible := regressionProblems(analyses, rules.Regressions, rules.Revision)
			s := ProblemSource{Status: StatsOK, Condition: fmt.Sprintf("%d stored comparison(s) evaluated", len(analyses))}
			if eligible == 0 {
				mu.Lock()
				res.Waiting = "Waiting for comparable release data: no generated comparison of this service's releases with an eligible p95 overlaps this window. Compare two releases to register a pair; a baseline is never guessed from version text."
				mu.Unlock()
			}
			add(ProblemSourceComparisons, s, ps)
		})
	} else {
		res.Sources[ProblemSourceComparisons] = ProblemSource{Status: "disabled", Condition: "release regressions are disabled in the detection rules"}
	}
	wg.Wait()

	sortProblems(all)
	res.Total = len(all)
	offset, _ := strconv.Atoi(req.Cursor)
	if offset > len(all) {
		offset = len(all)
	}
	end := min(offset+req.Limit, len(all))
	res.Problems = append(res.Problems, all[offset:end]...)
	if end < len(all) {
		res.Next = strconv.Itoa(end)
	}
	res.Status, res.Condition = listStatus(res.Sources, len(all))
	return res
}

// listStatus summarizes the sources: empty only when every enabled source
// answered completely and nothing was found.
func listStatus(sources map[string]ProblemSource, n int) (string, string) {
	var failed, expired []string
	for _, name := range []string{ProblemSourceErrors, ProblemSourceSlow, ProblemSourceComparisons} {
		s, ok := sources[name]
		if !ok {
			continue
		}
		switch s.Status {
		case StatsOK, "disabled":
		case StatsOutsideRetention:
			expired = append(expired, name)
		default:
			failed = append(failed, name)
		}
	}
	enabled := 0
	for _, s := range sources {
		if s.Status != "disabled" {
			enabled++
		}
	}
	switch {
	case enabled > 0 && len(expired) == enabled:
		return "expired", "Summary history is unavailable for this interval (outside retention)."
	case enabled > 0 && len(failed)+len(expired) == enabled:
		return "unavailable", "Problem summaries are unavailable: " + strings.Join(append(failed, expired...), ", ") + "."
	case len(failed)+len(expired) > 0:
		return "partial", "Incomplete results: " + strings.Join(append(failed, expired...), ", ") + " could not be evaluated; their problems are missing, not absent."
	case n == 0:
		return "empty", "No problems observed in the covered scope."
	}
	return "available", "ok"
}

// slowCallProblems groups canonical spans above the slow-call duration by
// operation, span kind and environment. It reads raw spans (bounded by the
// window, the service and the query limits); outside raw retention it is
// unavailable rather than estimated from statistics.
func slowCallProblems(ctx context.Context, pop Population, rule SlowRule, revision int64) (_ []Problem, src ProblemSource) {
	now := clock.Now()
	pol := policyFor(ctx)
	if pop.from() < rawHorizon(now, pol.TracesSeconds) {
		return nil, ProblemSource{Status: StatsOutsideRetention,
			Condition: "slow calls are found in raw spans, and part of this window is outside raw span retention"}
	}
	cutoff := time.Now().UnixNano()
	conds, params := pop.callConditions("")
	conds = append(conds, `"Duration" > `+num(int64(rule.DurationMs*1000)))
	// Only spans with a slow row are candidates: the canonical record of a
	// slow call is itself such a row, and few spans are grouped.
	parts := []interval{{pop.from(), pop.to()}}
	query := `SELECT "Operation", "SpanKind", "Environment", "Version", count(), min("UnixTime"), max("UnixTime")
	FROM (SELECT *, ` + spanDerivedDimensions() + ` FROM (` + statSpans(parts, cutoff, statsConfig.AllowedLateness, `"ServiceName" = {service:String} AND "Duration" > `+num(int64(rule.DurationMs*1000))) + `) WHERE "Eligible")` +
		whereClause(conds) + `
	GROUP BY "Operation", "SpanKind", "Environment", "Version"`
	ctx, span := startQuerySpan(ctx, "SELECT slow calls", query, params)
	var err error
	defer func() { endQuerySpan(span, err) }()
	rows, err := clickhouseDB.Query(withLimits(ctx), query, params...)
	if err != nil {
		status, msg := statsStatus(ctx, err)
		return nil, ProblemSource{Status: status, Condition: msg}
	}
	defer rows.Close()
	type key struct{ op, kind, env string }
	byKey := map[key]*Problem{}
	var order []key
	for rows.Next() {
		var k key
		var version string
		var n uint64
		var first, last int64
		if err = rows.Scan(&k.op, &k.kind, &k.env, &version, &n, &first, &last); err != nil {
			status, msg := statsStatus(ctx, err)
			return nil, ProblemSource{Status: status, Condition: msg}
		}
		p, ok := byKey[k]
		if !ok {
			p = &Problem{ID: problemID(ProblemSlowCalls, pop.Service, k.op, k.kind, k.env), Type: ProblemSlowCalls,
				Title:   k.op + " calls slower than " + strconv.FormatFloat(rule.DurationMs, 'f', -1, 64) + " ms",
				Service: pop.Service, Operation: k.op, SpanKind: k.kind, Environment: k.env,
				Slow:      &SlowCalls{ThresholdMs: rule.DurationMs, Versions: []VersionCount{}},
				FirstSeen: strconv.FormatInt(first, 10), LastSeen: strconv.FormatInt(last, 10)}
			byKey[k] = p
			order = append(order, k)
		}
		p.Slow.Calls += n
		p.Slow.Versions = append(p.Slow.Versions, VersionCount{Version: version, Occurrences: n})
		if f, _ := strconv.ParseInt(p.FirstSeen, 10, 64); first < f {
			p.FirstSeen = strconv.FormatInt(first, 10)
		}
		if l, _ := strconv.ParseInt(p.LastSeen, 10, 64); last > l {
			p.LastSeen = strconv.FormatInt(last, 10)
		}
	}
	if err = rows.Err(); err != nil {
		status, msg := statsStatus(ctx, err)
		return nil, ProblemSource{Status: status, Condition: msg}
	}
	var out []Problem
	for _, k := range order {
		p := byKey[k]
		if p.Slow.Calls < uint64(rule.MinCalls) {
			continue
		}
		p.Qualified = true
		sort.Slice(p.Slow.Versions, func(i, j int) bool { return p.Slow.Versions[i].Version < p.Slow.Versions[j].Version })
		p.Explanation = Explanation{
			RuleRevision: revision,
			Rule:         "Slow calls: an operation with enough distinct calls above the duration opens a problem.",
			Observed:     fmt.Sprintf("%d distinct call(s) longer than %g ms", p.Slow.Calls, rule.DurationMs),
			Threshold:    fmt.Sprintf("at least %d call(s) longer than %g ms in the selected window", rule.MinCalls, rule.DurationMs),
			Basis:        "canonical spans (one per TraceId and SpanId) by start time, per operation, span kind and environment; invalid durations never count",
			Scope:        pop.Label(),
		}
		out = append(out, *p)
	}
	return out, ProblemSource{Status: StatsOK, Condition: "ok"}
}

// MaxComparisonsEvaluated bounds the stored comparisons read for a list.
const MaxComparisonsEvaluated = 200

// comparisonsFor reads the newest stored comparisons of the service whose
// candidate window overlaps the population's window.
func comparisonsFor(ctx context.Context, pop Population) (_ []Analysis, err error) {
	query := `SELECT "Body" FROM analyses
	WHERE "Kind" = 'comparison'
		AND JSONExtractString("Body", 'candidate', 'population', 'service') = {service:String}
		AND toInt64OrZero(JSONExtractString("Body", 'candidate', 'population', 'time_from')) < {to:Int64}
		AND toInt64OrZero(JSONExtractString("Body", 'candidate', 'population', 'time_to')) > {from:Int64}
	ORDER BY "CreatedAt" DESC
	LIMIT ` + strconv.Itoa(MaxComparisonsEvaluated)
	params := []any{clickhouse.Named("service", pop.Service), clickhouse.Named("from", pop.TimeFrom), clickhouse.Named("to", pop.TimeTo)}
	ctx, span := startQuerySpan(ctx, "SELECT comparisons", query, params)
	defer func() { endQuerySpan(span, err) }()
	rows, err := clickhouseDB.Query(withLimits(ctx), query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Analysis
	for rows.Next() {
		var body string
		if err = rows.Scan(&body); err != nil {
			return nil, err
		}
		var a Analysis
		if json.Unmarshal([]byte(body), &a) != nil {
			continue // not a comparison this version can read
		}
		if pop.Environment != nil && deref(a.Candidate.Population.Environment) != *pop.Environment {
			continue
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListServices returns the services with recorded calls in the window, from
// the prepared call statistics and, for hours without them, nothing else:
// it is a picker, not evidence.
func ListServices(ctx context.Context, from, to int64) (_ []string, err error) {
	query := `SELECT DISTINCT "ServiceName" FROM stat_calls
	WHERE "BucketStart" >= toStartOfHour(fromUnixTimestamp64Nano({from:Int64}), 'UTC') AND "BucketStart" < fromUnixTimestamp64Nano({to:Int64}, 'UTC')
	UNION DISTINCT
	SELECT DISTINCT "ServiceName" FROM otel_traces WHERE "UnixTime" >= greatest({from:Int64}, {recent:Int64}) AND "UnixTime" < {to:Int64}
	ORDER BY 1 LIMIT 500`
	recent := clock.Now().Add(-2 * time.Hour).UnixNano()
	params := []any{clickhouse.Named("from", strconv.FormatInt(from, 10)), clickhouse.Named("to", strconv.FormatInt(to, 10)),
		clickhouse.Named("recent", strconv.FormatInt(recent, 10))}
	ctx, span := startQuerySpan(ctx, "SELECT services", query, params)
	defer func() { endQuerySpan(span, err) }()
	rows, err := clickhouseDB.Query(withLimits(ctx), query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err = rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
