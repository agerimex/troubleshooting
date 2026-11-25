package sender

import (
	"context"
	"runtime"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func trackedProvider() (*resourceTracker, trace.Tracer) {
	tracker := newResourceTracker()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(tracker))
	return tracker, tp.Tracer("test")
}

func attrsMap(attrs []attribute.KeyValue) map[string]attribute.Value {
	m := make(map[string]attribute.Value, len(attrs))
	for _, kv := range attrs {
		m[string(kv.Key)] = kv.Value
	}
	return m
}

func takeSpan(t *resourceTracker, s trace.Span) map[string]attribute.Value {
	sc := s.SpanContext()
	return attrsMap(t.take(sc.TraceID(), sc.SpanID()))
}

func TestEntryCallGetsResourcesChildDoesNot(t *testing.T) {
	tracker, tracer := trackedProvider()
	ctx, root := tracer.Start(context.Background(), "GET /x")
	_, child := tracer.Start(ctx, "clickhouse SELECT")
	buf := make([]byte, 0)
	for i := 0; i < 1000; i++ {
		buf = append(buf, make([]byte, 1024)...)
	}
	runtime.KeepAlive(buf)
	child.End()
	root.End()

	if got := takeSpan(tracker, child); len(got) != 0 {
		t.Fatalf("child span got resources: %v", got)
	}
	got := takeSpan(tracker, root)
	if v, ok := got[AttrCallOverlapping]; !ok || v.AsInt64() != 0 {
		t.Fatalf("overlapping = %v, want 0", got[AttrCallOverlapping])
	}
	if v := got[AttrCallAllocated]; v.AsInt64() < 1000*1024 {
		t.Fatalf("allocated = %d, want at least the 1 MB allocated in the call", v.AsInt64())
	}
	if _, ok := got[AttrCallCPU]; !ok && runtime.GOOS != "js" {
		t.Fatalf("no CPU time recorded: %v", got)
	}
	if again := takeSpan(tracker, root); len(again) != 0 {
		t.Fatal("take must forget the figures")
	}
}

func TestOverlappingCalls(t *testing.T) {
	tracker, tracer := trackedProvider()
	_, a := tracer.Start(context.Background(), "a")
	_, b := tracer.Start(context.Background(), "b") // runs while a runs
	b.End()
	_, c := tracer.Start(context.Background(), "c") // starts while a runs, after b
	c.End()
	a.End()
	_, d := tracer.Start(context.Background(), "d") // alone
	d.End()

	want := map[string]int64{"a": 2, "b": 1, "c": 1, "d": 0}
	for name, span := range map[string]trace.Span{"a": a, "b": b, "c": c, "d": d} {
		if got := takeSpan(tracker, span)[AttrCallOverlapping].AsInt64(); got != want[name] {
			t.Errorf("%s: overlapping = %d, want %d", name, got, want[name])
		}
	}
}

func TestRemoteParentIsAnEntry(t *testing.T) {
	tracker, tracer := trackedProvider()
	remote := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: [16]byte{1}, SpanID: [8]byte{2}, TraceFlags: trace.FlagsSampled, Remote: true,
	})
	_, s := tracer.Start(trace.ContextWithRemoteSpanContext(context.Background(), remote), "GET /y")
	s.End()
	if _, ok := takeSpan(tracker, s)[AttrCallOverlapping]; !ok {
		t.Fatal("a span with a remote parent is an entry call of this process")
	}
}

func TestTrackerIsBounded(t *testing.T) {
	tracker, tracer := trackedProvider()
	for i := 0; i < maxPendingCalls+10; i++ {
		_, s := tracer.Start(context.Background(), "x")
		s.End()
	}
	if len(tracker.done) > maxPendingCalls || len(tracker.open) != 0 {
		t.Fatalf("done %d (max %d), open %d", len(tracker.done), maxPendingCalls, len(tracker.open))
	}
}

func TestResourceAttributesDescribeTheProcess(t *testing.T) {
	got := attrsMap(resourceAttributes())
	if got["process.pid"].AsInt64() <= 0 || got[AttrHostCPUCount].AsInt64() != int64(runtime.NumCPU()) {
		t.Fatalf("resource attributes: %v", got)
	}
	if runtime.GOOS == "linux" && got[AttrHostMemory].AsInt64() <= 0 {
		t.Fatalf("no host memory on linux: %v", got)
	}
}
