package data

import (
	"context"
	"fmt"
	"time"

	pb "github.com/agerimex/troubleshooting/protos/logs"
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
	for _, d := range data {
		err := batch.Append(
			d.Timestamp.AsTime().UnixNano(),
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
			fmt.Println(err)
		}
	}
	err = batch.Send()
	if err != nil {
		fmt.Println(err)
	}
}

func (l *Log) InsertLogData(ctx context.Context, data []Log) {
	batch, err := clickhouseDB.PrepareBatch(ctx, "INSERT INTO logs")
	if err != nil {
		fmt.Println(err)
	}
	for i, d := range data {
		err := batch.Append(
			int64(i), d.Timestamp, "INFO", d.Message,
		)
		if err != nil {
			fmt.Println(err)
		}
	}
	batch.Send()
}
