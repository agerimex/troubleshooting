package sender

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.16.0"
	"go.opentelemetry.io/otel/trace"

	pb "github.com/agerimex/troubleshooting/protos/logs"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TokenEnv is the environment variable holding the shared token that the
// receiver checks when it is started with the same variable. WithToken overrides it.
const TokenEnv = "TROUBLESHOOTING_TOKEN"

var (
	// addrTrace is kept so existing flag.Set("addrTrace", ...) calls keep working.
	// The library never calls flag.Parse: the value is read once, in NewTracer.
	// Prefer WithAddress.
	addrTrace = flag.String("addrTrace", "localhost:50055", "address of the troubleshooting log-receiver")
)

type config struct {
	address string
	token   string
}

// Option configures NewTracer and NewCustomExporter.
type Option func(*config)

// WithAddress sets the log-receiver address (host:port). It takes precedence
// over the addrTrace flag.
func WithAddress(address string) Option {
	return func(c *config) { c.address = address }
}

// WithToken sets the shared token sent to the receiver. It takes precedence
// over the TROUBLESHOOTING_TOKEN environment variable.
func WithToken(token string) Option {
	return func(c *config) { c.token = token }
}

func newConfig(opts []Option) config {
	cfg := config{address: *addrTrace, token: os.Getenv(TokenEnv)}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// CustomExporter sends spans to the log-receiver over a single long-lived
// gRPC connection. Export errors are returned to the span processor, which
// reports them through otel.Handle (logged by default); they never stop the
// host application.
type CustomExporter struct {
	conn   *grpc.ClientConn
	client pb.LogServiceClient
	token  string
	err    error
}

func NewCustomExporter(opts ...Option) *CustomExporter {
	cfg := newConfig(opts)
	// grpc.Dial does not block: the connection is established in the
	// background and re-established if the receiver restarts.
	conn, err := grpc.Dial(cfg.address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return &CustomExporter{err: fmt.Errorf("troubleshooting: connect to %s: %w", cfg.address, err)}
	}
	return &CustomExporter{conn: conn, client: pb.NewLogServiceClient(conn), token: cfg.token}
}

func (e *CustomExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if e.err != nil {
		return e.err
	}
	if e.client == nil {
		return errors.New("troubleshooting: exporter is not connected, create it with NewCustomExporter")
	}

	protoSpans := make([]*pb.OneSpan, 0, len(spans))
	for _, span := range spans {
		protoSpans = append(protoSpans, spanToProto(span))
	}

	if e.token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+e.token)
	}
	if _, err := e.client.SendSpans(ctx, &pb.Spans{Spans: protoSpans}); err != nil {
		return fmt.Errorf("troubleshooting: send %d spans: %w", len(protoSpans), err)
	}
	return nil
}

// Shutdown closes the connection. The span processor calls it after the final flush.
func (e *CustomExporter) Shutdown(ctx context.Context) error {
	if e.conn == nil {
		return nil
	}
	return e.conn.Close()
}

// toValidUTF8 keeps valid UTF-8 (including non-Latin text) as is and replaces
// invalid byte sequences, which protobuf refuses to marshal in string fields.
func toValidUTF8(input string) string {
	return strings.ToValidUTF8(input, "�")
}

// attributesToMap formats values of every type (string, int, bool, float, slices).
func attributesToMap(attrs []attribute.KeyValue) map[string]string {
	result := make(map[string]string, len(attrs))
	for _, kv := range attrs {
		result[toValidUTF8(string(kv.Key))] = toValidUTF8(kv.Value.Emit())
	}
	return result
}

// encodeAttributes packs the attributes of one event or link into a single
// pb.Attribute: Key is the event name (empty for links) and Value is a JSON
// object. The receiver decodes it into one Map per event/link.
func encodeAttributes(name string, attrs []attribute.KeyValue) *pb.Attribute {
	value, err := json.Marshal(attributesToMap(attrs))
	if err != nil {
		value = []byte("{}")
	}
	return &pb.Attribute{Key: toValidUTF8(name), Value: string(value)}
}

func spanToProto(span sdktrace.ReadOnlySpan) *pb.OneSpan {
	protoSpan := &pb.OneSpan{
		Timestamp:          timestamppb.New(span.StartTime()),
		TraceId:            span.SpanContext().TraceID().String(),
		SpanId:             span.SpanContext().SpanID().String(),
		ParentSpanId:       span.Parent().SpanID().String(),
		TraceState:         toValidUTF8(span.Parent().TraceState().String()),
		SpanName:           toValidUTF8(span.Name()),
		SpanKind:           span.SpanKind().String(),
		ResourceAttributes: attributesToMap(span.Resource().Attributes()),
		SpanAttributes:     attributesToMap(span.Attributes()),
		ChildSpanCount:     int32(span.ChildSpanCount()),
		StatusCode:         int32(span.Status().Code),
		StatusMessage:      toValidUTF8(span.Status().Description),
		Duration:           span.EndTime().UnixMicro() - span.StartTime().UnixMicro(),
	}

	if serviceName, ok := span.Resource().Set().Value(semconv.ServiceNameKey); ok {
		protoSpan.ServiceName = toValidUTF8(serviceName.Emit())
	}

	for _, event := range span.Events() {
		protoSpan.Events_Timestamp = append(protoSpan.Events_Timestamp, timestamppb.New(event.Time))
		protoSpan.Events_Name = append(protoSpan.Events_Name, toValidUTF8(event.Name))
		protoSpan.Events_Attributes = append(protoSpan.Events_Attributes, encodeAttributes(event.Name, event.Attributes))
	}

	for _, link := range span.Links() {
		protoSpan.Links_TraceId = append(protoSpan.Links_TraceId, link.SpanContext.TraceID().String())
		protoSpan.Links_SpanId = append(protoSpan.Links_SpanId, link.SpanContext.SpanID().String())
		protoSpan.Links_TraceState = append(protoSpan.Links_TraceState, toValidUTF8(link.SpanContext.TraceState().String()))
		protoSpan.Links_Attributes = append(protoSpan.Links_Attributes, encodeAttributes("", link.Attributes))
	}

	return protoSpan
}

var (
	providerMu sync.Mutex
	provider   *sdktrace.TracerProvider
)

// NewTracer installs a global TracerProvider that sends spans to the
// log-receiver. Call Shutdown before the application exits so buffered spans
// are not lost.
func NewTracer(svcName string, opts ...Option) (trace.Tracer, error) {
	customExporter := NewCustomExporter(opts...)
	if customExporter.err != nil {
		return nil, customExporter.err
	}

	batcher := sdktrace.NewBatchSpanProcessor(customExporter)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(batcher),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(svcName),
		)),
	)

	providerMu.Lock()
	provider = tp
	providerMu.Unlock()

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return otel.Tracer(svcName), nil
}

// Shutdown flushes buffered spans and closes the connection of the provider
// created by the most recent NewTracer call.
func Shutdown(ctx context.Context) error {
	providerMu.Lock()
	tp := provider
	provider = nil
	providerMu.Unlock()

	if tp == nil {
		return nil
	}
	return tp.Shutdown(ctx)
}
