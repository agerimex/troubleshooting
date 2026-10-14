package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	sender "github.com/agerimex/troubleshooting/log-sender"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// Parity: the same two spans, recorded with the OpenTelemetry Go SDK, are
// sent once through log-sender (custom SendSpans, port 50055) and once
// through the SDK's own OTLP exporter (port 4317), under different trace ids.
// Both must read back with the same identity structure, status, duration
// (microseconds), events and links.

var paritySpanNames = []string{"parity.request", "parity.child"}

// fixedIDs hands out the span ids in order, for a fixed trace id.
type fixedIDs struct {
	trace trace.TraceID
	spans []trace.SpanID
}

func (g *fixedIDs) next() trace.SpanID {
	id := g.spans[0]
	g.spans = g.spans[1:]
	return id
}

func (g *fixedIDs) NewIDs(context.Context) (trace.TraceID, trace.SpanID)  { return g.trace, g.next() }
func (g *fixedIDs) NewSpanID(context.Context, trace.TraceID) trace.SpanID { return g.next() }

func mustTraceID(s string) trace.TraceID {
	id, err := trace.TraceIDFromHex(s)
	if err != nil {
		panic(err)
	}
	return id
}

func mustSpanID(s string) trace.SpanID {
	id, err := trace.SpanIDFromHex(s)
	if err != nil {
		panic(err)
	}
	return id
}

func (d dataset) parityTrace(path int) string { return d.traceID(scPar, path) }

// recordParity records the parity spans for one path.
func (d dataset) recordParity(path int, service string) []sdktrace.ReadOnlySpan {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
		sdktrace.WithIDGenerator(&fixedIDs{trace: mustTraceID(d.parityTrace(path)), spans: []trace.SpanID{mustSpanID(idRoot), mustSpanID(idChild1)}}),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", service))),
	)
	tracer := provider.Tracer("fixture-parity")
	t0 := d.at(-9 * time.Minute)

	link := trace.Link{
		SpanContext: trace.NewSpanContext(trace.SpanContextConfig{
			TraceID: mustTraceID(d.traceID(scRF04, 1)), SpanID: mustSpanID(idRoot), TraceFlags: trace.FlagsSampled,
		}),
		Attributes: []attribute.KeyValue{attribute.String("link.reason", "parity")},
	}
	ctx, root := tracer.Start(context.Background(), "parity.request", trace.WithTimestamp(t0), trace.WithLinks(link),
		trace.WithAttributes(attribute.String("http.method", "POST"), attribute.Int("attempt", 2), attribute.Bool("retried", true)))
	_, child := tracer.Start(ctx, "parity.child", trace.WithTimestamp(t0.Add(10*time.Millisecond)))
	child.End(trace.WithTimestamp(t0.Add(50 * time.Millisecond)))
	root.AddEvent("exception", trace.WithTimestamp(t0.Add(200*time.Millisecond)), trace.WithAttributes(
		attribute.String("exception.type", exceptionType),
		attribute.String("exception.message", exceptionMessage),
		attribute.String("exception.stacktrace", exceptionStack)))
	root.SetStatus(codes.Error, "parity failure")
	root.End(trace.WithTimestamp(t0.Add(250 * time.Millisecond)))
	_ = provider.Shutdown(context.Background())
	return recorder.Ended()
}

const (
	parityViaSender = 1
	parityViaOTLP   = 2
)

func sendParity(ctx context.Context, d dataset, senderAddr, otlpAddr string) error {
	viaSender := sender.NewCustomExporter(sender.WithAddress(senderAddr))
	defer viaSender.Shutdown(ctx)
	if err := viaSender.ExportSpans(ctx, d.recordParity(parityViaSender, d.service("parity-sender"))); err != nil {
		return fmt.Errorf("log-sender path: %w", err)
	}

	viaOTLP, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(otlpAddr), otlptracegrpc.WithInsecure())
	if err != nil {
		return err
	}
	defer viaOTLP.Shutdown(ctx)
	if err := viaOTLP.ExportSpans(ctx, d.recordParity(parityViaOTLP, d.service("parity-otlp"))); err != nil {
		return fmt.Errorf("OTLP path: %w", err)
	}
	return nil
}

// comparable is what both paths must agree on. Span attributes leave out
// otel.scope.*, which only the OTLP path stores (log-sender doesn't send the
// instrumentation scope).
type comparableSpan struct {
	SpanID, ParentSpanID, Status, StatusMessage string
	Duration                                    int64
	Attributes                                  map[string]string
	Events                                      []apiEvent
	Links                                       []apiLink
}

func comparableOf(s apiSpan) comparableSpan {
	attrs := map[string]string{}
	for _, t := range s.Tags {
		if !strings.HasPrefix(t["key"], "otel.scope.") {
			attrs[t["key"]] = t["value"]
		}
	}
	return comparableSpan{s.SpanID, s.ParentSpanID, s.Status, s.StatusMessage, s.Duration, attrs, s.Events, s.Links}
}

func (c *checker) parity(ctx context.Context) error {
	byPath := map[int]map[string]comparableSpan{}
	for _, path := range []int{parityViaSender, parityViaOTLP} {
		spans, err := c.trace(ctx, c.window(), c.d.parityTrace(path))
		if err != nil {
			return err
		}
		if err := expectNames(fmt.Sprintf("path %d", path), spans, paritySpanNames...); err != nil {
			return err
		}
		byPath[path] = map[string]comparableSpan{}
		for _, s := range spans {
			byPath[path][s.Name] = comparableOf(s)
		}
	}

	// Absolute expectations, then equality of the two paths.
	t0 := c.d.at(-9 * time.Minute)
	root := byPath[parityViaSender]["parity.request"]
	child := byPath[parityViaSender]["parity.child"]
	if root.SpanID != idRoot || root.ParentSpanID != "0000000000000000" || child.ParentSpanID != idRoot {
		return fmt.Errorf("log-sender path: identity %s/%s, child's parent %s", root.SpanID, root.ParentSpanID, child.ParentSpanID)
	}
	if root.Status != "error" || root.StatusMessage != "parity failure" || child.Status != "unset" {
		return fmt.Errorf("log-sender path: statuses %q %q / %q", root.Status, root.StatusMessage, child.Status)
	}
	if root.Duration != 250000 || child.Duration != 40000 {
		return fmt.Errorf("log-sender path: durations %d / %d µs, want 250000 / 40000", root.Duration, child.Duration)
	}
	if len(root.Events) != 1 || root.Events[0].TimeUnixNano != ns(t0.Add(200*time.Millisecond)) ||
		root.Events[0].Attributes["exception.stacktrace"] != exceptionStack {
		return fmt.Errorf("log-sender path: events %+v", root.Events)
	}
	wantLink := apiLink{TraceID: c.d.traceID(scRF04, 1), SpanID: idRoot, Attributes: map[string]string{"link.reason": "parity"}}
	if len(root.Links) != 1 || !reflect.DeepEqual(root.Links[0], wantLink) {
		return fmt.Errorf("log-sender path: links %+v", root.Links)
	}
	if want := map[string]string{"http.method": "POST", "attempt": "2", "retried": "true"}; !reflect.DeepEqual(root.Attributes, want) {
		return fmt.Errorf("log-sender path: attributes %v, want %v", root.Attributes, want)
	}
	for _, name := range paritySpanNames {
		if a, b := byPath[parityViaSender][name], byPath[parityViaOTLP][name]; !reflect.DeepEqual(a, b) {
			return fmt.Errorf("%s differs:\n  log-sender %+v\n  OTLP       %+v", name, a, b)
		}
	}
	return nil
}

// invalidIDs sends spans with invalid ids and expects them rejected while the
// valid span of the same request is stored.
func (c *checker) invalidIDs(ctx context.Context, otlp *otlpSender) error {
	rejected, err := otlp.send(ctx, c.d.invalidIDs())
	if err != nil {
		return err
	}
	if rejected != 3 {
		return fmt.Errorf("receiver rejected %d spans, want 3", rejected)
	}
	stored, err := c.api.all(ctx, c.service(c.window(), "invalid-ids", "all"))
	if err != nil {
		return err
	}
	return expectNames("stored spans", stored, "valid")
}
