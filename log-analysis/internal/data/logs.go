package data

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type Log struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"time"`
	Level     string    `json:"level"`
	Message   string    `json:"msg"`
}

type SpanFilter struct {
	ParentId    string `json:"parent_id"`
	RowsPerPage int    `json:"rows_per_page"`
	TimeFrom    string `json:"time_from"`
	Status      string `json:"status"`
	ServiceName string `json:"service_name"`
	MethodName  string `json:"method_name"`
	// AfterSpanId, together with TimeFrom, is the cursor of the next page: the
	// SpanId of the last row shown. Rows with the same UnixTime as that row are
	// then paged by SpanId instead of being skipped.
	AfterSpanId string `json:"after_span_id"`
}

type Span struct {
	Timestamp      string              `json:"timeStamp"`
	SpanName       string              `json:"name"`
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
	Duration       int64               `json:"duration"`
}

var StatusCodeMap = map[string]string{
	"unset": "0",
	"error": "1",
	"ok":    "2",
}

var ReverseStatusCodeMap map[string]string

func StatusCodeFromString(status string) string {
	return StatusCodeMap[status]
}

func StatusCodeToString(code string) string {
	if len(ReverseStatusCodeMap) == 0 {
		ReverseStatusCodeMap = make(map[string]string)
		for stringStatus, codeStatus := range StatusCodeMap {
			ReverseStatusCodeMap[codeStatus] = stringStatus
		}
	}

	return ReverseStatusCodeMap[code]
}

func (l *Log) SelectAllData(ctx context.Context) (_ []*Log, err error) {
	query := "SELECT id, timestamp, level, message FROM logs order by timestamp desc"
	ctx, span := startQuerySpan(ctx, "SELECT logs", query, nil)
	defer func() { endQuerySpan(span, err) }()

	rows, err := clickhouseDB.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*Log
	for rows.Next() {

		var log Log

		err := rows.Scan(
			&log.ID,
			&log.Timestamp,
			&log.Level,
			&log.Message,
		)
		if err != nil {
			return nil, err
		}

		logs = append(logs, &log)
	}

	return logs, nil
}

func addFilter(filter SpanFilter) (string, string) {
	v := reflect.ValueOf(filter)

	values := make([]interface{}, v.NumField())
	var condition []string
	var limit string
	for i := 0; i < v.NumField(); i++ {
		values[i] = v.Field(i).Interface()
		if !v.Field(i).IsZero() {
			switch field := v.Type().Field(i).Name; field {
			case "TimeFrom":
				if filter.AfterSpanId != "" {
					condition = append(condition, `("UnixTime" > toInt64({timeFrom:Int64}) or ("UnixTime" = toInt64({timeFrom:Int64}) and "SpanId" > {afterSpanId:String}))`)
				} else {
					condition = append(condition, `"UnixTime" > toInt64({timeFrom:Int64})`)
				}
			case "ParentId":
				condition = append(condition, `"ParentSpanId" = {parent:String}`)
			case "Status":
				condition = append(condition, `"StatusCode" = {statusCode:Int8}`)
			case "ServiceName":
				condition = append(condition, `"ServiceName" iLike {serviceName:String}`)
			case "MethodName":
				condition = append(condition, `"SpanName" iLike {spanName:String}`)
			case "RowsPerPage":
				limit = `limit {rowsPerPage:Int64}`
			}
		}
	}

	var where string = ""
	if len(condition) > 0 {
		where = " where " + strings.Join(condition[:], " and ")
	}

	return where, limit
}

// filterParams binds every placeholder addFilter may produce, so the span list
// and the span count always filter identically.
func filterParams(filter SpanFilter) []any {
	return []any{
		clickhouse.Named("rowsPerPage", strconv.Itoa(filter.RowsPerPage)),
		clickhouse.Named("parent", filter.ParentId),
		clickhouse.Named("timeFrom", filter.TimeFrom),
		clickhouse.Named("statusCode", StatusCodeFromString(filter.Status)),
		clickhouse.Named("serviceName", "%"+filter.ServiceName+"%"),
		clickhouse.Named("spanName", "%"+filter.MethodName+"%"),
		clickhouse.Named("afterSpanId", filter.AfterSpanId),
	}
}

var sqlPlaceholder = regexp.MustCompile(`\$(\d+)`)

// fillSQLArgs replaces $N placeholders with quoted argument values in a single
// pass, so $1 never matches inside $10 and values are never re-substituted.
func fillSQLArgs(statement string, args map[int]string) string {
	return sqlPlaceholder.ReplaceAllStringFunc(statement, func(placeholder string) string {
		argNum, err := strconv.Atoi(placeholder[1:])
		if err != nil {
			return placeholder
		}
		value, ok := args[argNum]
		if !ok {
			return placeholder
		}
		return "'" + value + "'"
	})
}

func addSorting() string {
	return `order by "UnixTime" ASC, "SpanId" ASC`
}

// SelectRootSpan returns one page of spans matching filter (root spans unless
// filter.ParentId says otherwise), with child counts read from the table.
func (l *Log) SelectRootSpan(ctx context.Context, filter SpanFilter) ([]*Span, error) {
	spans, err := selectSpans(ctx, filter)
	if err != nil {
		return nil, err
	}
	if err := fillChildSpanCounts(ctx, spans); err != nil {
		return nil, err
	}
	return spans, nil
}

func selectSpans(ctx context.Context, filter SpanFilter) (_ []*Span, err error) {
	query := `SELECT toString("UnixTime") as "Timestamp", "SpanName", "ServiceName", "TraceId", "SpanId", "ParentSpanId",
	 arrayMap(key -> map('key', key, 'value', SpanAttributes[key]), mapKeys(SpanAttributes)) AS tags,
	 arrayMap(key -> map('key', key, 'value', ResourceAttributes[key]), mapKeys(ResourceAttributes)) AS serviceTags, "ChildSpanCount", "StatusCode", "StatusMessage", "Duration" FROM otel_traces`

	where, limit := addFilter(filter)
	sorting := addSorting()
	query += where + " " + sorting + " " + limit
	params := filterParams(filter)
	ctx, dbSpan := startQuerySpan(ctx, "SELECT otel_traces", query, params)
	defer func() { endQuerySpan(dbSpan, err) }()

	rows, err := clickhouseDB.Query(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var spans []*Span

	var id int64 = 0
	for rows.Next() {
		var span Span
		err := rows.Scan(
			&span.Timestamp,
			&span.SpanName,
			&span.ServiceName,
			&span.TraceId,
			&span.SpanId,
			&span.ParentSpanId,
			&span.Tags,
			&span.ServiceTags,
			&span.ChildSpanCount,
			&span.StatusCode,
			&span.StatusMessage,
			&span.Duration,
		)
		if err != nil {
			return nil, err
		}

		var msgField string = ""
		sqlArgs := make(map[int]string)
		namedParams := make(map[string]string)
		for _, tag := range span.Tags {
			if tag["key"] == "db.statement" {
				msgField = tag["value"]
			}
			key := tag["key"]
			if strings.HasPrefix(key, queryParamPrefix) {
				namedParams[key[len(queryParamPrefix):]] = tag["value"]
			}
			if strings.HasPrefix(tag["key"], "db.sql.args.") {
				argNum := key[len("db.sql.args."):]
				value := tag["value"]
				argsNum := strings.TrimSuffix(argNum, ".")
				var argNumInt int
				fmt.Sscanf(argsNum, "%d", &argNumInt)
				sqlArgs[argNumInt] = value
			}
		}
		if len(msgField) > 0 && len(sqlArgs) > 0 {
			msgField = fillSQLArgs(msgField, sqlArgs)
		}
		if len(msgField) > 0 && len(namedParams) > 0 {
			msgField = fillNamedParams(msgField, namedParams)
		}
		span.Msg = msgField
		span.Status = StatusCodeToString(strconv.Itoa(int(span.StatusCode)))
		spans = append(spans, &span)
		id = id + 1
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return spans, nil
}

// fillChildSpanCounts replaces the stored ChildSpanCount with the number of
// children actually in the table. OTLP doesn't send that count at all, and the
// one log-sender sends misses children that start after their parent ends or
// come from other services. The UI shows the expand arrow from it.
func fillChildSpanCounts(ctx context.Context, spans []*Span) (err error) {
	if len(spans) == 0 {
		return nil
	}
	ids := make([]string, len(spans))
	for i, span := range spans {
		ids[i] = span.SpanId
	}

	query := `SELECT "ParentSpanId", toInt32(count()) FROM otel_traces WHERE "ParentSpanId" IN {spanIds:Array(String)}`
	params := []any{clickhouse.Named("spanIds", arrayLiteral(ids))}
	// Children never start before their parent, so a lower time bound lets
	// ClickHouse skip older data via the primary key (UnixTime). There is no
	// upper bound: children of async work (queues, background jobs) can start
	// long after their parent ended.
	if from, ok := earliestChildStart(spans); ok {
		query += ` AND "UnixTime" >= toInt64({childrenFrom:Int64})`
		params = append(params, clickhouse.Named("childrenFrom", strconv.FormatInt(from, 10)))
	}
	query += ` GROUP BY "ParentSpanId"`
	ctx, span := startQuerySpan(ctx, "SELECT children otel_traces", query, params)
	defer func() { endQuerySpan(span, err) }()

	rows, err := clickhouseDB.Query(ctx, query, params...)
	if err != nil {
		return err
	}
	defer rows.Close()

	counts := make(map[string]int32, len(spans))
	for rows.Next() {
		var parent string
		var count int32
		if err := rows.Scan(&parent, &count); err != nil {
			return err
		}
		counts[parent] = count
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, s := range spans {
		s.ChildSpanCount = counts[s.SpanId]
	}
	return nil
}

// childClockSkew allows for children recorded by another service whose clock
// is behind the parent's.
const childClockSkew = int64(time.Minute)

// earliestChildStart returns the earliest UnixTime (nanoseconds) at which a
// child of spans can start: the earliest parent start minus childClockSkew.
// ok is false if a timestamp can't be parsed, and then the query runs without
// the bound.
func earliestChildStart(spans []*Span) (from int64, ok bool) {
	for i, span := range spans {
		start, err := strconv.ParseInt(span.Timestamp, 10, 64)
		if err != nil {
			return 0, false
		}
		if i == 0 || start < from {
			from = start
		}
	}
	return from - childClockSkew, len(spans) > 0
}

// arrayLiteral formats values as a ClickHouse Array(String) literal such as
// ['a','b']. The driver only accepts strings for query parameters.
func arrayLiteral(values []string) string {
	escape := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "'" + escape.Replace(v) + "'"
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

func (l *Log) SelectCountSpans(ctx context.Context, filter SpanFilter) (_ uint64, err error) {
	query := `SELECT count(*) FROM otel_traces`
	where, _ := addFilter(filter)
	query += " " + where
	params := filterParams(filter)
	ctx, span := startQuerySpan(ctx, "SELECT count otel_traces", query, params)
	defer func() { endQuerySpan(span, err) }()

	var res uint64
	row := clickhouseDB.QueryRow(ctx, query, params...)
	err = row.Scan(&res)
	if err != nil {
		return 0, err
	}

	return res, nil
}
