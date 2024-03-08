package sender

import (
	"bytes"
	"context"
	"flag"
	"log"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.16.0"
	"go.opentelemetry.io/otel/trace"

	pb "github.com/artemparygin/troubleshooting/protos/logs"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	addrTrace = flag.String("addrTrace", "localhost:50055", "default address")
)

type CustomExporter struct{}

func NewCustomExporter() *CustomExporter {
	return &CustomExporter{}
}

func convertToUTF82(input string) string {
	reader := transform.NewReader(bytes.NewReader([]byte(input)), charmap.Windows1252.NewDecoder())
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(reader)
	if err != nil {
		return ""
	}
	return buf.String()
}

func writeTraceToBackend(spans []sdktrace.ReadOnlySpan) {
	flag.Parse()
	conn, err := grpc.Dial(*addrTrace, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewLogServiceClient(conn)

	var sta []*pb.OneSpan

	for _, span := range spans {
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
		}

		protoSpan.ResourceAttributes = make(map[string]string)
		protoSpan.SpanAttributes = make(map[string]string)

		for _, value := range span.Resource().Attributes() {
			protoSpan.ResourceAttributes[string(value.Key)] = convertToUTF82(value.Value.AsString())
		}

		for _, value := range span.Attributes() {
			if value.Value.Type() == attribute.INT64 {
				protoSpan.SpanAttributes[string(value.Key)] = strconv.FormatInt(value.Value.AsInt64(), 10)
			} else {
				protoSpan.SpanAttributes[string(value.Key)] = convertToUTF82(value.Value.AsString())
			}
		}

		for _, value := range span.Resource().Attributes() {
			if strings.Contains(string(value.Key), "service") {
				protoSpan.ServiceName = convertToUTF82(value.Value.AsString())
				break
			}
		}

		sta = append(sta, protoSpan)
	}

	_, err = c.SendSpans(context.Background(), &pb.Spans{Spans: sta})
	if err != nil {
		log.Fatalf("Error sending log message: %v", err)
	}
}

func (e *CustomExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	writeTraceToBackend(spans)

	return nil
}

func (e *CustomExporter) Shutdown(ctx context.Context) error {
	return nil
}

func NewTracer(svcName string) (trace.Tracer, error) {
	customExporter := NewCustomExporter()

	batcher := sdktrace.NewBatchSpanProcessor(customExporter)

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

	return otel.Tracer(svcName), nil
}
