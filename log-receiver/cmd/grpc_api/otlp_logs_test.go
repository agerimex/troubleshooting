package main

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"log-receiver/internal/data"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// fakeClickHouse records the log rows of each insert; sendErr fails Send.
type fakeClickHouse struct {
	driver.Conn
	mu      sync.Mutex
	sent    [][]any
	sendErr error
}

type fakeBatch struct {
	driver.Batch
	db   *fakeClickHouse
	rows []any
}

func (f *fakeClickHouse) PrepareBatch(ctx context.Context, query string, opts ...driver.PrepareBatchOption) (driver.Batch, error) {
	return &fakeBatch{db: f}, nil
}

func (b *fakeBatch) Append(v ...any) error { b.rows = append(b.rows, v); return nil }
func (b *fakeBatch) Abort() error          { return nil }
func (b *fakeBatch) Send() error {
	b.db.mu.Lock()
	defer b.db.mu.Unlock()
	if b.db.sendErr != nil {
		return b.db.sendErr
	}
	b.db.sent = append(b.db.sent, b.rows)
	return nil
}

func (f *fakeClickHouse) inserted() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, rows := range f.sent {
		n += len(rows)
	}
	return n
}

// serveLogs starts the OTLP logs service on an in-memory listener, with the
// token check main sets up when TROUBLESHOOTING_TOKEN is set.
func serveLogs(t *testing.T, db *fakeClickHouse, token string) collectorlogspb.LogsServiceClient {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	var opts []grpc.ServerOption
	if token != "" {
		opts = append(opts, grpc.UnaryInterceptor(tokenAuth(token)))
	}
	server := grpc.NewServer(opts...)
	collectorlogspb.RegisterLogsServiceServer(server, &otlpLogsServer{models: data.New(db)})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.Dial("bufnet", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return collectorlogspb.NewLogsServiceClient(conn)
}

func exportRequest(records ...*logspb.LogRecord) *collectorlogspb.ExportLogsServiceRequest {
	return &collectorlogspb.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{
		Resource:  &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "controller"}}}}},
		ScopeLogs: []*logspb.ScopeLogs{{LogRecords: records}},
	}}}
}

func logAt(body string, edit ...func(*logspb.LogRecord)) *logspb.LogRecord {
	r := &logspb.LogRecord{TimeUnixNano: uint64(time.Now().UnixNano()), SeverityText: "Information",
		Body: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: body}}}
	for _, e := range edit {
		e(r)
	}
	return r
}

func TestOTLPLogsExportStoresBeforeAnswering(t *testing.T) {
	db := &fakeClickHouse{}
	client := serveLogs(t, db, "")
	resp, err := client.Export(context.Background(), exportRequest(logAt("one"), logAt("two"), logAt("bad", func(r *logspb.LogRecord) { r.SpanId = []byte{1} })))
	if err != nil {
		t.Fatal(err)
	}
	// The answer came after the insert: both valid records are already stored.
	if n := db.inserted(); n != 2 {
		t.Errorf("stored %d records before answering, want 2", n)
	}
	if ps := resp.GetPartialSuccess(); ps.GetRejectedLogRecords() != 1 || ps.GetErrorMessage() == "" {
		t.Errorf("partial success = %+v, want 1 rejected record with a message", ps)
	}
	// A request without rejections has no partial success.
	resp, err = client.Export(context.Background(), exportRequest(logAt("three")))
	if err != nil || resp.GetPartialSuccess() != nil {
		t.Errorf("clean request: %v, partial success %+v", err, resp.GetPartialSuccess())
	}
}

func TestOTLPLogsInsertFailureIsRetryable(t *testing.T) {
	db := &fakeClickHouse{sendErr: errors.New("connection refused")}
	client := serveLogs(t, db, "")
	_, err := client.Export(context.Background(), exportRequest(logAt("one")))
	if status.Code(err) != codes.Unavailable {
		t.Errorf("code = %v, want Unavailable (OTLP exporters retry it)", status.Code(err))
	}
}

func TestOTLPLogsNeedTheToken(t *testing.T) {
	db := &fakeClickHouse{}
	client := serveLogs(t, db, "secret")
	if _, err := client.Export(context.Background(), exportRequest(logAt("one"))); status.Code(err) != codes.Unauthenticated {
		t.Errorf("without the token: code %v, want Unauthenticated", status.Code(err))
	}
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer secret")
	if _, err := client.Export(ctx, exportRequest(logAt("one"))); err != nil {
		t.Errorf("with the token: %v", err)
	}
	if n := db.inserted(); n != 1 {
		t.Errorf("stored %d records, want 1 (only the authorized request)", n)
	}
}
