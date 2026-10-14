package data

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Percentile evidence statuses.
const (
	PercentileAvailable    = "available"
	PercentileInsufficient = "insufficient_sample"
	PercentileUnavailable  = "unavailable"
)

// PercentileMinimum is the comparison-v1 sample heuristic: the number of
// valid durations below which a percentile is not shown as a headline value
// nor classified. Meeting it doesn't prove precision.
var PercentileMinimum = map[string]uint64{"p50": 20, "p95": 200, "p99": 1000}

var percentileNames = []string{"p50", "p95", "p99"}

// Percentile is one duration percentile in milliseconds.
type Percentile struct {
	ValueMs *float64 `json:"valueMs"` // nil when unavailable
	Status  string   `json:"status"`
	Minimum uint64   `json:"minimum"`
	Samples uint64   `json:"samples"`
}

// CallStats are the statistics of one (operation, span kind[, version])
// population.
type CallStats struct {
	Operation string  `json:"operation"`
	SpanKind  string  `json:"spanKind"`
	Version   *string `json:"version,omitempty"` // set when grouped by version; "" = not recorded

	Calls  uint64 `json:"calls"`
	Errors uint64 `json:"errors"`
	Ok     uint64 `json:"ok"`
	Unset  uint64 `json:"unset"`
	// ErrorFraction is Errors/Calls (0..1); nil when there are no calls.
	ErrorFraction *float64 `json:"errorFraction"`
	// RatePerMinute is Calls over the whole selected window.
	RatePerMinute float64 `json:"ratePerMinute"`

	ValidDurations   uint64                `json:"validDurations"`
	InvalidDurations uint64                `json:"invalidDurations"`
	Percentiles      map[string]Percentile `json:"percentiles"`

	Conflicts uint64   `json:"conflicts"`
	Sampling  []string `json:"sampling"` // declared policies seen ("" = not declared)
}

// BucketPoint is one bucket of a time series.
type BucketPoint struct {
	From      string  `json:"from"`
	To        string  `json:"to"`
	Source    string  `json:"source"`
	State     string  `json:"state,omitempty"`
	Revision  int64   `json:"revision,omitempty"`
	Coverage  string  `json:"coverage"`
	Calls     *uint64 `json:"calls"` // nil when unavailable (never a zero)
	Errors    *uint64 `json:"errors"`
	Available bool    `json:"available"`
}

// Status of a whole statistics result.
const (
	StatsOK                     = "ok"
	StatsUnavailable            = "unavailable"
	StatsTimeout                = "timeout"
	StatsError                  = "error"
	StatsOutsideRetention       = "outside_retention"
	StatsExactBoundsUnavailable = "exact_bounds_unavailable"
)

// PopulationStats answers a statistics request.
type PopulationStats struct {
	Population      Population         `json:"population"`
	Label           string             `json:"label"`
	DurationSeconds float64            `json:"durationSeconds"`
	Status          string             `json:"status"`
	Condition       string             `json:"condition"` // what the status means for people
	Error           string             `json:"error,omitempty"`
	SuggestedFrom   string             `json:"suggestedFrom,omitempty"`
	SuggestedTo     string             `json:"suggestedTo,omitempty"`
	Rows            []CallStats        `json:"rows"`
	Series          []BucketPoint      `json:"series"`
	Coverage        CoverageAssessment `json:"coverage"`
	Plan            []PlanPart         `json:"plan"`
	Notices         []Notice           `json:"notices"`
	LateExcluded    uint64             `json:"lateExcluded"`
	UnknownReceipt  uint64             `json:"unknownReceipt"`
	Cutoff          string             `json:"cutoff"`      // analysis cutoff: records received later are not included
	GeneratedAt     string             `json:"generatedAt"` // by the application clock
	Definitions     map[string]string  `json:"definitions"`
}

func definitions() map[string]string {
	return map[string]string{
		"metrics":    MetricsContract,
		"canonical":  CanonicalContract,
		"percentile": PercentileMethod,
		"rate":       "recorded calls / full selected window, calls per minute",
		"timeBasis":  "UTC; call membership by span start time; buckets of one hour",
	}
}

// StatsRequest is the body of call-stats.
type StatsRequest struct {
	Population Population `json:"population"`
	// GroupByVersion adds one row per recorded version (and one for "not
	// recorded").
	GroupByVersion bool `json:"group_by_version"`
	// Series adds the per-bucket call counts.
	Series bool `json:"series"`
}

// statsQuery runs the call aggregate over the planned parts. group are
// extra columns (e.g. "Version", "BucketStart").
type callRow struct {
	group       map[string]string
	bucket      int64
	calls       uint64
	errors      uint64
	ok          uint64
	unset       uint64
	valid       uint64
	invalid     uint64
	conflicts   uint64
	percentiles []float32
	sampling    []string
}

func callsUnion(plan []PlanPart, cutoff int64, prefilter string) string {
	var branches []string
	if cond := storedRevisions(plan); cond != "" {
		branches = append(branches, `SELECT `+callColumns+` FROM stat_calls WHERE `+cond)
	}
	if raw := partsBy(plan, SourceRaw); len(raw) > 0 {
		branches = append(branches, `SELECT `+callColumns+` FROM (`+rawCalls(raw, cutoff, statsConfig.AllowedLateness, prefilter)+`)`)
	}
	return strings.Join(branches, " UNION ALL ")
}

func selectCalls(ctx context.Context, pop Population, plan []PlanPart, cutoff int64, groupBy []string) (_ []callRow, err error) {
	union := callsUnion(plan, cutoff, `"ServiceName" = {rawservice:String}`)
	if union == "" {
		return nil, nil
	}
	conds, params := pop.callConditions("")
	params = append(params, namedParam("rawservice", pop.Service))
	groupCols := append([]string{"Operation", "SpanKind"}, groupBy...)
	quoted := make([]string, len(groupCols))
	for i, c := range groupCols {
		quoted[i] = `"` + c + `"`
		if c == "BucketStart" {
			quoted[i] = `toString(toInt64(toUnixTimestamp("BucketStart")))`
		}
	}
	query := `SELECT ` + strings.Join(quoted, ", ") + `,
		sum("Calls"), sum("Errors"), sum("Ok"), sum("Unset"), sum("ValidDurations"), sum("InvalidDurations"), sum("Conflicts"),
		quantilesTDigestIfMerge(0.5, 0.95, 0.99)("Durations"), arraySort(groupUniqArray("Sampling"))
	FROM (` + union + `)` + whereClause(conds) + `
	GROUP BY ` + strings.Join(quoted, ", ") + `
	ORDER BY ` + strings.Join(quoted, ", ")

	ctx, span := startQuerySpan(ctx, "SELECT call statistics", query, params)
	defer func() { endQuerySpan(span, err) }()
	rows, err := clickhouseDB.Query(withLimits(ctx), query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []callRow
	for rows.Next() {
		keys := make([]string, len(groupCols))
		dest := make([]any, 0, len(groupCols)+9)
		for i := range keys {
			dest = append(dest, &keys[i])
		}
		var r callRow
		dest = append(dest, &r.calls, &r.errors, &r.ok, &r.unset, &r.valid, &r.invalid, &r.conflicts, &r.percentiles, &r.sampling)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		r.group = map[string]string{}
		for i, c := range groupCols {
			r.group[c] = keys[i]
		}
		if b, ok := r.group["BucketStart"]; ok {
			r.bucket, _ = strconv.ParseInt(b, 10, 64)
			r.bucket *= 1e9
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (r callRow) stats(window time.Duration) CallStats {
	s := CallStats{
		Operation: r.group["Operation"], SpanKind: r.group["SpanKind"],
		Calls: r.calls, Errors: r.errors, Ok: r.ok, Unset: r.unset,
		ValidDurations: r.valid, InvalidDurations: r.invalid, Conflicts: r.conflicts,
		Sampling: r.sampling,
	}
	if v, ok := r.group["Version"]; ok {
		s.Version = &v
	}
	if s.Sampling == nil {
		s.Sampling = []string{}
	}
	s.finish(window, r.percentiles)
	return s
}

// finish derives fraction, rate and percentile statuses.
func (s *CallStats) finish(window time.Duration, values []float32) {
	if s.Calls > 0 {
		f := float64(s.Errors) / float64(s.Calls)
		s.ErrorFraction = &f
	}
	if window > 0 {
		s.RatePerMinute = float64(s.Calls) / window.Minutes()
	}
	s.Percentiles = map[string]Percentile{}
	for i, name := range percentileNames {
		p := Percentile{Minimum: PercentileMinimum[name], Samples: s.ValidDurations}
		switch {
		case s.ValidDurations == 0:
			p.Status = PercentileUnavailable
		case s.ValidDurations < p.Minimum:
			p.Status = PercentileInsufficient
		default:
			p.Status = PercentileAvailable
		}
		if s.ValidDurations > 0 && i < len(values) && !math.IsNaN(float64(values[i])) {
			v := float64(values[i])
			p.ValueMs = &v
		}
		s.Percentiles[name] = p
	}
}

// ComputeStats answers a statistics request. It never fails as a whole on a
// query error: the status says what is unavailable, so it is never mistaken
// for zero calls.
func ComputeStats(ctx context.Context, req StatsRequest) PopulationStats {
	pop := req.Population
	now := clock.Now()
	cutoff := time.Now().UnixNano()
	pol := policyFor(ctx)
	res := PopulationStats{
		Population:      pop,
		Label:           pop.Label(),
		DurationSeconds: pop.Duration().Seconds(),
		Rows:            []CallStats{},
		Series:          []BucketPoint{},
		Notices:         []Notice{},
		Plan:            []PlanPart{},
		Cutoff:          strconv.FormatInt(cutoff, 10),
		GeneratedAt:     nanos(now),
		Definitions:     definitions(),
		Coverage:        CoverageAssessment{Status: CoverageUnknown, Reasons: []string{}},
	}
	fail := func(err error) PopulationStats {
		res.Status, res.Error = statsStatus(ctx, err)
		res.Condition = conditionOf(res.Status)
		res.Rows = []CallStats{}
		return res
	}

	revisions, err := loadRevisions(ctx, pop.from(), pop.to())
	if err != nil {
		return fail(err)
	}
	plan := planWindow(pop.from(), pop.to(), revisions, rawHorizon(now, pol.TracesSeconds), statsHorizon(now, pol))
	res.Plan = plan
	res.Notices = append(res.Notices, planNotices(plan)...)
	for _, p := range plan {
		if p.Source != SourceUnavailable {
			continue
		}
		if from, to, ok := wholeBucketAlternative(pop.from(), pop.to(), revisions, statsHorizon(now, pol)); ok && strings.HasPrefix(p.Reason, "exact bounds") {
			res.Status = StatsExactBoundsUnavailable
			res.SuggestedFrom, res.SuggestedTo = strconv.FormatInt(from, 10), strconv.FormatInt(to, 10)
			res.Condition = fmt.Sprintf("exact statistics for these bounds are unavailable; whole-hour statistics exist for [%s, %s)",
				time.Unix(0, from).UTC().Format(time.RFC3339), time.Unix(0, to).UTC().Format(time.RFC3339))
			return res
		}
		res.Status = StatsOutsideRetention
		res.Condition = "unavailable: part of the window is outside retained coverage (" + p.Reason + ")"
		return res
	}

	ev, err := loadCoverage(ctx, pop.from(), pop.to())
	if err != nil {
		return fail(err)
	}
	var assessments []CoverageAssessment
	for _, p := range plan {
		assessments = append(assessments, ev.assess(p.from, p.to, "traces", now))
	}
	res.Coverage = combineCoverage(assessments)

	groupBy := []string{}
	if req.GroupByVersion {
		groupBy = append(groupBy, "Version")
	}
	rows, err := selectCalls(ctx, pop, plan, cutoff, groupBy)
	if err != nil {
		return fail(err)
	}
	for _, r := range rows {
		res.Rows = append(res.Rows, r.stats(pop.Duration()))
	}
	if len(res.Rows) == 0 && pop.Operation != nil && pop.SpanKind != nil && !req.GroupByVersion {
		// An explicitly selected population with no calls is a confirmed
		// zero (the query succeeded), not a missing row.
		empty := CallStats{Operation: *pop.Operation, SpanKind: *pop.SpanKind, Sampling: []string{}}
		empty.finish(pop.Duration(), nil)
		res.Rows = append(res.Rows, empty)
	}

	if req.Series {
		series, err := selectCalls(ctx, pop, plan, cutoff, []string{"BucketStart"})
		if err != nil {
			return fail(err)
		}
		res.Series = seriesOf(plan, series, assessments)
	}

	late, unknown, err := receiptNotices(ctx, pop, plan, revisions, cutoff)
	if err != nil {
		return fail(err)
	}
	res.LateExcluded, res.UnknownReceipt = late, unknown
	res.Notices = append(res.Notices, qualityNotices(res)...)

	res.Status = StatsOK
	res.Condition = "ok"
	if len(res.Rows) == 0 || allZero(res.Rows) {
		res.Condition = "no observed calls in this scope"
	}
	return res
}

func allZero(rows []CallStats) bool {
	for _, r := range rows {
		if r.Calls > 0 {
			return false
		}
	}
	return true
}

// seriesOf is one point per planned part; a part without coverage is a gap,
// not a zero.
func seriesOf(plan []PlanPart, rows []callRow, coverage []CoverageAssessment) []BucketPoint {
	byBucket := map[int64][2]uint64{}
	for _, r := range rows {
		v := byBucket[r.bucket]
		byBucket[r.bucket] = [2]uint64{v[0] + r.calls, v[1] + r.errors}
	}
	points := make([]BucketPoint, len(plan))
	for i, p := range plan {
		pt := BucketPoint{From: p.From, To: p.To, Source: p.Source, State: p.State, Revision: p.Revision, Coverage: coverage[i].Status}
		if p.Source != SourceUnavailable && coverage[i].Status != CoverageGap && coverage[i].Status != CoverageUnknown {
			v := byBucket[floorHour(p.from)]
			calls, errors := v[0], v[1]
			pt.Calls, pt.Errors, pt.Available = &calls, &errors, true
		} else if p.Source != SourceUnavailable {
			// Counted records are shown, but a gap is never a confirmed zero.
			if v, ok := byBucket[floorHour(p.from)]; ok && v[0] > 0 {
				calls, errors := v[0], v[1]
				pt.Calls, pt.Errors = &calls, &errors
			}
		}
		points[i] = pt
	}
	return points
}

func qualityNotices(res PopulationStats) []Notice {
	var notices []Notice
	if res.Coverage.Status != CoverageComplete {
		notices = append(notices, Notice{Code: "coverage", Message: "Collection coverage of this window is " + res.Coverage.Status +
			": " + strings.Join(res.Coverage.Reasons, "; ") + ". Counts describe recorded evidence only."})
	}
	var conflicts, invalid uint64
	policies := map[string]bool{}
	for _, r := range res.Rows {
		conflicts += r.Conflicts
		invalid += r.InvalidDurations
		for _, s := range r.Sampling {
			policies[s] = true
		}
	}
	if conflicts > 0 {
		notices = append(notices, Notice{Code: "conflict", Message: fmt.Sprintf(
			"%d call(s) were stored with conflicting payloads for one (TraceId, SpanId). Each counts once, by the %s rule (error status first, then more events, more links, then payload hash); change signals are withheld for affected populations.",
			conflicts, CanonicalContract)})
	}
	if invalid > 0 {
		notices = append(notices, Notice{Code: "invalid_duration", Message: fmt.Sprintf(
			"%d call(s) have an invalid (negative) duration: they are counted as calls but excluded from percentiles.", invalid)})
	}
	sampling := samplingOf(res.Rows)
	switch {
	case sampling == "":
		notices = append(notices, Notice{Code: "sampling", Message: "The sampling policy is unknown (no " + AttrSamplingPolicy +
			" attribute and no SAMPLING_POLICY_DEFAULT): counts are recorded spans, possibly a sample of the traffic."})
	case sampling == "mixed":
		notices = append(notices, Notice{Code: "sampling", Message: "Calls declare different sampling policies: " + strings.Join(sortedKeys(policies), ", ")})
	case sampling != "unsampled" && sampling != "always_on":
		notices = append(notices, Notice{Code: "sampling", Message: "Sampling policy " + sampling +
			": counts are recorded spans and are not extrapolated to all traffic."})
	}
	if res.LateExcluded > 0 {
		notices = append(notices, Notice{Code: "late", Message: fmt.Sprintf(
			"%d record(s) arrived after their bucket's allowed lateness (or after it was finalized) and are excluded from statistics. They were still stored as raw evidence.", res.LateExcluded)})
	}
	if res.UnknownReceipt > 0 {
		notices = append(notices, Notice{Code: "unknown_receipt", Message: fmt.Sprintf(
			"%d call(s) were stored before arrival times were recorded; whether they arrived within the allowed lateness is unknown.", res.UnknownReceipt)})
	}
	return notices
}

// samplingOf is the population's sampling policy: one declared policy, ""
// (unknown) or "mixed". Undeclared spans take SAMPLING_POLICY_DEFAULT.
func samplingOf(rows []CallStats) string {
	policies := map[string]bool{}
	for _, r := range rows {
		if r.Calls == 0 {
			continue
		}
		for _, s := range r.Sampling {
			if s == "" {
				s = statsConfig.DefaultSampling
			}
			policies[s] = true
		}
	}
	switch len(policies) {
	case 0:
		return statsConfig.DefaultSampling
	case 1:
		for s := range policies {
			return s
		}
	}
	if policies[""] {
		return ""
	}
	return "mixed"
}

// receiptNotices counts the population's identities that are not in the
// statistics because they arrived too late (after the bucket's deadline, or
// after its final revision was computed), and those without an arrival time.
func receiptNotices(ctx context.Context, pop Population, plan []PlanPart, revisions map[int64]Revision, cutoff int64) (_, _ uint64, err error) {
	var buckets, limits []string
	var parts []interval
	for _, p := range plan {
		if p.Source == SourceUnavailable {
			continue
		}
		parts = append(parts, interval{p.from, p.to})
		limit := cutoff
		if p.Source == SourceStored {
			limit = min(cutoff, revisions[floorHour(p.from)].ReceiptCutoff)
		}
		buckets = append(buckets, num(floorHour(p.from)))
		limits = append(limits, num(limit))
	}
	if len(parts) == 0 {
		return 0, 0, nil
	}
	conds, params := pop.callConditions("")
	deadline := `least(transform(intDiv("UnixTime", ` + num(hourNs) + `) * ` + num(hourNs) + `, CAST([` + strings.Join(buckets, ", ") + `] AS Array(Int64)), CAST([` + strings.Join(limits, ", ") + `] AS Array(Int64)), toInt64(` + num(cutoff) + `)), intDiv("UnixTime", ` + num(hourNs) + `) * ` + num(hourNs) + ` + ` + num(hourNs+int64(statsConfig.AllowedLateness)) + `)`
	query := `SELECT countIf(first >= limit AND first != 0 AND first < ` + num(cutoff) + `), countIf(first = 0)
	FROM (
		SELECT "TraceId", "SpanId", min(r_received) AS first, any(r_limit) AS limit,
			any(r_service) AS "ServiceName", any(r_operation) AS "Operation", any(r_kind) AS "SpanKind", any(r_env) AS "Environment",
			any(r_version) AS "Version", any(r_installation) AS "Installation", any(r_segment) AS "Segment"
		FROM (
			SELECT "TraceId", "SpanId", "ReceivedTime" AS r_received, ` + deadline + ` AS r_limit,
				"ServiceName" AS r_service, "SpanName" AS r_operation, "SpanKind" AS r_kind,
				if(` + spanAttr(AttrEnvironment) + ` != '', ` + spanAttr(AttrEnvironment) + `, ` + spanAttr(AttrEnvironmentOld) + `) AS r_env,
				` + spanAttr(AttrServiceVersion) + ` AS r_version, ` + spanAttr(AttrInstallation) + ` AS r_installation,
				` + spanAttr(AttrWorkloadSegment) + ` AS r_segment
			FROM otel_traces
			WHERE ` + windowsCondition(`"UnixTime"`, parts) + ` AND "ServiceName" = {service:String}
			-- Only a span with a row received at or after its limit, or
			-- without a receipt time, can count; grouping just those keeps
			-- the notice from grouping every span of the window.
			AND ("TraceId", "SpanId") IN (SELECT "TraceId", "SpanId" FROM otel_traces
				WHERE ` + windowsCondition(`"UnixTime"`, parts) + ` AND "ServiceName" = {service:String}
				AND ("ReceivedTime" >= ` + deadline + ` OR "ReceivedTime" = 0))
		)
		GROUP BY "TraceId", "SpanId"
	)` + whereClause(conds)
	ctx, span := startQuerySpan(ctx, "SELECT late calls", query, params)
	defer func() { endQuerySpan(span, err) }()
	var late, unknown uint64
	err = clickhouseDB.QueryRow(withLimits(ctx), query, params...).Scan(&late, &unknown)
	return late, unknown, err
}

// statsStatus maps a query error to a status and message.
func statsStatus(ctx context.Context, err error) (string, string) {
	status, msg := classify(ctx, err)
	switch status {
	case SectionLimit:
		return StatsTimeout, msg
	case SectionUnavailable:
		return StatsUnavailable, msg
	}
	return StatsError, msg
}

func conditionOf(status string) string {
	switch status {
	case StatsTimeout:
		return "statistics query timed out"
	case StatsUnavailable:
		return "the statistics source is unavailable"
	case StatsError:
		return "the statistics query failed"
	}
	return status
}

func sortCallStats(rows []CallStats) {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Operation != b.Operation {
			return a.Operation < b.Operation
		}
		return a.SpanKind < b.SpanKind
	})
}
