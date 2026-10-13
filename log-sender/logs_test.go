package sender

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	pb "github.com/agerimex/troubleshooting/protos/logs"
	"github.com/rs/zerolog"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeLogReceiver struct {
	pb.UnimplementedLogServiceServer
	mu      sync.Mutex
	fail    bool
	records []*pb.LogMessageRequest
}

func (f *fakeLogReceiver) SendLogs(_ context.Context, req *pb.Logs) (*pb.LogMessageResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return nil, status.Error(codes.Unavailable, "storage unavailable")
	}
	f.records = append(f.records, req.Logs...)
	return &pb.LogMessageResponse{Success: true}, nil
}

func (f *fakeLogReceiver) setFail(fail bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = fail
}

func (f *fakeLogReceiver) snapshot() []*pb.LogMessageRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*pb.LogMessageRequest(nil), f.records...)
}

func startFakeLogReceiver(t *testing.T) (*fakeLogReceiver, string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	receiver := &fakeLogReceiver{}
	server := grpc.NewServer()
	pb.RegisterLogServiceServer(server, receiver)
	go server.Serve(lis)
	t.Cleanup(server.Stop)
	return receiver, lis.Addr().String()
}

func TestLogExporterReportsFailureAndRetriesSameIdentity(t *testing.T) {
	receiver, addr := startFakeLogReceiver(t)
	exporter, err := NewLogExporter("recording-worker", WithAddress(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer exporter.Close()

	record := NewLogRecord(context.Background(), "ERROR", "Storage returned 403 AccessDenied")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	receiver.setFail(true)
	if err := exporter.Send(ctx, []LogRecord{record}); err == nil {
		t.Fatal("Send confirmed a record the receiver did not store")
	}
	receiver.setFail(false)
	if err := exporter.Send(ctx, []LogRecord{record}); err != nil {
		t.Fatalf("retry: %v", err)
	}
	got := receiver.snapshot()
	if len(got) != 1 || got[0].RecordId != record.RecordID || got[0].ServiceName != "recording-worker" ||
		got[0].Severity != "ERROR" || got[0].TraceId != "" || got[0].SpanId != "" {
		t.Fatalf("receiver got %v", got)
	}
}

func TestNewLogRecordTakesTheActiveSpan(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	ctx, span := tp.Tracer("test").Start(context.Background(), "upload")
	defer span.End()

	r := NewLogRecord(ctx, "WARN", "Upload retry 1 of 3")
	if r.TraceID != span.SpanContext().TraceID().String() || r.SpanID != span.SpanContext().SpanID().String() {
		t.Fatalf("record trace %s span %s, want the active span", r.TraceID, r.SpanID)
	}
	if other := NewLogRecord(ctx, "WARN", "Upload retry 1 of 3"); other.RecordID == r.RecordID {
		t.Fatal("two events got the same RecordID")
	}
	if none := NewLogRecord(context.Background(), "INFO", "x"); none.TraceID != "" || none.SpanID != "" {
		t.Fatalf("record without a span got trace %q span %q", none.TraceID, none.SpanID)
	}
}

func TestLogWriterKeepsRecordsUntilConfirmed(t *testing.T) {
	receiver, addr := startFakeLogReceiver(t)
	exporter, err := NewLogExporter("recording-worker", WithAddress(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer exporter.Close()
	w := NewLogWriter(exporter, 0)

	tp := sdktrace.NewTracerProvider()
	spanCtx, span := tp.Tracer("test").Start(context.Background(), "upload")
	defer span.End()

	logger := zerolog.New(w).Hook(TraceHook{})
	logger.Error().Ctx(spanCtx).Str("bucket", "recordings").Msg("Storage returned 403 AccessDenied")
	logger.Info().Msg("Recording queued for processing")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	receiver.setFail(true)
	if err := w.Flush(ctx); err == nil || w.Pending() != 2 || w.Err() == nil {
		t.Fatalf("Flush = %v, pending %d: want an error and both records kept", err, w.Pending())
	}
	receiver.setFail(false)
	if err := w.Flush(ctx); err != nil || w.Pending() != 0 || w.Err() != nil {
		t.Fatalf("Flush = %v, pending %d", err, w.Pending())
	}

	got := receiver.snapshot()
	if len(got) != 2 {
		t.Fatalf("receiver got %d records, want 2", len(got))
	}
	denied, queued := got[0], got[1]
	if denied.Severity != "error" || denied.TraceId != span.SpanContext().TraceID().String() ||
		denied.SpanId != span.SpanContext().SpanID().String() || denied.Attributes["bucket"] != "recordings" ||
		denied.RecordId == "" || denied.Message != "Storage returned 403 AccessDenied" {
		t.Fatalf("error record = %v", denied)
	}
	if queued.TraceId != "" || queued.SpanId != "" || queued.RecordId == denied.RecordId {
		t.Fatalf("record without a span = %v", queued)
	}
}

func TestLogWriterDropsOldestWhenFull(t *testing.T) {
	w := NewLogWriter(&LogExporter{}, 0)
	w.limit = 2
	logger := zerolog.New(w)
	for i := 0; i < 3; i++ {
		logger.Info().Int("i", i).Msg("x")
	}
	if w.Pending() != 2 || w.Dropped() != 1 {
		t.Fatalf("pending %d dropped %d, want 2 and 1", w.Pending(), w.Dropped())
	}
	if err := w.Flush(context.Background()); err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("Flush with an unconnected exporter = %v, want an error", err)
	}
}

func TestLogExporterAddsResourceAttributesWithoutOverriding(t *testing.T) {
	e := &LogExporter{service: "worker", resource: map[string]string{AttrServiceVersion: "1.1.0", AttrEnvironment: "fixture"}}
	r := LogRecord{Time: time.Unix(1, 0), Message: "m", Attributes: map[string]string{AttrEnvironment: "test", AttrOperation: "upload"}}
	got := e.toProto(r).Attributes
	want := map[string]string{AttrServiceVersion: "1.1.0", AttrEnvironment: "test", AttrOperation: "upload"}
	if len(got) != len(want) {
		t.Fatalf("attributes = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("attribute %s = %q, want %q", k, got[k], v)
		}
	}
}
