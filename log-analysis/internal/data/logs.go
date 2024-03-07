package data

import (
	"context"
	"fmt"
	"log"
	"reflect"
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

func (l *Log) SelectAllData(ctx context.Context) ([]*Log, error) {
	query := "SELECT * FROM logs order by timestamp desc"
	rows, err := clickhouseDB.Query(ctx, query)
	if err != nil {
		log.Fatal(err)
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
				condition = append(condition, `"UnixTime" > toInt64({timeFrom:Int64})`)
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

func addSorting() string {
	return `order by "UnixTime" ASC`
}

func (l *Log) SelectRootSpan(ctx context.Context, filter SpanFilter) ([]*Span, error) {
	query := `SELECT toString("UnixTime") as "Timestamp", "SpanName", "ServiceName", "TraceId", "SpanId", "ParentSpanId",
	 arrayMap(key -> map('key', key, 'value', SpanAttributes[key]), mapKeys(SpanAttributes)) AS tags,
	 arrayMap(key -> map('key', key, 'value', ResourceAttributes[key]), mapKeys(ResourceAttributes)) AS serviceTags, "ChildSpanCount", "StatusCode", "StatusMessage", "Duration" FROM otel_traces`

	where, limit := addFilter(filter)
	sorting := addSorting()
	query += where + " " + sorting + " " + limit
	rows, err := clickhouseDB.Query(ctx, query,
		clickhouse.Named("rowsPerPage", strconv.Itoa(filter.RowsPerPage)),
		clickhouse.Named("parent", filter.ParentId),
		clickhouse.Named("timeFrom", filter.TimeFrom),
		clickhouse.Named("statusCode", StatusCodeFromString(filter.Status)),
		clickhouse.Named("serviceName", "%"+filter.ServiceName+"%"),
		clickhouse.Named("spanName", "%"+filter.MethodName+"%"))
	if err != nil {
		log.Fatal(err)
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
		for _, tag := range span.Tags {
			if tag["key"] == "db.statement" {
				msgField = tag["value"]
			}
			key := tag["key"]
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
			for argNum, value := range sqlArgs {
				placeholder := fmt.Sprintf("$%d", argNum)
				msgField = strings.ReplaceAll(msgField, placeholder, "'"+value+"'")
			}
		}
		span.Msg = msgField
		span.Status = StatusCodeToString(strconv.Itoa(int(span.StatusCode)))
		spans = append(spans, &span)
		id = id + 1
	}

	return spans, nil
}

func (l *Log) SelectCountSpans(ctx context.Context, filter SpanFilter) (uint64, error) {
	query := `SELECT count(*) FROM otel_traces`
	where, _ := addFilter(filter)
	query += " " + where
	var res uint64
	row := clickhouseDB.QueryRow(ctx, query,
		clickhouse.Named("parent", filter.ParentId),
		clickhouse.Named("timeFrom", filter.TimeFrom),
		clickhouse.Named("statusCode", StatusCodeFromString(filter.Status)),
		clickhouse.Named("serviceName", filter.ServiceName))
	err := row.Scan(&res)
	if err != nil {
		return 0, err
	}

	return res, nil
}
