package data

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// The sequence library (shapes-v1) groups executions by shape. An execution
// is a trace whose root span starts in the window; its shape is the entry
// operation (root service and name) and the set of distinct calls in it, a
// call being caller (service, operation) -> callee (service, operation, kind)
// along the recorded parent links. Repeats and order don't change a shape (a
// call with 3 or 5 queries is one shape); a different set of calls does (an
// early failure that skips a call is another shape). A child whose parent
// wasn't recorded is a call from an unknown caller.
//
// Shapes are computed per hour from canonical eligible spans like the other
// statistics and stored by the refresher in stat_shapes under the bucket's
// revision; hours without a stored revision are computed from raw spans.
const ShapesContract = "shapes-v1"

// shapeLookahead is how long after an hour the spans of executions that
// started in it are read: a call starting later is not part of the shape.
const shapeLookahead = 15 * time.Minute

// shapeSamples is how many executions per shape and hour are kept to open a
// representative one; the p50/p95/p99 example is the kept execution whose
// duration is closest to that percentile.
const shapeSamples = 64

// MaxShapes caps the shapes of one library request (most executions first).
const MaxShapes = 500

// traceShapes is one row per execution whose root starts in parts: the root's
// time, duration (µs), entry, whether any span failed, its services and its
// shape.
func traceShapes(parts []interval, cutoff int64, lateness time.Duration) string {
	spanParts := make([]interval, len(parts))
	for i, p := range parts {
		spanParts[i] = interval{p.From, p.To + int64(shapeLookahead)}
	}
	root := `"ParentSpanId" = '` + RootParentSpanID + `'`
	return `SELECT "TraceId", tupleElement(r, 1) AS "RootTime", tupleElement(r, 2) AS "RootDuration",
		tupleElement(r, 3) AS "EntryService", tupleElement(r, 4) AS "EntryOperation", "Failed", "Services",
		arraySort(arrayDistinct(arrayFilter(e -> e != 0,
			arrayMap(s -> if(s.2 = 0, toUInt64(0), cityHash64(arrayFirst(x -> x.1 = s.2, spans).3, s.3, s.4)), spans)))) AS "Edges",
		cityHash64("EntryService", "EntryOperation", "Edges") AS "ShapeId"
	FROM (
		SELECT "TraceId",
			-- One compact tuple per span (id, parent id, node hash, kind): the
			-- per-trace state of an hour's traces has to fit the query memory.
			groupArray(tuple(reinterpretAsUInt64(unhex("SpanId")), reinterpretAsUInt64(unhex("ParentSpanId")),
				cityHash64("ServiceName", "SpanName"), "SpanKind")) AS spans,
			argMinIf(tuple("UnixTime", "Duration", "ServiceName", "SpanName"), "UnixTime", ` + root + `) AS r,
			countIf(` + root + `) AS roots,
			max("StatusCode" = 1) AS "Failed",
			arraySort(groupUniqArray("ServiceName")) AS "Services"
		FROM (` + statSpans(spanParts, cutoff, lateness, "") + `)
		WHERE "Eligible"
		GROUP BY "TraceId"
		HAVING roots > 0
	)
	WHERE ` + windowsCondition(`tupleElement(r, 1)`, parts)
}

// shapeColumns are the columns of stat_shapes after Revision.
const shapeColumns = `"ShapeId", "EntryService", "EntryOperation", "Services", "Calls", "Traces", "ErrorTraces", "InvalidDurations",
	"Durations", "Samples", "Slowest"`

// bucketShapes aggregates the executions of parts per hour and shape: Calls is
// the number of distinct calls, Traces the executions, ErrorTraces those with
// a failed span, Durations the root durations in ms (valid ones only).
func bucketShapes(parts []interval, cutoff int64, lateness time.Duration) string {
	sample := `tuple("RootDuration", "TraceId", "RootTime")`
	return `SELECT toStartOfHour(fromUnixTimestamp64Nano("RootTime"), 'UTC') AS "BucketStart", "ShapeId",
		any("EntryService") AS "EntryService", any("EntryOperation") AS "EntryOperation", any("Services") AS "Services",
		toUInt32(any(length("Edges"))) AS "Calls",
		count() AS "Traces",
		countIf("Failed") AS "ErrorTraces",
		countIf("RootDuration" < 0) AS "InvalidDurations",
		quantilesTDigestIfState(0.5, 0.95, 0.99)("RootDuration" / 1000, "RootDuration" >= 0) AS "Durations",
		groupArraySample(` + strconv.Itoa(shapeSamples) + `, 1)(` + sample + `) AS "Samples",
		argMax(` + sample + `, "RootDuration") AS "Slowest"
	FROM (` + traceShapes(parts, cutoff, lateness) + `)
	GROUP BY "BucketStart", "ShapeId"`
}

// ShapeFilter selects shapes of the library. Service matches any service of
// a shape, Operation the entry operation (case-insensitive substrings).
type ShapeFilter struct {
	TimeFrom   string `json:"time_from"`
	TimeTo     string `json:"time_to"`
	Service    string `json:"service"`
	Operation  string `json:"operation"`
	ErrorsOnly bool   `json:"errors_only"`
}

// ShapeExample is one recorded execution of a shape.
type ShapeExample struct {
	TraceId    string `json:"traceId"`
	Time       string `json:"time"`       // root start, Unix ns
	DurationUs int64  `json:"durationUs"` // root duration
}

// Shape is one row of the library.
type Shape struct {
	ShapeId          string   `json:"shapeId"`
	EntryService     string   `json:"entryService"`
	EntryOperation   string   `json:"entryOperation"`
	Services         []string `json:"services"`
	DistinctCalls    uint32   `json:"distinctCalls"`
	Executions       uint64   `json:"executions"`
	FailedExecutions uint64   `json:"failedExecutions"`
	InvalidDurations uint64   `json:"invalidDurations"`
	// Percentiles of the root duration in ms; null with fewer valid durations
	// than PercentileMinimum asks for.
	Percentiles map[string]*float64 `json:"percentiles"`
	// Examples: the kept execution closest to each shown percentile, and the
	// slowest one.
	Examples map[string]*ShapeExample `json:"examples"`
}

// ShapeLibrary is the answer of the library endpoint.
type ShapeLibrary struct {
	Status      string     `json:"status"` // ok, partial (some hours unavailable) or an error status
	Error       string     `json:"error,omitempty"`
	Shapes      []Shape    `json:"shapes"`
	Truncated   bool       `json:"truncated"` // more than MaxShapes matched
	Plan        []PlanPart `json:"plan"`
	Notices     []Notice   `json:"notices"`
	Contract    string     `json:"contract"`
	GeneratedAt string     `json:"generatedAt"`
}

// shapePlan is planWindow for the library: a stored revision from before
// shapes-v1 has no shapes, so its hour is computed from raw spans while they
// are retained and is unavailable otherwise.
func shapePlan(from, to int64, revisions map[int64]Revision, rawFrom, statsFrom int64) []PlanPart {
	plan := planWindow(from, to, revisions, rawFrom, statsFrom)
	for i, p := range plan {
		if p.Source != SourceStored || strings.Contains(p.Definitions, ShapesContract) {
			continue
		}
		if p.from >= rawFrom {
			plan[i] = PlanPart{From: p.From, To: p.To, Source: SourceRaw, WholeBucket: p.WholeBucket, from: p.from, to: p.to}
		} else {
			plan[i] = PlanPart{From: p.From, To: p.To, Source: SourceUnavailable, WholeBucket: p.WholeBucket, from: p.from, to: p.to,
				Reason: "no execution shapes: these hours were summarized before the sequence library existed and their raw spans have expired"}
		}
	}
	return plan
}

// shapeLibraryQuery merges the stored and raw shapes of a plan, filters them
// and picks the examples. filters use the named parameters of SelectShapes.
func shapeLibraryQuery(plan []PlanPart, cutoff int64, lateness time.Duration, filters []string) string {
	var sources []string
	if stored := storedRevisions(plan); stored != "" {
		sources = append(sources, `SELECT `+shapeColumns+` FROM stat_shapes WHERE `+stored)
	}
	if raw := partsBy(plan, SourceRaw); len(raw) > 0 {
		sources = append(sources, `SELECT `+shapeColumns+` FROM (`+bucketShapes(raw, cutoff, lateness)+`)`)
	}
	where := ""
	if len(filters) > 0 {
		where = ` WHERE ` + strings.Join(filters, " AND ")
	}
	closest := func(i int) string {
		return `arraySort(x -> abs(tupleElement(x, 1) / 1000 - q[` + strconv.Itoa(i) + `]), valid)[1]`
	}
	return `SELECT toString("ShapeId"), "EntryService", "EntryOperation", "Services", "Calls", t, e, invalid, q,
		` + closest(1) + `, ` + closest(2) + `, ` + closest(3) + `, slowest
	FROM (
		SELECT "ShapeId", any("EntryService") AS "EntryService", any("EntryOperation") AS "EntryOperation",
			any("Services") AS "Services", any("Calls") AS "Calls",
			sum("Traces") AS t, sum("ErrorTraces") AS e, sum("InvalidDurations") AS invalid,
			quantilesTDigestIfMerge(0.5, 0.95, 0.99)("Durations") AS q,
			arrayFilter(x -> tupleElement(x, 1) >= 0, groupArrayArray("Samples")) AS valid,
			argMax("Slowest", tupleElement("Slowest", 1)) AS slowest
		FROM (` + strings.Join(sources, ` UNION ALL `) + `)
		GROUP BY "ShapeId"
	)` + where + `
	ORDER BY t DESC, "ShapeId"
	LIMIT ` + strconv.Itoa(MaxShapes+1)
}

// shapeFilters are the WHERE conditions of a filter on the merged shapes and
// their parameters.
func shapeFilters(f ShapeFilter) ([]string, []any) {
	var conds []string
	var params []any
	if f.Service != "" {
		conds = append(conds, `arrayExists(s -> positionCaseInsensitiveUTF8(s, {service:String}) > 0, "Services")`)
		params = append(params, clickhouse.Named("service", f.Service))
	}
	if f.Operation != "" {
		conds = append(conds, `positionCaseInsensitiveUTF8("EntryOperation", {operation:String}) > 0`)
		params = append(params, clickhouse.Named("operation", f.Operation))
	}
	if f.ErrorsOnly {
		conds = append(conds, `e > 0`)
	}
	return conds, params
}

type sampleTuple = []any

// Validate checks the window: Unix ns, from < to, at most MaxStatsWindow.
func (f ShapeFilter) Validate() error {
	from, err1 := strconv.ParseInt(f.TimeFrom, 10, 64)
	to, err2 := strconv.ParseInt(f.TimeTo, 10, 64)
	if err1 != nil || err2 != nil || to <= from || time.Duration(to-from) > MaxStatsWindow {
		return invalid("time_from and time_to must be Unix nanoseconds with time_from < time_to, at most %s apart", MaxStatsWindow)
	}
	return nil
}

// ComputeShapes answers the sequence library for a validated filter.
func ComputeShapes(ctx context.Context, f ShapeFilter) ShapeLibrary {
	now := clock.Now()
	cutoff := time.Now().UnixNano()
	lib := ShapeLibrary{Shapes: []Shape{}, Plan: []PlanPart{}, Notices: []Notice{}, Contract: ShapesContract, GeneratedAt: nanos(now)}
	fail := func(err error) ShapeLibrary {
		lib.Status, lib.Error = statsStatus(ctx, err)
		lib.Shapes = []Shape{}
		return lib
	}
	from, _ := strconv.ParseInt(f.TimeFrom, 10, 64)
	to, _ := strconv.ParseInt(f.TimeTo, 10, 64)
	pol := policyFor(ctx)
	revisions, err := loadRevisions(ctx, from, to)
	if err != nil {
		return fail(err)
	}
	plan := shapePlan(from, to, revisions, rawHorizon(now, pol.TracesSeconds), statsHorizon(now, pol))
	lib.Plan = plan
	lib.Notices = append(lib.Notices, planNotices(plan)...)
	lib.Status = StatsOK
	if len(partsBy(plan, SourceStored)) == 0 && len(partsBy(plan, SourceRaw)) == 0 {
		lib.Status = StatsOutsideRetention
		return lib
	}
	for _, p := range plan {
		if p.Source == SourceUnavailable {
			lib.Status = "partial"
		}
	}

	filters, params := shapeFilters(f)
	query := shapeLibraryQuery(plan, cutoff, statsConfig.AllowedLateness, filters)
	ctx, span := startQuerySpan(ctx, "SELECT shapes", query, params)
	defer func() { endQuerySpan(span, err) }()
	rows, err := clickhouseDB.Query(withRefreshLimits(ctx), query, params...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	for rows.Next() {
		var s Shape
		var q []float64
		var p50, p95, p99, slowest sampleTuple
		var valid uint64
		if err = rows.Scan(&s.ShapeId, &s.EntryService, &s.EntryOperation, &s.Services, &s.DistinctCalls,
			&s.Executions, &s.FailedExecutions, &s.InvalidDurations, &q, &p50, &p95, &p99, &slowest); err != nil {
			return fail(err)
		}
		if len(lib.Shapes) == MaxShapes {
			lib.Truncated = true
			break
		}
		valid = s.Executions - s.InvalidDurations
		s.Percentiles = map[string]*float64{}
		s.Examples = map[string]*ShapeExample{"slowest": exampleOf(slowest)}
		for i, name := range []string{"p50", "p95", "p99"} {
			s.Percentiles[name] = nil
			if valid >= PercentileMinimum[name] && i < len(q) {
				v := q[i]
				s.Percentiles[name] = &v
				s.Examples[name] = exampleOf([]sampleTuple{p50, p95, p99}[i])
			}
		}
		lib.Shapes = append(lib.Shapes, s)
	}
	if err = rows.Err(); err != nil {
		return fail(err)
	}
	return lib
}

// exampleOf turns a (duration µs, TraceId, root time) tuple into an example;
// an empty trace id (no valid sample) is none.
func exampleOf(t sampleTuple) *ShapeExample {
	if len(t) != 3 {
		return nil
	}
	dur, _ := t[0].(int64)
	trace, _ := t[1].(string)
	at, _ := t[2].(int64)
	if trace == "" {
		return nil
	}
	return &ShapeExample{TraceId: trace, Time: strconv.FormatInt(at, 10), DurationUs: dur}
}
