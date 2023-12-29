package sender

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"go.opentelemetry.io/otel"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"

	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.16.0"
	"go.opentelemetry.io/otel/trace"

	pb "log-api/logs"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultNameTrace = "world"
)

var (
	addrTrace = flag.String("addrTrace", "localhost:50052", "the address to connect to")
	nameTrace = flag.String("nameTrace", defaultNameTrace, "Name to greet")
)

// CustomExporter is a simple custom exporter that prints trace information to the console.
type CustomExporter struct{}

// NewCustomExporter creates a new instance of the CustomExporter.
func NewCustomExporter() *CustomExporter {
	return &CustomExporter{}
}

func convertToUTF82(input string) string {
	// Convert from Windows-1252 to UTF-8
	reader := transform.NewReader(bytes.NewReader([]byte(input)), charmap.Windows1252.NewDecoder())
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(reader)
	if err != nil {
		return ""
	}
	return buf.String()

	//return input
}

func writeTraceToBackend(spans []sdktrace.ReadOnlySpan) {
	flag.Parse()
	// Set up a connection to the server.
	conn, err := grpc.Dial(*addrTrace, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewLogServiceClient(conn)

	var sta []*pb.OneSpan

	for _, span := range spans {
		fmt.Println("span time", span.EndTime().UnixMicro()-span.StartTime().UnixMicro())
		fmt.Println("span status", span.Status().Description, span.Status().Code.String())
		fmt.Println("span", span)
		fmt.Println("\n\n\n\n")
		protoSpan := &pb.OneSpan{
			Timestamp:      timestamppb.New(span.StartTime()),
			TraceId:        convertToUTF82(span.SpanContext().TraceID().String()),
			SpanId:         convertToUTF82(span.SpanContext().SpanID().String()),
			ParentSpanId:   convertToUTF82(span.Parent().SpanID().String()),
			TraceState:     convertToUTF82(span.Parent().TraceState().String()),
			SpanName:       convertToUTF82(span.Name()),
			SpanKind:       convertToUTF82(span.SpanKind().String()),
			ChildSpanCount: int32(span.ChildSpanCount()),
			StatusCode:     int32(span.Status().Code),
			StatusMessage:  convertToUTF82(span.Status().Description),
			Duration:       span.EndTime().UnixMicro() - span.StartTime().UnixMicro(),
			// ServiceName:  span.Resource().String(),
		}

		protoSpan.ResourceAttributes = make(map[string]string)
		protoSpan.SpanAttributes = make(map[string]string)

		// Iterate over the ResourceAttributes and populate the map.
		for _, value := range span.Resource().Attributes() {
			protoSpan.ResourceAttributes[string(value.Key)] = convertToUTF82(value.Value.AsString())
		}

		// // Iterate over the SpanAttributes and populate the map.
		for _, value := range span.Attributes() {
			protoSpan.SpanAttributes[string(value.Key)] = convertToUTF82(value.Value.AsString())
		}

		for _, value := range span.Resource().Attributes() {
			if strings.Contains(string(value.Key), "service") {
				protoSpan.ServiceName = convertToUTF82(value.Value.AsString())
				break
			}
		}

		sta = append(sta, protoSpan)
	}

	response, err := c.SendSpans(context.Background(), &pb.Spans{Spans: sta})
	if err != nil {
		log.Fatalf("Error sending log message: %v", err)
	}

	if response.Success {
		log.Println("Log message sent successfully")
	}
}

// ExportSpans exports trace spans to the custom storage backend (in this case, prints to the console).
func (e *CustomExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	writeTraceToBackend(spans)

	return nil
}

// Shutdown is called when the exporter is shut down.
func (e *CustomExporter) Shutdown(ctx context.Context) error {
	// Implement any necessary cleanup logic here.
	return nil
}

func InitTracer(jaegerURL string, serviceName string) (trace.Tracer, error) {
	exporter, err := NewJaegerExporter(jaegerURL)
	if err != nil {
		return nil, fmt.Errorf("initialize exporter: %w", err)
	}

	tp, err := NewTraceProvider(exporter, serviceName)
	if err != nil {
		return nil, fmt.Errorf("initialize provider: %w", err)
	}

	otel.SetTracerProvider(tp)

	return tp.Tracer("main tracer"), nil
}

// NewJaegerExporter creates new jaeger exporter
//
//	url example - http://localhost:14268/api/traces
func NewJaegerExporter(url string) (tracesdk.SpanExporter, error) {
	return jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
}

func NewTraceProvider(exp tracesdk.SpanExporter, ServiceName string) (*tracesdk.TracerProvider, error) {
	// Ensure default SDK resources and the required service name are set.
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(ServiceName),
		),
	)
	if err != nil {
		return nil, err
	}

	return tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithResource(r),
		//tracesdk.WithSampler(tracesdk.Sample)
	), nil
}

func NewTracer(svcName, jaegerEndpoint string) (trace.Tracer, error) {
	customExporter := NewCustomExporter()

	batcher := sdktrace.NewBatchSpanProcessor(customExporter)

	// Create a TracerProvider with the batcher
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(batcher),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(svcName),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	// returns tracer
	return otel.Tracer(svcName), nil
}
