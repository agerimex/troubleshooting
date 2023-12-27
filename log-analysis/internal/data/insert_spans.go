package data

import (
	"context"
	"time"

	pb "logs-backend/cmd/grpc_api/logs"
)

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
