package data

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// LogFilter is the body of search-logs. Like SpanFilter it needs a half-open
// window [TimeFrom, TimeTo) in Unix nanoseconds. A logical log record is
// identified by RecordId (retries repeat it); the cursor is the last row's
// (UnixTime, RecordId).
type LogFilter struct {
	TimeFrom    string `json:"time_from"`
	TimeTo      string `json:"time_to"`
	TraceId     string `json:"trace_id"`
	ServiceName string `json:"service_name"` // case-insensitive substring
	Severity    string `json:"severity"`     // exact, e.g. ERROR
	Message     string `json:"message"`      // case-insensitive substring, no wildcards
	RowsPerPage int    `json:"rows_per_page"`

	AfterTime     string `json:"after_time"`
	AfterRecordId string `json:"after_record_id"`
}

// MaxLogRowsPerPage caps a page of log records.
const MaxLogRowsPerPage = 1000

// Validate checks the filter, normalizes ids and severity and caps RowsPerPage.
func (f *LogFilter) Validate() error {
	if err := validateWindow(f.TimeFrom, f.TimeTo); err != nil {
		return err
	}
	f.TraceId = strings.ToLower(f.TraceId)
	if f.TraceId != "" && !traceIDPattern.MatchString(f.TraceId) {
		return invalid("trace_id must be 32 hex digits")
	}
	f.Severity = strings.ToUpper(strings.TrimSpace(f.Severity))
	if f.AfterTime != "" || f.AfterRecordId != "" {
		if _, err := strconv.ParseInt(f.AfterTime, 10, 64); err != nil {
			return invalid("after_time must be Unix nanoseconds")
		}
		if f.AfterRecordId == "" {
			return invalid("the cursor needs after_time and after_record_id")
		}
	}
	if f.RowsPerPage <= 0 {
		f.RowsPerPage = DefaultRowsPerPage
	}
	if f.RowsPerPage > MaxLogRowsPerPage {
		f.RowsPerPage = MaxLogRowsPerPage
	}
	return nil
}

// validateWindow checks a half-open window [time_from, time_to); an empty one
// (time_to <= time_from) can match nothing and is rejected.
func validateWindow(timeFrom, timeTo string) error {
	from, err := strconv.ParseInt(timeFrom, 10, 64)
	if err != nil {
		return invalid("time_from must be Unix nanoseconds")
	}
	to, err := strconv.ParseInt(timeTo, 10, 64)
	if err != nil {
		return invalid("time_to must be Unix nanoseconds")
	}
	if to <= from {
		return invalid("time_to must be after time_from (the window is [time_from, time_to))")
	}
	if time.Duration(to-from) > limits.MaxWindow {
		return invalid("the time window is longer than %s", limits.MaxWindow)
	}
	return nil
}

func (f *LogFilter) conditions(withCursor bool) ([]string, []any) {
	var conds []string
	var params []any
	add := func(cond string, named ...any) {
		conds = append(conds, cond)
		params = append(params, named...)
	}
	if f.TraceId != "" {
		add(`"TraceId" = {traceId:String}`, clickhouse.Named("traceId", f.TraceId))
	}
	if f.ServiceName != "" {
		add(`positionCaseInsensitiveUTF8("ServiceName", {serviceName:String}) > 0`, clickhouse.Named("serviceName", f.ServiceName))
	}
	if f.Severity != "" {
		add(`"Severity" = {severity:String}`, clickhouse.Named("severity", f.Severity))
	}
	if f.Message != "" {
		add(`positionCaseInsensitiveUTF8("Body", {message:String}) > 0`, clickhouse.Named("message", f.Message))
	}
	if withCursor && f.AfterTime != "" {
		add(`("UnixTime", "RecordId") > (toInt64({afterTime:Int64}), {afterRecordId:String})`,
			clickhouse.Named("afterTime", f.AfterTime),
			clickhouse.Named("afterRecordId", f.AfterRecordId))
	}
	return conds, params
}

func (f *LogFilter) windowParams() []any {
	return []any{clickhouse.Named("timeFrom", f.TimeFrom), clickhouse.Named("timeTo", f.TimeTo)}
}

const logColumns = `"UnixTime", "RecordId", "TraceId", "SpanId", "Severity", "ServiceName", "Body", "Attributes"`

// logPayloadColumns are the columns of a log row with its attribute map sorted
// (see spanPayloadColumns).
const logPayloadColumns = `"UnixTime", "RecordId", "TraceId", "SpanId", "Severity", "ServiceName", "Body", mapSort("Attributes")`

// canonicalLogs is a subquery with one row per RecordId among the record's
// rows in the window, chosen before conds apply (like canonicalSpans). Exact
// retries are identical rows; for conflicting payloads with one RecordId the
// row with the larger hash wins, independent of arrival order. Separate
// events have separate RecordIds and are never merged, however alike.
func canonicalLogs(conds []string) string {
	preselect := ""
	if len(conds) > 0 {
		preselect = ` AND "RecordId" IN (SELECT "RecordId" FROM log_records WHERE ` +
			windowCondition + ` AND ` + strings.Join(conds, " AND ") + `)`
	}
	return `SELECT ` + logColumns + ` FROM log_records WHERE ` + windowCondition + preselect + `
	ORDER BY "RecordId", cityHash64(` + logPayloadColumns + `) DESC
	LIMIT 1 BY "RecordId"`
}

// LogRecord is one logical log record. TraceId and SpanId are exactly what was
// recorded; empty means not recorded.
type LogRecord struct {
	TimeUnixNano string            `json:"timeUnixNano"`
	RecordId     string            `json:"recordId"`
	TraceId      string            `json:"traceId"`
	SpanId       string            `json:"spanId"`
	Severity     string            `json:"severity"`
	ServiceName  string            `json:"service"`
	Message      string            `json:"message"`
	Attributes   map[string]string `json:"attributes"`
}

// LogPage is one page of log records; HasMore is read from one extra row.
type LogPage struct {
	Logs    []*LogRecord
	HasMore bool
}

// logPageCandidates is the first {scanRows:UInt32} rows matching conds (with
// the cursor) in time order, read like pageCandidates: "UnixTime" alone, the
// table's sorting key, so ClickHouse stops early.
func logPageCandidates(conds []string) string {
	where := windowCondition
	if len(conds) > 0 {
		where += ` AND ` + strings.Join(conds, " AND ")
	}
	return `SELECT "UnixTime", "RecordId" FROM log_records WHERE ` + where + `
	ORDER BY "UnixTime" LIMIT {scanRows:UInt32}`
}

// logPageCanonical is canonicalLogs for one page: one row per candidate
// RecordId among its rows in [{pageFrom:Int64}, {pageTo:Int64}) before conds
// apply. A retry repeats a record with its time, so all its rows are in that
// range; copies of one RecordId stored with different times are not compared
// (as for spans, see pageCanonicalSpans).
func logPageCanonical(conds []string) string {
	return `SELECT ` + logColumns + ` FROM log_records WHERE ` + windowCondition + `
	AND "UnixTime" >= toInt64({pageFrom:Int64}) AND "UnixTime" < toInt64({pageTo:Int64})
	AND "RecordId" IN (SELECT "RecordId" FROM (` + logPageCandidates(conds) + `))
	ORDER BY "RecordId", cityHash64(` + logPayloadColumns + `) DESC
	LIMIT 1 BY "RecordId"`
}

// SelectLogs returns one page of canonical log records matching a validated
// filter, in (UnixTime, RecordId) order. Like selectSpans it reads the next
// candidates in time order and canonicalizes only them, so a page doesn't read
// the rest of the window; it is exact when the candidates ran out or the page
// is full, and falls back to canonicalLogs over the window after pageScans.
func (l *Log) SelectLogs(ctx context.Context, filter LogFilter) (LogPage, error) {
	conds, params := filter.conditions(true)
	params = append(params, filter.windowParams()...)
	want := filter.RowsPerPage + 1

	for _, factor := range pageScans {
		scan := factor * want
		scanParams := append(params[:len(params):len(params)], clickhouse.Named("scanRows", strconv.Itoa(scan)))
		first, last, candidates, err := logPageRange(ctx, conds, scanParams)
		if err != nil {
			return LogPage{}, err
		}
		if candidates == 0 {
			return LogPage{}, nil
		}
		ranOut := candidates < uint64(scan)
		to := last
		if ranOut {
			to++ // every matching row is a candidate, the last time included
		}
		pageParams := append(scanParams, clickhouse.Named("pageFrom", strconv.FormatInt(first, 10)),
			clickhouse.Named("pageTo", strconv.FormatInt(to, 10)))
		page, read, err := queryLogPage(ctx, filter, logPageCanonical(conds), conds, pageParams)
		if err != nil || ranOut || read == want {
			return page, err
		}
	}
	page, _, err := queryLogPage(ctx, filter, canonicalLogs(conds), conds, params)
	return page, err
}

// logPageRange returns the first and last candidate time of a log page and the
// number of candidate rows.
func logPageRange(ctx context.Context, conds []string, params []any) (first, last int64, candidates uint64, err error) {
	query := `SELECT min("UnixTime"), max("UnixTime"), count() FROM (` + logPageCandidates(conds) + `)`

	ctx, dbSpan := startQuerySpan(ctx, "SELECT page log_records", query, params)
	defer func() { endQuerySpan(dbSpan, err) }()

	err = clickhouseDB.QueryRow(withLimits(ctx), query, params...).Scan(&first, &last, &candidates)
	return first, last, candidates, err
}

// queryLogPage reads up to RowsPerPage+1 canonical records of source that
// match conds, in (UnixTime, RecordId) order, and returns how many it read.
func queryLogPage(ctx context.Context, filter LogFilter, source string, conds []string, params []any) (_ LogPage, read int, err error) {
	params = append(params[:len(params):len(params)], clickhouse.Named("limit", strconv.Itoa(filter.RowsPerPage+1)))

	query := `SELECT toString("UnixTime"), "RecordId", "TraceId", "SpanId", "Severity", "ServiceName", "Body", "Attributes"
	FROM (` + source + `)` + whereClause(conds) + `
	ORDER BY "UnixTime", "RecordId" LIMIT {limit:UInt32}`

	ctx, dbSpan := startQuerySpan(ctx, "SELECT log_records", query, params)
	defer func() { endQuerySpan(dbSpan, err) }()

	rows, err := clickhouseDB.Query(withLimits(ctx), query, params...)
	if err != nil {
		return LogPage{}, 0, err
	}
	defer rows.Close()

	var page LogPage
	for rows.Next() {
		var r LogRecord
		if err := rows.Scan(&r.TimeUnixNano, &r.RecordId, &r.TraceId, &r.SpanId, &r.Severity,
			&r.ServiceName, &r.Message, &r.Attributes); err != nil {
			return LogPage{}, 0, err
		}
		if len(page.Logs) == filter.RowsPerPage {
			page.HasMore = true
			break
		}
		if r.Attributes == nil {
			r.Attributes = map[string]string{}
		}
		page.Logs = append(page.Logs, &r)
	}
	if err := rows.Err(); err != nil {
		return LogPage{}, 0, err
	}
	read = len(page.Logs)
	if page.HasMore {
		read++
	}
	return page, read, nil
}

// sharedTimeLogs counts the canonical records matching conds among the rows
// whose time is shared (times is an IN operand), ranked like canonicalLogs
// before conds apply.
func sharedTimeLogs(conds []string, times string) string {
	return `SELECT countIf(w) FROM (SELECT argMax(` + matchAll(conds) + `, cityHash64(` + logPayloadColumns + `)) AS w
	FROM log_records WHERE ` + windowCondition + ` AND "UnixTime" IN ` + times + `
	GROUP BY "UnixTime", "RecordId")`
}

// SelectCountLogs counts the canonical log records matching a validated filter
// in its window (the cursor is ignored), the way SelectCountSpans counts spans:
// a row at a time no other row shares counts directly, only rows at shared
// times are ranked.
func (l *Log) SelectCountLogs(ctx context.Context, filter LogFilter) (uint64, error) {
	conds, params := filter.conditions(false)
	params = append(params, filter.windowParams()...)
	return countByTime(ctx, "log_records", conds, params, sharedTimeLogs)
}
