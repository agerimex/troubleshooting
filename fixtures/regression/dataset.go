package main

import (
	"encoding/hex"
	"fmt"
	"time"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// The dataset is generated from a run id and a base time only, so a replay
// can rebuild byte-identical requests from the state file. Every span of a run
// has a trace id that starts with the run id and a service name that starts
// with "fx<run id>", so checks select exactly the fixture's records and never
// see the API's own telemetry or another run.

// Scenario numbers, embedded in trace ids (see traceID).
const (
	scRF01 = 0x0101
	scRF03 = 0x0103
	scRF04 = 0x0104
	scRF05 = 0x0105
	scRF06 = 0x0106
	scRF07 = 0x0107
	scRF08 = 0x0108
	scRF09 = 0x0109
	scRF10 = 0x0110
	scRF12 = 0x0112
	scRF16 = 0x0116
	scPar  = 0x0200
	scBad  = 0x0300
)

// childCap is the API's MaxRowsPerPage and the UI's child limit (RF10).
const childCap = 1000

type dataset struct {
	run  string
	base time.Time
}

// traceID is 32 hex digits: run id (8), scenario (4), sequence (20).
func (d dataset) traceID(scenario, n int) string {
	return fmt.Sprintf("%s%04x%020x", d.run, scenario, n)
}

// service names one scenario's service, e.g. fx1a2b3c4d-rf04-api.
func (d dataset) service(suffix string) string {
	return "fx" + d.run + "-" + suffix
}

// at is a time relative to the base, which is ten minutes after the fixture's
// spans: scenarios mostly sit around base-10m.
func (d dataset) at(offset time.Duration) time.Time {
	return d.base.Add(offset)
}

// window is the default search window of the checks: [base-30m, base+5m].
func (d dataset) window() (time.Time, time.Time) {
	return d.at(-30 * time.Minute), d.at(5 * time.Minute)
}

// Fixed span ids. RF01 and RF08 deliberately reuse ids across traces.
const (
	idRoot     = "00000000000000a0"
	idChild1   = "00000000000000a1"
	idChild2   = "00000000000000a2"
	idGrand    = "00000000000000a3"
	idGreat    = "00000000000000a4"
	idShared   = "00000000000000e8"
	idConflict = "00000000000000c3"
	idLateRoot = "00000000000000b1"
	idLateKid  = "00000000000000b2"
)

type spanSpec struct {
	trace, id, parent string // parent "" = root
	name              string
	start             time.Time
	dur               time.Duration
	status            tracepb.Status_StatusCode
	message           string
	attrs             map[string]string
	events            []*tracepb.Span_Event
	links             []*tracepb.Span_Link
	kind              tracepb.Span_SpanKind // unspecified means internal
	noEnd             bool                  // send no end time (an invalid duration)
}

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func kvs(m map[string]string) []*commonpb.KeyValue {
	var out []*commonpb.KeyValue
	for _, k := range sortedKeys(m) {
		out = append(out, &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: m[k]}}})
	}
	return out
}

func (s spanSpec) proto() *tracepb.Span {
	span := &tracepb.Span{
		TraceId:           mustHex(s.trace),
		SpanId:            mustHex(s.id),
		Name:              s.name,
		Kind:              tracepb.Span_SPAN_KIND_INTERNAL,
		StartTimeUnixNano: uint64(s.start.UnixNano()),
		EndTimeUnixNano:   uint64(s.start.Add(s.dur).UnixNano()),
		Attributes:        kvs(s.attrs),
		Status:            &tracepb.Status{Code: s.status, Message: s.message},
		Events:            s.events,
		Links:             s.links,
	}
	if s.parent != "" {
		span.ParentSpanId = mustHex(s.parent)
	}
	if s.kind != tracepb.Span_SPAN_KIND_UNSPECIFIED {
		span.Kind = s.kind
	}
	if s.noEnd {
		span.EndTimeUnixNano = 0
	}
	return span
}

// request wraps spans of one service (with optional extra resource
// attributes) in an OTLP export request.
func request(service string, resource map[string]string, specs ...spanSpec) *collectortracepb.ExportTraceServiceRequest {
	attrs := map[string]string{"service.name": service}
	for k, v := range resource {
		attrs[k] = v
	}
	spans := make([]*tracepb.Span, len(specs))
	for i, s := range specs {
		spans[i] = s.proto()
	}
	return &collectortracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{
		Resource:   &resourcepb.Resource{Attributes: kvs(attrs)},
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: spans}},
	}}}
}

func merge(reqs ...*collectortracepb.ExportTraceServiceRequest) *collectortracepb.ExportTraceServiceRequest {
	out := &collectortracepb.ExportTraceServiceRequest{}
	for _, r := range reqs {
		out.ResourceSpans = append(out.ResourceSpans, r.ResourceSpans...)
	}
	return out
}

// The recorded exception of RF04 and RF07.
const (
	exceptionType    = "AccessDeniedException"
	exceptionMessage = "access denied: s3://recordings/2026/10/10/rec-4711.mp4"
	exceptionStack   = "AccessDeniedException: access denied: s3://recordings/2026/10/10/rec-4711.mp4\n" +
		"\tat storage.(*Client).Put(client.go:118)\n" +
		"\tat upload.(*Worker).uploadRecording(worker.go:64)\n" +
		"\tat upload.(*Worker).Run(worker.go:31)"
)

func (d dataset) exceptionEvent(at time.Time, extra map[string]string) *tracepb.Span_Event {
	attrs := map[string]string{
		"exception.type":       exceptionType,
		"exception.message":    exceptionMessage,
		"exception.stacktrace": exceptionStack,
	}
	for k, v := range extra {
		attrs[k] = v
	}
	return &tracepb.Span_Event{TimeUnixNano: uint64(at.UnixNano()), Name: "exception", Attributes: kvs(attrs)}
}

// rf16Durations are the v1 durations of RF16; v2 takes exactly twice as long.
func rf16Durations() []time.Duration {
	var out []time.Duration
	for i := 0; i < 20; i++ {
		out = append(out, 100*time.Millisecond+time.Duration(i)*5*time.Millisecond)
	}
	return out
}

// deliveries returns the export requests in the order they are sent. Order
// matters only for RF03, which sends conflicting versions in both orders.
// The RF05 parent is not included: see late.
func (d dataset) deliveries() []*collectortracepb.ExportTraceServiceRequest {
	t0 := d.at(-10 * time.Minute)
	ok, failed := tracepb.Status_STATUS_CODE_OK, tracepb.Status_STATUS_CODE_ERROR
	var reqs []*collectortracepb.ExportTraceServiceRequest

	// RF01: two traces whose parents share a span id; different children.
	a, b := d.traceID(scRF01, 1), d.traceID(scRF01, 2)
	reqs = append(reqs, request(d.service("rf01"), nil,
		spanSpec{trace: a, id: idRoot, name: "rf01.parent", start: t0, dur: time.Second, status: ok},
		spanSpec{trace: a, id: idChild1, parent: idRoot, name: "rf01.a.child1", start: t0.Add(time.Millisecond), dur: 10 * time.Millisecond},
		spanSpec{trace: a, id: idChild2, parent: idRoot, name: "rf01.a.child2", start: t0.Add(2 * time.Millisecond), dur: 10 * time.Millisecond},
		spanSpec{trace: b, id: idRoot, name: "rf01.parent", start: t0, dur: time.Second, status: ok},
		spanSpec{trace: b, id: idChild1, parent: idRoot, name: "rf01.b.child1", start: t0.Add(time.Millisecond), dur: 10 * time.Millisecond},
	))

	// RF03: conflicting payloads for one identity, in both arrival orders.
	// Versions ok/error: the error version wins by rule 1. Versions c/d
	// differ only in attributes and duration: rule 4 (hash) decides.
	verOK := func(trace string) spanSpec {
		return spanSpec{trace: trace, id: idConflict, name: "rf03.conflict", start: t0, dur: 100 * time.Millisecond, status: ok,
			attrs: map[string]string{"variant": "ok"}}
	}
	verErr := func(trace string) spanSpec {
		return spanSpec{trace: trace, id: idConflict, name: "rf03.conflict", start: t0, dur: 100 * time.Millisecond, status: failed,
			message: "conflict version error", attrs: map[string]string{"variant": "error"},
			events: []*tracepb.Span_Event{{TimeUnixNano: uint64(t0.Add(50 * time.Millisecond).UnixNano()), Name: "retry"}}}
	}
	verC := func(trace string) spanSpec {
		return spanSpec{trace: trace, id: idConflict, name: "rf03.tie", start: t0, dur: 100 * time.Millisecond, attrs: map[string]string{"variant": "c"}}
	}
	verD := func(trace string) spanSpec {
		return spanSpec{trace: trace, id: idConflict, name: "rf03.tie", start: t0, dur: 200 * time.Millisecond, attrs: map[string]string{"variant": "d"}}
	}
	k1, k2, k3, k4 := d.traceID(scRF03, 1), d.traceID(scRF03, 2), d.traceID(scRF03, 3), d.traceID(scRF03, 4)
	svc3, svc3tie := d.service("rf03-status"), d.service("rf03-tie")
	reqs = append(reqs,
		request(svc3, nil, verOK(k1)), request(svc3, nil, verErr(k1)),
		request(svc3, nil, verErr(k2)), request(svc3, nil, verOK(k2)),
		request(svc3tie, nil, verC(k3)), request(svc3tie, nil, verD(k3)),
		request(svc3tie, nil, verD(k4)), request(svc3tie, nil, verC(k4)),
	)

	// RF04: a successful request whose background upload failed two levels down.
	l := d.traceID(scRF04, 1)
	reqs = append(reqs, merge(
		request(d.service("rf04-api"), nil,
			spanSpec{trace: l, id: idRoot, name: "POST /recordings", start: t0, dur: 300 * time.Millisecond, status: ok}),
		request(d.service("rf04-worker"), nil,
			spanSpec{trace: l, id: idChild1, parent: idRoot, name: "upload pipeline", start: t0.Add(10 * time.Millisecond), dur: 250 * time.Millisecond, status: ok},
			spanSpec{trace: l, id: idGrand, parent: idChild1, name: "UploadToStorage", start: t0.Add(20 * time.Millisecond), dur: 200 * time.Millisecond,
				status: failed, message: "upload failed",
				events: []*tracepb.Span_Event{d.exceptionEvent(t0.Add(219*time.Millisecond), nil)}}),
	))

	// RF05: a child whose parent arrives later (see late).
	reqs = append(reqs, request(d.service("rf05"), nil,
		spanSpec{trace: d.traceID(scRF05, 1), id: idLateKid, parent: idLateRoot, name: "rf05.child", start: t0.Add(5 * time.Millisecond), dur: 10 * time.Millisecond}))

	// RF06: a parent 50 minutes before the base, outside the default window.
	n := d.traceID(scRF06, 1)
	reqs = append(reqs, request(d.service("rf06"), nil,
		spanSpec{trace: n, id: idRoot, name: "rf06.parent", start: d.at(-50 * time.Minute), dur: 41 * time.Minute},
		spanSpec{trace: n, id: idChild1, parent: idRoot, name: "rf06.child", start: t0, dur: 10 * time.Millisecond}))

	// RF07: an exception event with a stack trace, and a link to RF04's root.
	reqs = append(reqs, request(d.service("rf07"), nil,
		spanSpec{trace: d.traceID(scRF07, 1), id: idRoot, name: "rf07.retry-upload", start: t0.Add(time.Second), dur: 50 * time.Millisecond,
			status: failed, message: "retry failed",
			events: []*tracepb.Span_Event{d.exceptionEvent(t0.Add(time.Second+40*time.Millisecond), map[string]string{"retry.count": "3"})},
			links: []*tracepb.Span_Link{{TraceId: mustHex(l), SpanId: mustHex(idRoot),
				Attributes: kvs(map[string]string{"link.reason": "retry of upload"})}}}))

	// RF08: eight roots, seven at the same time, five of them with the same span id.
	tSame := t0.Add(3 * time.Second)
	var rf08 []spanSpec
	for i := 1; i <= 5; i++ {
		rf08 = append(rf08, spanSpec{trace: d.traceID(scRF08, i), id: idShared, name: fmt.Sprintf("rf08.%d", i), start: tSame, dur: time.Millisecond})
	}
	rf08 = append(rf08,
		spanSpec{trace: d.traceID(scRF08, 6), id: "00000000000000e1", name: "rf08.6", start: tSame, dur: time.Millisecond},
		spanSpec{trace: d.traceID(scRF08, 7), id: "00000000000000ef", name: "rf08.7", start: tSame, dur: time.Millisecond},
		spanSpec{trace: d.traceID(scRF08, 8), id: idShared, name: "rf08.8", start: tSame.Add(time.Nanosecond), dur: time.Millisecond})
	reqs = append(reqs, request(d.service("rf08"), nil, rf08...))

	// RF09: four levels.
	p := d.traceID(scRF09, 1)
	reqs = append(reqs, request(d.service("rf09"), nil,
		spanSpec{trace: p, id: idRoot, name: "rf09.level1", start: t0, dur: 40 * time.Millisecond},
		spanSpec{trace: p, id: idChild1, parent: idRoot, name: "rf09.level2", start: t0.Add(time.Millisecond), dur: 30 * time.Millisecond},
		spanSpec{trace: p, id: idGrand, parent: idChild1, name: "rf09.level3", start: t0.Add(2 * time.Millisecond), dur: 20 * time.Millisecond},
		spanSpec{trace: p, id: idGreat, parent: idGrand, name: "rf09.level4", start: t0.Add(3 * time.Millisecond), dur: 10 * time.Millisecond}))

	// RF10: exactly childCap children, and childCap+1.
	for i, count := range []int{childCap, childCap + 1} {
		trace := d.traceID(scRF10, i+1)
		specs := []spanSpec{{trace: trace, id: idRoot, name: fmt.Sprintf("rf10.parent-%d", count), start: t0, dur: time.Second}}
		for c := 1; c <= count; c++ {
			specs = append(specs, spanSpec{trace: trace, id: fmt.Sprintf("%016x", 0x1000+c), parent: idRoot,
				name: "rf10.child", start: t0.Add(time.Duration(c) * time.Microsecond), dur: time.Microsecond})
		}
		reqs = append(reqs, request(d.service("rf10"), nil, specs...))
	}

	// RF12: a child from a clock 2 minutes behind, and an async child 5
	// minutes after the parent ended.
	q := d.traceID(scRF12, 1)
	reqs = append(reqs, request(d.service("rf12"), nil,
		spanSpec{trace: q, id: idRoot, name: "rf12.parent", start: t0, dur: time.Second},
		spanSpec{trace: q, id: idChild1, parent: idRoot, name: "rf12.skewed-child", start: t0.Add(-2 * time.Minute), dur: 10 * time.Millisecond},
		spanSpec{trace: q, id: idChild2, parent: idRoot, name: "rf12.async-child", start: t0.Add(time.Second + 5*time.Minute), dur: 10 * time.Millisecond}))

	// RF16: v1 and v2 of one operation, 20 calls each, v2 twice as slow.
	var v1, v2 []spanSpec
	for i, dur := range rf16Durations() {
		start := t0.Add(time.Duration(i) * time.Second)
		v1 = append(v1, spanSpec{trace: d.traceID(scRF16, 100+i), id: idRoot, name: "TranscodeRecording", start: start, dur: dur, status: ok,
			attrs: map[string]string{"recording.format": "mp4"}})
		v2 = append(v2, spanSpec{trace: d.traceID(scRF16, 200+i), id: idRoot, name: "TranscodeRecording", start: start, dur: 2 * dur, status: ok,
			attrs: map[string]string{"recording.format": "mp4"}})
	}
	reqs = append(reqs,
		request(d.service("rf16"), map[string]string{"service.version": "1.0.0"}, v1...),
		request(d.service("rf16"), map[string]string{"service.version": "2.0.0"}, v2...))

	reqs = append(reqs, d.m1Spans()...)

	return reqs
}

// late is the RF05 parent, delivered only after the check has seen the child
// without it.
func (d dataset) late() *collectortracepb.ExportTraceServiceRequest {
	t0 := d.at(-10 * time.Minute)
	return request(d.service("rf05"), nil,
		spanSpec{trace: d.traceID(scRF05, 1), id: idLateRoot, name: "rf05.parent", start: t0, dur: time.Second})
}

// invalidIDs has one valid span and three the receiver must reject.
func (d dataset) invalidIDs() *collectortracepb.ExportTraceServiceRequest {
	t0 := d.at(-10 * time.Minute)
	req := request(d.service("invalid-ids"), nil,
		spanSpec{trace: d.traceID(scBad, 1), id: idRoot, name: "valid", start: t0, dur: time.Millisecond})
	spans := &req.ResourceSpans[0].ScopeSpans[0].Spans
	zeroTrace := spanSpec{trace: "00000000000000000000000000000000", id: idRoot, name: "zero trace id", start: t0, dur: time.Millisecond}.proto()
	zeroSpan := spanSpec{trace: d.traceID(scBad, 2), id: "0000000000000000", name: "zero span id", start: t0, dur: time.Millisecond}.proto()
	badParent := spanSpec{trace: d.traceID(scBad, 3), id: idRoot, name: "short parent", start: t0, dur: time.Millisecond}.proto()
	badParent.ParentSpanId = []byte{1, 2, 3}
	*spans = append(*spans, zeroTrace, zeroSpan, badParent)
	return req
}
