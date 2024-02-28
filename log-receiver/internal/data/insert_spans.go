package data

import (
	"context"
	"fmt"
	"time"

	pb "github.com/artemparygin/troubleshooting/protos/logs"
)

type Log struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"time"`
	Level     string    `json:"level"`
	Message   string    `json:"msg"`
}

func (l *Log) InsertTraceData(ctx context.Context, data []*pb.OneSpan) {
	batch, err := clickhouseDB.PrepareBatch(ctx, "INSERT INTO otel_traces")
	if err != nil {
		panic((err))
	}
	for _, d := range data { //i := 0; i < len(*data); i++ {
		//fmt.Println("insert", d)
		err := batch.Append(
			d.Timestamp.AsTime().UnixNano(),
			d.Timestamp.AsTime(),
			d.Timestamp.AsTime(),
			d.TraceId,
			d.SpanId,
			d.ParentSpanId,
			d.TraceState,
			d.SpanName,
			d.SpanKind,
			d.ServiceName,
			d.ResourceAttributes,
			d.SpanAttributes,
			d.Duration,
			int8(d.StatusCode),
			d.StatusMessage,
			[]time.Time{d.Timestamp.AsTime()},
			[]string{"event-name"},
			[]map[string]string{{"key": "value"}},
			[]string{},
			[]string{},
			[]string{},
			[]map[string]string{},
			d.ChildSpanCount,
		)
		if err != nil {
			panic((err))
		}
	}
	err = batch.Send()
	if err != nil {
		panic((err))
	}
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
