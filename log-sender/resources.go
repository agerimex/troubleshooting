package sender

import (
	"context"
	"os"
	"runtime"
	"runtime/metrics"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.16.0"
)

// Process resources (WithProcessResources).
//
// The machine and the process are described once, as resource attributes of
// every span: host name, process id, CPU count, the machine's memory and the
// process's memory limit (its container's, when it has one).
//
// Every entry call of the process (a span without a parent in this process:
// an incoming request, a background job) gets what the process used while it
// ran and how much memory it held at its end. Go can't attribute CPU time or
// allocations to one request, so these are the whole process's figures for
// the time of the call; AttrCallOverlapping says how many other entry calls
// ran at the same time. With 0 the figures are the call's own (plus the
// runtime's background work); with more, they are shared.
const (
	AttrHostCPUCount    = "troubleshooting.host.cpu_count"
	AttrHostMemory      = "troubleshooting.host.memory_bytes"
	AttrProcessMemLimit = "troubleshooting.process.memory_limit_bytes"

	AttrCallCPU         = "troubleshooting.call.process_cpu_us"
	AttrCallAllocated   = "troubleshooting.call.allocated_bytes"
	AttrCallOverlapping = "troubleshooting.call.overlapping_calls"
	AttrCallHeap        = "troubleshooting.call.heap_bytes"
	AttrCallRSS         = "troubleshooting.call.rss_bytes"
	AttrCallContainer   = "troubleshooting.call.container_memory_bytes"
)

// WithProcessResources records the machine and process resources described
// above. Off by default: it reads a few process counters at the start and end
// of every entry call.
func WithProcessResources() Option {
	return func(c *config) { c.resources = true }
}

// resourceAttributes describes the machine and the process.
func resourceAttributes() []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.ProcessPIDKey.Int(os.Getpid()),
		attribute.Int(AttrHostCPUCount, runtime.NumCPU()),
	}
	if host, err := os.Hostname(); err == nil && host != "" {
		attrs = append(attrs, semconv.HostNameKey.String(host))
	}
	if total, ok := hostMemory(); ok {
		attrs = append(attrs, attribute.Int64(AttrHostMemory, int64(total)))
	}
	if limit, ok := memoryLimit(); ok {
		attrs = append(attrs, attribute.Int64(AttrProcessMemLimit, int64(limit)))
	}
	return attrs
}

// Bounds of the tracker's maps: spans that never end, or end without being
// exported (a full export queue drops them), must not grow memory.
const (
	maxOpenCalls    = 10000
	maxPendingCalls = 10000
)

type spanKey [24]byte

func keyOf(traceID [16]byte, spanID [8]byte) spanKey {
	var k spanKey
	copy(k[:16], traceID[:])
	copy(k[16:], spanID[:])
	return k
}

type callStart struct {
	cpu       time.Duration
	cpuOK     bool
	allocated uint64
	inflight  int    // entry calls running when it started, itself included
	seq       uint64 // entry calls started before it, itself included
}

// resourceTracker is a span processor that measures entry calls. It runs
// before the batch processor, so a call's figures are ready when the exporter
// converts its span (spans are read-only once they end).
type resourceTracker struct {
	mu       sync.Mutex
	inflight int
	started  uint64
	open     map[spanKey]callStart
	done     map[spanKey][]attribute.KeyValue
}

func newResourceTracker() *resourceTracker {
	return &resourceTracker{open: make(map[spanKey]callStart), done: make(map[spanKey][]attribute.KeyValue)}
}

func isEntry(s sdktrace.ReadOnlySpan) bool {
	parent := s.Parent()
	return !parent.IsValid() || parent.IsRemote()
}

func (t *resourceTracker) OnStart(_ context.Context, s sdktrace.ReadWriteSpan) {
	if !isEntry(s) {
		return
	}
	cpu, cpuOK := processCPU()
	allocated := heapAllocated()
	key := keyOf(s.SpanContext().TraceID(), s.SpanContext().SpanID())

	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.open) >= maxOpenCalls {
		return
	}
	t.inflight++
	t.started++
	t.open[key] = callStart{cpu: cpu, cpuOK: cpuOK, allocated: allocated, inflight: t.inflight, seq: t.started}
}

func (t *resourceTracker) OnEnd(s sdktrace.ReadOnlySpan) {
	key := keyOf(s.SpanContext().TraceID(), s.SpanContext().SpanID())
	t.mu.Lock()
	start, ok := t.open[key]
	if ok {
		delete(t.open, key)
		t.inflight--
	}
	started := t.started
	t.mu.Unlock()
	if !ok {
		return
	}

	attrs := callAttributes(start, overlapping(start, started))
	t.mu.Lock()
	if len(t.done) < maxPendingCalls {
		t.done[key] = attrs
	}
	t.mu.Unlock()
}

// overlapping counts the other entry calls that ran during a call: those
// running when it started and those started before it ended.
func overlapping(start callStart, startedNow uint64) int {
	return start.inflight - 1 + int(startedNow-start.seq)
}

func callAttributes(start callStart, others int) []attribute.KeyValue {
	attrs := []attribute.KeyValue{attribute.Int(AttrCallOverlapping, others)}
	if cpu, ok := processCPU(); ok && start.cpuOK && cpu >= start.cpu {
		attrs = append(attrs, attribute.Int64(AttrCallCPU, (cpu-start.cpu).Microseconds()))
	}
	if allocated := heapAllocated(); allocated >= start.allocated {
		attrs = append(attrs, attribute.Int64(AttrCallAllocated, int64(allocated-start.allocated)))
	}
	if heap := heapInUse(); heap > 0 {
		attrs = append(attrs, attribute.Int64(AttrCallHeap, int64(heap)))
	}
	if rss, ok := residentMemory(); ok {
		attrs = append(attrs, attribute.Int64(AttrCallRSS, int64(rss)))
	}
	if used, ok := containerMemory(); ok {
		attrs = append(attrs, attribute.Int64(AttrCallContainer, int64(used)))
	}
	return attrs
}

// take returns and forgets the figures of an ended entry call.
func (t *resourceTracker) take(traceID [16]byte, spanID [8]byte) []attribute.KeyValue {
	key := keyOf(traceID, spanID)
	t.mu.Lock()
	defer t.mu.Unlock()
	attrs, ok := t.done[key]
	if ok {
		delete(t.done, key)
	}
	return attrs
}

func (t *resourceTracker) Shutdown(context.Context) error   { return nil }
func (t *resourceTracker) ForceFlush(context.Context) error { return nil }

func readMetric(name string) uint64 {
	sample := []metrics.Sample{{Name: name}}
	metrics.Read(sample)
	if sample[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return sample[0].Value.Uint64()
}

// heapAllocated is the total of bytes the process has allocated on the heap.
func heapAllocated() uint64 { return readMetric("/gc/heap/allocs:bytes") }

// heapInUse is the memory held by live and not yet collected heap objects.
func heapInUse() uint64 { return readMetric("/memory/classes/heap/objects:bytes") }
