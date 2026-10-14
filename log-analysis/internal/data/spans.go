package data

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// RootParentSpanID marks root spans in otel_traces.
const RootParentSpanID = "0000000000000000"

// Scopes of a span search without parent_id.
const (
	ScopeRoot = "root" // root spans only (the default without trace_id)
	ScopeAll  = "all"  // spans at any depth (the default with trace_id)
)

// SpanFilter is the body of view-spans and count-spans.
//
// A logical span is identified by (TraceId, SpanId): span ids are unique only
// within a trace. Every request names a half-open time window
// [TimeFrom, TimeTo) in Unix nanoseconds; lists, counts and child counts only
// see spans that start inside it. The cursor (After*) is the last row of the
// previous page; it moves within the window and never changes it.
type SpanFilter struct {
	TimeFrom    string `json:"time_from"`
	TimeTo      string `json:"time_to"`
	Scope       string `json:"scope"`
	TraceId     string `json:"trace_id"`
	ParentId    string `json:"parent_id"` // requires TraceId
	Status      string `json:"status"`
	ServiceName string `json:"service_name"`
	MethodName  string `json:"method_name"`
	RowsPerPage int    `json:"rows_per_page"`

	AfterTime    string `json:"after_time"`
	AfterTraceId string `json:"after_trace_id"`
	AfterSpanId  string `json:"after_span_id"`
}

const (
	DefaultRowsPerPage = 25
	// MaxRowsPerPage caps a span page, so one request can't read the whole table.
	MaxRowsPerPage = 1000
)

// LimitRows sets RowsPerPage to DefaultRowsPerPage when it is missing and caps
// it at MaxRowsPerPage.
func (f *SpanFilter) LimitRows() {
	if f.RowsPerPage <= 0 {
		f.RowsPerPage = DefaultRowsPerPage
	}
	if f.RowsPerPage > MaxRowsPerPage {
		f.RowsPerPage = MaxRowsPerPage
	}
}

// ErrInvalidFilter is wrapped by every Validate error; the API answers 400.
var ErrInvalidFilter = errors.New("invalid filter")

var (
	traceIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	spanIDPattern  = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidFilter, fmt.Sprintf(format, args...))
}

// Validate checks the filter, lower-cases ids, fills in the default scope and
// caps RowsPerPage.
func (f *SpanFilter) Validate() error {
	if err := validateWindow(f.TimeFrom, f.TimeTo); err != nil {
		return err
	}

	f.TraceId = strings.ToLower(f.TraceId)
	f.ParentId = strings.ToLower(f.ParentId)
	f.AfterTraceId = strings.ToLower(f.AfterTraceId)
	f.AfterSpanId = strings.ToLower(f.AfterSpanId)

	if f.TraceId != "" && !traceIDPattern.MatchString(f.TraceId) {
		return invalid("trace_id must be 32 hex digits")
	}
	if f.ParentId != "" {
		if f.TraceId == "" {
			return invalid("parent_id needs trace_id: span ids are unique only within a trace")
		}
		if !spanIDPattern.MatchString(f.ParentId) {
			return invalid("parent_id must be 16 hex digits")
		}
	}

	switch f.Scope {
	case "":
		f.Scope = ScopeRoot
		if f.TraceId != "" {
			f.Scope = ScopeAll
		}
	case ScopeRoot, ScopeAll:
	default:
		return invalid("scope must be %q or %q", ScopeRoot, ScopeAll)
	}

	if f.Status != "" && StatusCodeFromString(f.Status) == "" {
		return invalid("unknown status %q", f.Status)
	}

	if f.AfterTime != "" || f.AfterTraceId != "" || f.AfterSpanId != "" {
		if _, err := strconv.ParseInt(f.AfterTime, 10, 64); err != nil {
			return invalid("after_time must be Unix nanoseconds")
		}
		if !traceIDPattern.MatchString(f.AfterTraceId) || !spanIDPattern.MatchString(f.AfterSpanId) {
			return invalid("the cursor needs after_time, after_trace_id and after_span_id")
		}
	}

	f.LimitRows()
	return nil
}

func (f *SpanFilter) hasCursor() bool {
	return f.AfterTime != ""
}

// conditions returns the WHERE conditions of a validated filter (the time
// window is added separately) and the parameters they use.
func (f *SpanFilter) conditions(withCursor bool) ([]string, []any) {
	var conds []string
	var params []any
	add := func(cond string, named ...any) {
		conds = append(conds, cond)
		params = append(params, named...)
	}

	if f.TraceId != "" {
		add(`"TraceId" = {traceId:String}`, clickhouse.Named("traceId", f.TraceId))
	}
	switch {
	case f.ParentId != "":
		add(`"ParentSpanId" = {parentId:String}`, clickhouse.Named("parentId", f.ParentId))
	case f.Scope == ScopeRoot:
		add(`"ParentSpanId" = '` + RootParentSpanID + `'`)
	}
	if f.Status != "" {
		add(`"StatusCode" = {statusCode:Int8}`, clickhouse.Named("statusCode", StatusCodeFromString(f.Status)))
	}
	if f.ServiceName != "" {
		add(`"ServiceName" ILIKE {serviceName:String}`, clickhouse.Named("serviceName", "%"+f.ServiceName+"%"))
	}
	if f.MethodName != "" {
		add(`"SpanName" ILIKE {spanName:String}`, clickhouse.Named("spanName", "%"+f.MethodName+"%"))
	}
	if withCursor && f.hasCursor() {
		add(`("UnixTime", "TraceId", "SpanId") > (toInt64({afterTime:Int64}), {afterTraceId:String}, {afterSpanId:String})`,
			clickhouse.Named("afterTime", f.AfterTime),
			clickhouse.Named("afterTraceId", f.AfterTraceId),
			clickhouse.Named("afterSpanId", f.AfterSpanId))
	}
	return conds, params
}

func (f *SpanFilter) windowParams() []any {
	return []any{clickhouse.Named("timeFrom", f.TimeFrom), clickhouse.Named("timeTo", f.TimeTo)}
}

// windowCondition is the half-open window [time_from, time_to): a span starting
// exactly at time_to belongs to the next window, so adjacent windows never
// share a record.
const windowCondition = `"UnixTime" >= toInt64({timeFrom:Int64}) AND "UnixTime" < toInt64({timeTo:Int64})`

// spanColumns are the stored columns a canonical record carries.
const spanColumns = `"UnixTime", "TraceId", "SpanId", "ParentSpanId", "TraceState", "SpanName", "SpanKind", "ServiceName",
	"ResourceAttributes", "SpanAttributes", "Duration", "StatusCode", "StatusMessage",
	"Events.Timestamp", "Events.Name", "Events.Attributes",
	"Links.TraceId", "Links.SpanId", "Links.TraceState", "Links.Attributes"`

// winnerOrder ranks the stored rows of one (TraceId, SpanId), best first.
// It is the canonical-record rule: exact retries are identical rows, so any of
// them represents the span; for conflicting payloads the whole record that
// ranks highest wins, never a mix of fields:
//  1. a row with error status (recorded failure evidence is never hidden),
//  2. then more events, 3. then more links,
//  4. then the larger hash of every non-identity column.
//
// The hash makes the choice independent of arrival order and storage order.
// It leaves out TraceId and SpanId, so the same pair of payloads resolves the
// same way in every trace.
const winnerOrder = `"StatusCode" = 1 DESC, length("Events.Name") DESC, length("Links.SpanId") DESC,
	cityHash64(` + spanPayloadColumns + `) DESC`

// spanPayloadColumns are the non-identity columns of a span row, with maps
// sorted: the receiver writes attribute maps in no particular order, so an
// exact retry can store the same map with its keys in another order.
const spanPayloadColumns = `"UnixTime", "ParentSpanId", "TraceState", "SpanName", "SpanKind", "ServiceName",
		mapSort("ResourceAttributes"), mapSort("SpanAttributes"), "Duration", "StatusCode", "StatusMessage",
		"Events.Timestamp", "Events.Name", arrayMap(m -> mapSort(m), "Events.Attributes"),
		"Links.TraceId", "Links.SpanId", "Links.TraceState", arrayMap(m -> mapSort(m), "Links.Attributes"), "ChildSpanCount"`

// canonicalSpans is a subquery with one canonical record per (TraceId, SpanId)
// for the spans inside the window that may match conds. The canonical record
// is chosen among all rows of the span in the window before conds are applied,
// so a superseded row can't be selected by a filter. The IN pre-selection only
// narrows the work: the canonical row is one of the span's rows, so if it
// matches conds, some row of that span matched them too. Callers apply conds
// again to the result.
func canonicalSpans(conds []string) string {
	preselect := ""
	if len(conds) > 0 {
		preselect = ` AND ("TraceId", "SpanId") IN (SELECT "TraceId", "SpanId" FROM otel_traces WHERE ` +
			windowCondition + ` AND ` + strings.Join(conds, " AND ") + `)`
	}
	return `SELECT ` + spanColumns + ` FROM otel_traces WHERE ` + windowCondition + preselect + `
	ORDER BY "TraceId", "SpanId", ` + winnerOrder + `
	LIMIT 1 BY "TraceId", "SpanId"`
}

// pageCandidates is the first {scanRows:UInt32} rows matching conds (with the
// cursor) in time order. It orders by "UnixTime" alone, the table's sorting
// key, with a plain LIMIT, so ClickHouse reads forward from the cursor and
// stops early instead of reading the rest of the window (WITH TIES or more
// sort columns make it read everything). Every matching row that starts
// before the last candidate's time is among the candidates.
func pageCandidates(conds []string) string {
	where := windowCondition
	if len(conds) > 0 {
		where += ` AND ` + strings.Join(conds, " AND ")
	}
	return `SELECT "UnixTime", "TraceId", "SpanId" FROM otel_traces WHERE ` + where + `
	ORDER BY "UnixTime" LIMIT {scanRows:UInt32}`
}

// pageCanonicalSpans is canonicalSpans for one page: one canonical record per
// candidate span, chosen among the span's rows in [{pageFrom:Int64},
// {pageTo:Int64}) before conds apply. selectSpans ends that range at the last
// candidate time (rows at that time may be cut off by the LIMIT) unless the
// candidates ran out. Retries and replays repeat a span with its start time,
// so all its rows are in that range; only copies of one span recorded with
// different start times are not compared (the whole-window rule of
// canonicalSpans would need a scan of the window per page).
func pageCanonicalSpans(conds []string) string {
	return `SELECT ` + spanColumns + ` FROM otel_traces WHERE ` + windowCondition + `
	AND "UnixTime" >= toInt64({pageFrom:Int64}) AND "UnixTime" < toInt64({pageTo:Int64})
	AND ("TraceId", "SpanId") IN (SELECT "TraceId", "SpanId" FROM (` + pageCandidates(conds) + `))
	ORDER BY "TraceId", "SpanId", ` + winnerOrder + `
	LIMIT 1 BY "TraceId", "SpanId"`
}

func whereClause(conds []string) string {
	if len(conds) == 0 {
		return ""
	}
	return ` WHERE ` + strings.Join(conds, " AND ")
}

type Event struct {
	TimeUnixNano string            `json:"timeUnixNano"`
	Name         string            `json:"name"`
	Attributes   map[string]string `json:"attributes"`
}

type Link struct {
	TraceId    string            `json:"traceId"`
	SpanId     string            `json:"spanId"`
	TraceState string            `json:"traceState"`
	Attributes map[string]string `json:"attributes"`
}

type Span struct {
	Timestamp string `json:"timeStamp"`
	SpanName  string `json:"name"`
	// Kind is the recorded span kind (server, client, producer, consumer,
	// internal, unspecified), as the receiver stored it; "" for older rows.
	Kind           string              `json:"spanKind"`
	ServiceName    string              `json:"service"`
	TraceId        string              `json:"traceId"`
	SpanId         string              `json:"spanId"`
	ParentSpanId   string              `json:"parentSpanId"`
	Msg            string              `json:"msg"`
	Tags           []map[string]string `json:"tags"`
	ServiceTags    []map[string]string `json:"serviceTags"`
	ChildSpanCount int32               `json:"childSpanCount"`
	StatusCode     int8                `json:"statusCode"`
	Status         string              `json:"status"`
	StatusMessage  string              `json:"statusMessage"`
	Duration       int64               `json:"duration"` // microseconds
	Events         []Event             `json:"events"`
	Links          []Link              `json:"links"`
}

// SpanPage is one page of spans. HasMore says whether more rows match after
// the last one (it is checked by reading one extra row, not guessed from the
// page size).
type SpanPage struct {
	Spans   []*Span
	HasMore bool
}

// SelectSpans returns one page of canonical spans matching a validated filter,
// in (UnixTime, TraceId, SpanId) order, with child counts within the window.
func (l *Log) SelectSpans(ctx context.Context, filter SpanFilter) (SpanPage, error) {
	page, err := selectSpans(ctx, filter)
	if err != nil {
		return SpanPage{}, err
	}
	if err := fillChildSpanCounts(ctx, filter, page.Spans); err != nil {
		return SpanPage{}, err
	}
	return page, nil
}

// pageScans are the candidate row counts of the attempts at one page, as
// multiples of the rows it needs (RowsPerPage+1). Candidates can be fewer
// spans than rows (retried copies) or have a superseded canonical record that
// no longer matches; then the next attempt reads more, and after the last one
// the page falls back to canonicalSpans over the whole window.
var pageScans = []int{2, 16}

// selectSpans reads one page: the next candidates in time order, then their
// canonical records before the last candidate time. It is exact when the
// candidates ran out or when the page is full: every matching span that isn't
// a candidate starts at or after the last candidate time, so after every span
// on the page.
func selectSpans(ctx context.Context, filter SpanFilter) (SpanPage, error) {
	conds, params := filter.conditions(true)
	params = append(params, filter.windowParams()...)
	want := filter.RowsPerPage + 1

	for _, factor := range pageScans {
		scan := factor * want
		scanParams := append(params[:len(params):len(params)], clickhouse.Named("scanRows", strconv.Itoa(scan)))
		from, last, candidates, err := pageRange(ctx, conds, scanParams)
		if err != nil {
			return SpanPage{}, err
		}
		if candidates == 0 {
			return SpanPage{}, nil
		}
		ranOut := candidates < uint64(scan)
		to := last
		if ranOut {
			to++ // every matching row is a candidate, the last time included
		}
		pageParams := append(scanParams, clickhouse.Named("pageFrom", strconv.FormatInt(from, 10)),
			clickhouse.Named("pageTo", strconv.FormatInt(to, 10)))
		page, read, err := querySpanPage(ctx, filter, pageCanonicalSpans(conds), conds, pageParams)
		if err != nil || ranOut || read == want {
			return page, err
		}
	}
	page, _, err := querySpanPage(ctx, filter, canonicalSpans(conds), conds, params)
	return page, err
}

// pageRange returns the first and last candidate time of a page and the number
// of candidate rows (fewer than scanRows: the window has no more).
func pageRange(ctx context.Context, conds []string, params []any) (first, last int64, candidates uint64, err error) {
	query := `SELECT min("UnixTime"), max("UnixTime"), count() FROM (` + pageCandidates(conds) + `)`

	ctx, dbSpan := startQuerySpan(ctx, "SELECT page otel_traces", query, params)
	defer func() { endQuerySpan(dbSpan, err) }()

	err = clickhouseDB.QueryRow(withLimits(ctx), query, params...).Scan(&first, &last, &candidates)
	return first, last, candidates, err
}

// querySpanPage reads up to RowsPerPage+1 canonical records of source that
// match conds, in (UnixTime, TraceId, SpanId) order, and returns how many rows
// it read.
func querySpanPage(ctx context.Context, filter SpanFilter, source string, conds []string, params []any) (_ SpanPage, read int, err error) {
	params = append(params[:len(params):len(params)], clickhouse.Named("limit", strconv.Itoa(filter.RowsPerPage+1)))

	query := `SELECT toString("UnixTime"), "SpanName", "SpanKind", "ServiceName", "TraceId", "SpanId", "ParentSpanId",
	 arrayMap(key -> map('key', key, 'value', SpanAttributes[key]), mapKeys(SpanAttributes)) AS tags,
	 arrayMap(key -> map('key', key, 'value', ResourceAttributes[key]), mapKeys(ResourceAttributes)) AS serviceTags,
	 "StatusCode", "StatusMessage", "Duration",
	 "Events.Timestamp", "Events.Name", "Events.Attributes",
	 "Links.TraceId", "Links.SpanId", "Links.TraceState", "Links.Attributes"
	FROM (` + source + `)` + whereClause(conds) + `
	ORDER BY "UnixTime", "TraceId", "SpanId" LIMIT {limit:UInt32}`

	ctx, dbSpan := startQuerySpan(ctx, "SELECT otel_traces", query, params)
	defer func() { endQuerySpan(dbSpan, err) }()

	rows, err := clickhouseDB.Query(withLimits(ctx), query, params...)
	if err != nil {
		return SpanPage{}, 0, err
	}
	defer rows.Close()

	var page SpanPage
	for rows.Next() {
		var span Span
		var eventTimes []time.Time
		var eventNames []string
		var eventAttrs []map[string]string
		var linkTraces, linkSpans, linkStates []string
		var linkAttrs []map[string]string
		err := rows.Scan(
			&span.Timestamp,
			&span.SpanName,
			&span.Kind,
			&span.ServiceName,
			&span.TraceId,
			&span.SpanId,
			&span.ParentSpanId,
			&span.Tags,
			&span.ServiceTags,
			&span.StatusCode,
			&span.StatusMessage,
			&span.Duration,
			&eventTimes, &eventNames, &eventAttrs,
			&linkTraces, &linkSpans, &linkStates, &linkAttrs,
		)
		if err != nil {
			return SpanPage{}, 0, err
		}
		if len(page.Spans) == filter.RowsPerPage {
			page.HasMore = true
			break
		}
		span.Events = zipEvents(eventTimes, eventNames, eventAttrs)
		span.Links = zipLinks(linkTraces, linkSpans, linkStates, linkAttrs)
		span.Msg = statementOf(span.Tags)
		span.Status = StatusCodeToString(strconv.Itoa(int(span.StatusCode)))
		page.Spans = append(page.Spans, &span)
	}
	if err := rows.Err(); err != nil {
		return SpanPage{}, 0, err
	}
	read = len(page.Spans)
	if page.HasMore {
		read++
	}
	return page, read, nil
}

// zipEvents turns the parallel Events.* arrays into events. The receiver
// stores them with equal lengths (ClickHouse Nested).
func zipEvents(times []time.Time, names []string, attrs []map[string]string) []Event {
	events := make([]Event, len(times))
	for i, ts := range times {
		events[i] = Event{TimeUnixNano: strconv.FormatInt(ts.UnixNano(), 10), Attributes: map[string]string{}}
		if i < len(names) {
			events[i].Name = names[i]
		}
		if i < len(attrs) && attrs[i] != nil {
			events[i].Attributes = attrs[i]
		}
	}
	return events
}

func zipLinks(traces, spans, states []string, attrs []map[string]string) []Link {
	links := make([]Link, len(traces))
	for i, trace := range traces {
		links[i] = Link{TraceId: trace, Attributes: map[string]string{}}
		if i < len(spans) {
			links[i].SpanId = spans[i]
		}
		if i < len(states) {
			links[i].TraceState = states[i]
		}
		if i < len(attrs) && attrs[i] != nil {
			links[i].Attributes = attrs[i]
		}
	}
	return links
}

// statementOf returns db.statement with its recorded arguments filled in:
// db.sql.args.N for $N placeholders and db.query.parameter.<name> for
// ClickHouse {name:Type} placeholders.
func statementOf(tags []map[string]string) string {
	var statement string
	sqlArgs := make(map[int]string)
	namedParams := make(map[string]string)
	for _, tag := range tags {
		key := tag["key"]
		if key == "db.statement" {
			statement = tag["value"]
		}
		if strings.HasPrefix(key, queryParamPrefix) {
			namedParams[key[len(queryParamPrefix):]] = tag["value"]
		}
		if strings.HasPrefix(key, "db.sql.args.") {
			argNum := strings.TrimSuffix(key[len("db.sql.args."):], ".")
			var argNumInt int
			fmt.Sscanf(argNum, "%d", &argNumInt)
			sqlArgs[argNumInt] = tag["value"]
		}
	}
	if len(statement) > 0 && len(sqlArgs) > 0 {
		statement = fillSQLArgs(statement, sqlArgs)
	}
	if len(statement) > 0 && len(namedParams) > 0 {
		statement = fillNamedParams(statement, namedParams)
	}
	return statement
}

// isChildOf matches rows whose (TraceId, ParentSpanId) is one of {parents}.
const isChildOf = `("TraceId", "ParentSpanId") IN {parents:Array(Tuple(String, String))}`

// childGroups is one row per (TraceId, parent) with the number of child spans
// whose copies all name that parent; children whose copies name different
// parents are listed under an empty parent (their SpanIds), for the canonical
// rule.
// A span's copies share its start time, so the copies of the children are
// read by the children's start times (the table's sorting key) instead of
// ranking whole rows: when every copy names the same parent, the winner does
// too, whichever it is.
const childGroups = `SELECT "TraceId", parent, toInt32(count()), groupArrayIf("SpanId", parent = '')
	FROM (SELECT "TraceId", "SpanId", if(min("ParentSpanId") = max("ParentSpanId"), min("ParentSpanId"), '') AS parent,
			max(` + isChildOf + `) AS child
		FROM otel_traces WHERE ` + windowCondition + `
		AND "UnixTime" IN (SELECT "UnixTime" FROM otel_traces WHERE ` + windowCondition + ` AND ` + isChildOf + `)
		GROUP BY "UnixTime", "TraceId", "SpanId")
	WHERE child GROUP BY "TraceId", parent`

// fillChildSpanCounts sets ChildSpanCount to the number of canonical children
// of each span, (TraceId, ParentSpanId) = the span's identity, inside the
// filter's window. The stored column isn't used: OTLP doesn't send it and
// log-sender's misses children from other services. The UI shows the expand
// arrow from it, and expanding a span shows exactly these children.
func fillChildSpanCounts(ctx context.Context, filter SpanFilter, spans []*Span) (err error) {
	if len(spans) == 0 {
		return nil
	}
	parents := make([][2]string, len(spans))
	for i, span := range spans {
		parents[i] = [2]string{span.TraceId, span.SpanId}
	}
	params := append(filter.windowParams(), clickhouse.Named("parents", pairsLiteral(parents)))

	counts := make(map[[2]string]int32, len(spans))
	var disputed [][2]string
	err = queryChildCounts(ctx, "SELECT children otel_traces", childGroups, params, func(rows driver.Rows) error {
		var trace, parent string
		var count int32
		var spanIDs []string
		if err := rows.Scan(&trace, &parent, &count, &spanIDs); err != nil {
			return err
		}
		if parent == "" {
			for _, id := range spanIDs {
				disputed = append(disputed, [2]string{trace, id})
			}
			return nil
		}
		counts[[2]string{trace, parent}] = count
		return nil
	})
	if err != nil {
		return err
	}

	// Children whose copies name different parents count under the parent of
	// their canonical record.
	if len(disputed) > 0 {
		conds := []string{`("TraceId", "SpanId") IN {disputed:Array(Tuple(String, String))}`}
		query := `SELECT "TraceId", "ParentSpanId", toInt32(count()) FROM (` + canonicalSpans(conds) + `)
		WHERE ` + isChildOf + ` GROUP BY "TraceId", "ParentSpanId"`
		disputedParams := append(params[:len(params):len(params)], clickhouse.Named("disputed", pairsLiteral(disputed)))
		err = queryChildCounts(ctx, "SELECT disputed children otel_traces", query, disputedParams, func(rows driver.Rows) error {
			var trace, parent string
			var count int32
			if err := rows.Scan(&trace, &parent, &count); err != nil {
				return err
			}
			counts[[2]string{trace, parent}] += count
			return nil
		})
		if err != nil {
			return err
		}
	}

	for _, s := range spans {
		s.ChildSpanCount = counts[[2]string{s.TraceId, s.SpanId}]
	}
	return nil
}

func queryChildCounts(ctx context.Context, name, query string, params []any, scan func(driver.Rows) error) (err error) {
	ctx, span := startQuerySpan(ctx, name, query, params)
	defer func() { endQuerySpan(span, err) }()

	rows, err := clickhouseDB.Query(withLimits(ctx), query, params...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// maxSharedTimes is how many shared start times countSpans passes to its second
// query as a list; with more (after a mass replay, say) it reads them again
// with a subquery instead.
const maxSharedTimes = 20000

// matchAll is conds joined into one condition, or 1 without conditions.
func matchAll(conds []string) string {
	if len(conds) == 0 {
		return "1"
	}
	return strings.Join(conds, " AND ")
}

// timeGroups is one row per time of a matching row of table in the window:
// how many rows have that time (n) and how many of them match conds (s).
// Grouping only the matching rows' times keeps a selective filter cheap and
// the hash table small (grouping every start time of a day needed ~0.8 GiB).
func timeGroups(table string, conds []string) string {
	if len(conds) == 0 {
		return `SELECT "UnixTime" AS t, count() AS n, n AS s FROM ` + table + ` WHERE ` + windowCondition + ` GROUP BY t`
	}
	match := matchAll(conds)
	return `SELECT "UnixTime" AS t, count() AS n, sum(` + match + `) AS s FROM ` + table + ` WHERE ` + windowCondition + `
	AND "UnixTime" IN (SELECT "UnixTime" FROM ` + table + ` WHERE ` + windowCondition + ` AND ` + match + `) GROUP BY t`
}

// sharedTimeSpans counts the canonical spans matching conds among the rows
// whose start time is shared (times is an IN operand); the winner is chosen
// by the canonical rule (winnerOrder as a tuple) before conds apply.
func sharedTimeSpans(conds []string, times string) string {
	return `SELECT countIf(w) FROM (SELECT argMax(` + matchAll(conds) + `,
		tuple("StatusCode" = 1, length("Events.Name"), length("Links.SpanId"), ` + spanPayloadHash + `)) AS w
	FROM otel_traces WHERE ` + windowCondition + ` AND "UnixTime" IN ` + times + `
	GROUP BY "UnixTime", "TraceId", "SpanId")`
}

// SelectCountSpans counts the canonical spans matching a validated filter in
// its window; the cursor is ignored, so it is the total of the whole list.
func (l *Log) SelectCountSpans(ctx context.Context, filter SpanFilter) (uint64, error) {
	conds, params := filter.conditions(false)
	params = append(params, filter.windowParams()...)
	return countByTime(ctx, "otel_traces", conds, params, sharedTimeSpans)
}

// countByTime counts the canonical records of table matching conds in the
// window. Retries and replays repeat a record with its time, so a row whose
// time no other row in the window has is its record's only row and counts
// when it matches. Only rows at shared times are grouped per record and
// ranked (shared builds that query for an IN operand of times). That reads one
// narrow column pass instead of canonicalizing every row of the window, which
// took seconds on millions of rows. Like the lists (pageCanonicalSpans,
// logPageCanonical), copies of one record stored with different times count
// separately.
func countByTime(ctx context.Context, table string, conds []string, params []any, shared func(conds []string, times string) string) (_ uint64, err error) {
	query := `SELECT sumIf(s, n = 1), countIf(n > 1), groupArrayIf(` + strconv.Itoa(maxSharedTimes) + `)(toString(t), n > 1)
	FROM (` + timeGroups(table, conds) + `)`

	ctx, span := startQuerySpan(ctx, "SELECT count "+table, query, params)
	defer func() { endQuerySpan(span, err) }()

	var single, sharedCount uint64
	var sharedTimes []string
	if err = clickhouseDB.QueryRow(withLimits(ctx), query, params...).Scan(&single, &sharedCount, &sharedTimes); err != nil {
		return 0, err
	}
	if sharedCount == 0 {
		return single, nil
	}

	times := `(SELECT t FROM (` + timeGroups(table, conds) + `) WHERE n > 1)`
	sharedParams := params
	if sharedCount <= maxSharedTimes {
		times = `{sharedTimes:Array(Int64)}`
		sharedParams = append(params[:len(params):len(params)], clickhouse.Named("sharedTimes", "["+strings.Join(sharedTimes, ",")+"]"))
	}
	counted, err := countShared(ctx, "SELECT count shared "+table, shared(conds, times), sharedParams)
	return single + counted, err
}

func countShared(ctx context.Context, name, query string, params []any) (counted uint64, err error) {
	ctx, span := startQuerySpan(ctx, name, query, params)
	defer func() { endQuerySpan(span, err) }()
	err = clickhouseDB.QueryRow(withLimits(ctx), query, params...).Scan(&counted)
	return counted, err
}
