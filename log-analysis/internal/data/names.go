package data

import (
	"context"
	"strconv"
	"strings"
)

// The services and span names of a window, for picking exact names in Log
// Analysis. They are read like statistics (plan.go): every whole hour with a
// committed revision from the hourly aggregates (stat_calls; entry operations
// from stat_shapes), every other part of the window (the current hour, the
// window's partial edge hours, hours not summarized yet) from raw spans. So a
// long window doesn't scan raw spans, and an hour is never counted twice.

// SpanNamesRequest is the window of a name list.
type SpanNamesRequest struct {
	TimeFrom string `json:"time_from"`
	TimeTo   string `json:"time_to"`
}

// Validate checks the window like a span search (at most MaxWindow).
func (r *SpanNamesRequest) Validate() error {
	return validateWindow(r.TimeFrom, r.TimeTo)
}

// SpanName is one span name of one service.
type SpanName struct {
	Service string `json:"service"`
	Name    string `json:"name"`
	// Spans counts the span's calls: canonical calls from statistics, stored
	// rows (retries included) from raw spans, so it is approximate.
	Spans uint64 `json:"spans"`
	// Entry: it started an execution (a root span) in the window.
	Entry bool `json:"entry"`
}

// SpanNameList is the answer of POST /api/v1/span-names.
type SpanNameList struct {
	Status      string     `json:"status"` // ok, partial (some hours unavailable) or an error status
	Error       string     `json:"error,omitempty"`
	Names       []SpanName `json:"names"`
	Truncated   bool       `json:"truncated"` // more than MaxSpanNames
	Plan        []PlanPart `json:"plan"`
	Notices     []Notice   `json:"notices"`
	GeneratedAt string     `json:"generatedAt"`
}

// MaxSpanNames bounds the list (sorted by service and name).
const MaxSpanNames = 5000

// SelectSpanNames lists the services and span names of a validated window.
func SelectSpanNames(ctx context.Context, req SpanNamesRequest) (list SpanNameList) {
	now := clock.Now()
	list = SpanNameList{Names: []SpanName{}, Plan: []PlanPart{}, Notices: []Notice{}, GeneratedAt: nanos(now)}
	fail := func(err error) SpanNameList {
		list.Status, list.Error = statsStatus(ctx, err)
		list.Names = []SpanName{}
		return list
	}
	from, _ := strconv.ParseInt(req.TimeFrom, 10, 64)
	to, _ := strconv.ParseInt(req.TimeTo, 10, 64)
	pol := policyFor(ctx)
	revisions, err := loadRevisions(ctx, from, to)
	if err != nil {
		return fail(err)
	}
	plan := planWindow(from, to, revisions, rawHorizon(now, pol.TracesSeconds), statsHorizon(now, pol))
	list.Plan = plan
	list.Notices = append(list.Notices, planNotices(plan)...)
	list.Status = StatsOK
	query := spanNamesQuery(plan)
	if query == "" {
		list.Status = StatsOutsideRetention
		return list
	}
	for _, p := range plan {
		if p.Source == SourceUnavailable {
			list.Status = "partial"
		}
	}

	ctx, span := startQuerySpan(ctx, "SELECT span names", query, nil)
	defer func() { endQuerySpan(span, err) }()
	rows, err := dbFor(ctx).Query(withLimits(ctx), query)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	for rows.Next() {
		var n SpanName
		var entry uint8
		if err = rows.Scan(&n.Service, &n.Name, &n.Spans, &entry); err != nil {
			return fail(err)
		}
		if len(list.Names) == MaxSpanNames {
			list.Truncated = true
			break
		}
		n.Entry = entry == 1
		list.Names = append(list.Names, n)
	}
	if err = rows.Err(); err != nil {
		return fail(err)
	}
	return list
}

// spanNamesQuery merges the plan's stored and raw parts into one row per
// (service, span name); "" when the plan has neither.
func spanNamesQuery(plan []PlanPart) string {
	var sources []string
	if stored := storedRevisions(plan); stored != "" {
		sources = append(sources,
			`SELECT "ServiceName" AS s, "Operation" AS n, sum("Calls") AS c, toUInt8(0) AS e FROM stat_calls WHERE `+stored+` GROUP BY s, n`,
			`SELECT "EntryService" AS s, "EntryOperation" AS n, toUInt64(0) AS c, toUInt8(1) AS e FROM stat_shapes WHERE `+stored+` GROUP BY s, n`)
	}
	if raw := partsBy(plan, SourceRaw); len(raw) > 0 {
		sources = append(sources, `SELECT "ServiceName" AS s, "SpanName" AS n, count() AS c, max(toUInt8("ParentSpanId" = '`+RootParentSpanID+`')) AS e
		FROM otel_traces WHERE `+intervalsCondition(raw)+` GROUP BY s, n`)
	}
	if len(sources) == 0 {
		return ""
	}
	return `SELECT s, n, sum(c), max(e) FROM (` + strings.Join(sources, ` UNION ALL `) + `)
	GROUP BY s, n
	ORDER BY s, n
	LIMIT ` + strconv.Itoa(MaxSpanNames+1)
}

// intervalsCondition selects raw rows whose start lies in one of the half-open
// intervals.
func intervalsCondition(parts []interval) string {
	conds := make([]string, len(parts))
	for i, p := range parts {
		conds[i] = `("UnixTime" >= ` + num(p.From) + ` AND "UnixTime" < ` + num(p.To) + `)`
	}
	return `(` + strings.Join(conds, ` OR `) + `)`
}
