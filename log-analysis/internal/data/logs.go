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
	// Query logs.
	query := "SELECT * FROM logs order by timestamp desc"
	rows, err := clickhouseDB.Query(ctx, query)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var logs []*Log

	// Iterate over the result set and print logs.
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
		//values[i] = v.Field(i).Interface()
		values[i] = v.Field(i).Interface()
		fmt.Println(v.Type().Field(i).Name, "=", v.Field(i).Interface(), "=", v.Field(i).IsZero())
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

func (l *Log) SelectRootSpan(ctx context.Context, filter SpanFilter /*parent string, rowsPerPage int, timeFrom string, status string*/) ([]*Span, error) {
	// Query logs.
	query := `SELECT toString("UnixTime") as "Timestamp", "SpanName", "ServiceName", "TraceId", "SpanId", "ParentSpanId",
	 arrayMap(key -> map('key', key, 'value', SpanAttributes[key]), mapKeys(SpanAttributes)) AS tags,
	 arrayMap(key -> map('key', key, 'value', ResourceAttributes[key]), mapKeys(ResourceAttributes)) AS serviceTags, "ChildSpanCount", "StatusCode", "StatusMessage", "Duration" FROM otel_traces`

	where, limit := addFilter(filter)
	sorting := addSorting()

	// var where string
	// if len(filter.TimeFrom) > 0 {
	// 	where = `where "ParentSpanId" = {parent:String} and "UnixTime" > toInt64({timeFrom:Int64})`
	// } else {
	// 	where = `where "ParentSpanId" = {parent:String}`
	// }

	// var statusCode string = ""
	// if filter.Status == "error" {
	// 	statusCode = "1"
	// } else if filter.Status == "ok" {
	// 	statusCode = "2"
	// } else if filter.Status == "unset" {
	// 	statusCode = "0"
	// }
	// if len(statusCode) > 0 {
	// 	where = where + ` and "StatusCode" = {statusCode:Int8}`
	// }

	//query = query + ` ` + where + ` and "ServiceName" = 'MC5' order by "UnixTime" ASC limit {rowsPerPage:Int64}`
	query += where + " " + sorting + " " + limit

	fmt.Println("query", query)
	rows, err := clickhouseDB.Query(ctx, query,
		clickhouse.Named("rowsPerPage", strconv.Itoa(filter.RowsPerPage)),
		clickhouse.Named("parent", filter.ParentId),
		clickhouse.Named("timeFrom", filter.TimeFrom),
		clickhouse.Named("statusCode", StatusCodeFromString(filter.Status)),
		clickhouse.Named("serviceName", "%"+filter.ServiceName+"%"))
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var spans []*Span

	var id int64
	id = 0
	// Iterate over the result set and print logs.
	for rows.Next() {
		//fmt.Println(rows, err)
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

		fmt.Println("UnixTimestamp", span.Timestamp)
		for _, tag := range span.Tags {
			if tag["key"] == "db.statement" {
				span.Msg = tag["value"]
			}
		}

		//Unset Code = 0
		//Error Code = 1
		//Ok Code = 2

		span.Status = StatusCodeToString(strconv.Itoa(int(span.StatusCode)))
		// if span.StatusCode == 1 {
		// 	span.Status = "error"
		// } else if span.StatusCode == 2 {
		// 	span.Status = "ok"
		// } else {
		// 	span.Status = "unset"
		// }

		spans = append(spans, &span)
		id = id + 1
	}

	return spans, nil
}

func (l *Log) SelectCountSpans(ctx context.Context, filter SpanFilter) (uint64, error) {
	// Query logs.
	query := `SELECT count(*) FROM otel_traces`
	where, _ := addFilter(filter)
	query += " " + where
	var res uint64
	fmt.Println(query)
	row := clickhouseDB.QueryRow(ctx, query,
		clickhouse.Named("parent", filter.ParentId),
		clickhouse.Named("timeFrom", filter.TimeFrom),
		clickhouse.Named("statusCode", StatusCodeFromString(filter.Status)),
		clickhouse.Named("serviceName", filter.ServiceName))
	err := row.Scan(&res)
	if err != nil {
		fmt.Println(err)
		return 0, err
	}

	return res, nil
}

func (l *Log) InsertTestData(ctx context.Context, data []Log) {
	batch, err := clickhouseDB.PrepareBatch(ctx, "INSERT INTO logs")
	if err != nil {
		panic((err))
	}
	for i, d := range data { //i := 0; i < len(*data); i++ {
		fmt.Println("insert", d)
		err := batch.Append(
			int64(i), d.Timestamp, "INFO", d.Message,
			// uint8(42),
			// "ClickHouse",
			// "Inc",
			// uuid.New(),
			// map[string]uint8{"key": 1},             // Map(String, UInt8)
			// []string{"Q", "W", "E", "R", "T", "Y"}, // Array(String)
			// []any{ // Tuple(String, UInt8, Array(Map(String, String)))
			// 	"String Value", uint8(5), []map[string]string{
			// 		{"key": "value"},
			// 		{"key": "value"},
			// 		{"key": "value"},
			// 	},
			// },
			// time.Now(),
		)
		if err != nil {
			panic((err))
		}
	}
	batch.Send()
}
