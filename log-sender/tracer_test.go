package sender

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	pb "github.com/agerimex/troubleshooting/protos/logs"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func testSpan() sdktrace.ReadOnlySpan {
	start := time.Unix(1700000000, 0)
	linked := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{0x0a},
		SpanID:  trace.SpanID{0x0b},
	})
	return tracetest.SpanStub{
		Name:      "SELECT users",
		StartTime: start,
		EndTime:   start.Add(1500 * time.Microsecond),
		Resource: resource.NewSchemaless(
			attribute.String("service.version", "1.2.3"),
			attribute.String("service.name", "billing"),
		),
		Attributes: []attribute.KeyValue{
			attribute.String("db.statement", "SELECT * FROM пользователи WHERE id=$1"),
			attribute.Int64("http.status_code", 500),
			attribute.Bool("error", true),
		},
		Events: []sdktrace.Event{{
			Name:       "exception",
			Time:       start,
			Attributes: []attribute.KeyValue{attribute.String("exception.message", "boom")},
		}},
		Links: []sdktrace.Link{{
			SpanContext: linked,
			Attributes:  []attribute.KeyValue{attribute.String("reason", "retry")},
		}},
	}.Snapshot()
}

func TestSpanToProto(t *testing.T) {
	got := spanToProto(testSpan())

	if got.ServiceName != "billing" {
		t.Errorf("ServiceName = %q, want the service.name attribute %q", got.ServiceName, "billing")
	}
	if got.Duration != 1500 {
		t.Errorf("Duration = %d, want 1500 (microseconds)", got.Duration)
	}
	wantAttrs := map[string]string{
		"db.statement":     "SELECT * FROM пользователи WHERE id=$1",
		"http.status_code": "500",
		"error":            "true",
	}
	for key, want := range wantAttrs {
		if got.SpanAttributes[key] != want {
			t.Errorf("SpanAttributes[%q] = %q, want %q", key, got.SpanAttributes[key], want)
		}
	}

	if len(got.Events_Name) != 1 || got.Events_Name[0] != "exception" || len(got.Events_Timestamp) != 1 {
		t.Fatalf("events not converted: names=%v timestamps=%v", got.Events_Name, got.Events_Timestamp)
	}
	var eventAttrs map[string]string
	if err := json.Unmarshal([]byte(got.Events_Attributes[0].Value), &eventAttrs); err != nil {
		t.Fatalf("event attributes are not JSON: %v", err)
	}
	if eventAttrs["exception.message"] != "boom" {
		t.Errorf("event attributes = %v", eventAttrs)
	}

	if len(got.Links_SpanId) != 1 || got.Links_SpanId[0] != (trace.SpanID{0x0b}).String() || len(got.Links_Attributes) != 1 {
		t.Errorf("links not converted: span ids=%v attributes=%v", got.Links_SpanId, got.Links_Attributes)
	}
}

func TestToValidUTF8KeepsCyrillic(t *testing.T) {
	if got := toValidUTF8("Привет"); got != "Привет" {
		t.Errorf("toValidUTF8 changed valid text: %q", got)
	}
	if got := toValidUTF8("a\xffb"); got != "a�b" {
		t.Errorf("toValidUTF8 did not replace invalid bytes: %q", got)
	}
}

type fakeReceiver struct {
	pb.UnimplementedLogServiceServer
	mu    sync.Mutex
	spans []*pb.OneSpan
	auth  []string
}

func (f *fakeReceiver) SendSpans(ctx context.Context, req *pb.Spans) (*pb.LogMessageResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spans = append(f.spans, req.Spans...)
	f.auth = append(f.auth, md.Get("authorization")...)
	return &pb.LogMessageResponse{Success: true}, nil
}

func TestExporterSendsSpansAndSurvivesReceiverOutage(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	receiver := &fakeReceiver{}
	server := grpc.NewServer()
	pb.RegisterLogServiceServer(server, receiver)
	go server.Serve(lis)

	exporter := NewCustomExporter(WithAddress(lis.Addr().String()), WithToken("secret"))
	defer exporter.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exporter.ExportSpans(ctx, []sdktrace.ReadOnlySpan{testSpan()}); err != nil {
		t.Fatalf("ExportSpans: %v", err)
	}
	if len(receiver.spans) != 1 || len(receiver.auth) != 1 || receiver.auth[0] != "Bearer secret" {
		t.Fatalf("receiver got spans=%d auth=%v", len(receiver.spans), receiver.auth)
	}

	// With the receiver gone, exporting must return an error instead of exiting.
	server.Stop()
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := exporter.ExportSpans(ctx2, []sdktrace.ReadOnlySpan{testSpan()}); err == nil {
		t.Fatal("ExportSpans succeeded with the receiver stopped")
	}
}
