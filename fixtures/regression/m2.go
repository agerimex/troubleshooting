package main

import (
	"context"
	"fmt"
	"time"

	sender "github.com/agerimex/troubleshooting/log-sender"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// Milestone 2 (features/milestone_2): statistics, error groups and release
// comparison. The reference regression of REFERENCE_FIXTURE.md has three
// releases of recording-worker / upload_recording; small isolated cases cover
// the remaining scenarios, each under its own service so they never mix.
//
// Time: T is a whole UTC hour eight hours before the run's base time (so its
// records stay out of the windows of the RF and M1 checks). The
// reference releases use [T, T+1h), [T+2h, T+3h) and [T+4h, T+5h); the small
// cases mostly use [T+5h, T+6h). Everything is received now, well within the
// 24 h allowed lateness, so every bucket is provisional until the fixture
// moves the test clock forward.

const (
	scM2Ref      = 0x0501 // + release index (0x0501..0x0503)
	scM2Kinds    = 0x0510 // CAL-001
	scM2Env      = 0x0511 // CAL-002
	scM2Window   = 0x0512 // CAL-003
	scM2Collide  = 0x0513 // CAL-004
	scM2Conflict = 0x0514 // CAL-007
	scM2Dist     = 0x0515 // LAT-002
	scM2Invalid  = 0x0516 // LAT-005
	scM2Exc      = 0x0517 // ERR-001
	scM2OkErr    = 0x0519 // ERR-003, INV-002
	scM2Unlinked = 0x051a // ERR-004
	scM2Dups     = 0x051b // ERR-006
	scM2Slow     = 0x051d // INV-003
	scM2Rate     = 0x051e // CMP-004
	scM2Ops      = 0x051f // CMP-008
	scM2Buckets  = 0x0520 // LAT-003
	scM2Prov     = 0x0521 // RET-005
	scM2Fail     = 0x0522 // RET-008
	scM2Mix      = 0x0523 // RET-007
	scM2Series   = 0x0524 // CAL-008
	scM2Sampled  = 0x0525 // CAL-006
	scM2Rep      = 0x0526 // REP-002
	scM2Late     = 0x0527 // RET-006
)

// Reference resource attributes (REFERENCE_FIXTURE.md).
const (
	m2Env          = "fixture"
	m2Installation = "reference-local"
	m2Segment      = "upload-small"
	m2APIVersion   = "3.2.0"
)

type release struct {
	version  string
	offset   time.Duration // window start after T
	duration time.Duration // of every upload
	failures int           // of 1000 uploads
}

var releases = []release{
	{"1.0.0", 0, 100 * time.Millisecond, 10},
	{"1.1.0", 2 * time.Hour, 160 * time.Millisecond, 50},
	{"1.1.1", 4 * time.Hour, 105 * time.Millisecond, 10},
}

// m2T is the reference base time T.
func (d dataset) m2T() time.Time { return d.base.Truncate(time.Hour).Add(-8 * time.Hour) }

// m2Hour is T+n hours.
func (d dataset) m2Hour(n int) time.Time { return d.m2T().Add(time.Duration(n) * time.Hour) }

func workerResource(version string) map[string]string {
	r := map[string]string{
		"deployment.environment.name":      m2Env,
		"troubleshooting.installation":     m2Installation,
		"troubleshooting.workload_segment": m2Segment,
		"troubleshooting.sampling_policy":  "unsampled",
	}
	if version != "" {
		r["service.version"] = version
	}
	return r
}

// uploadStart is when upload k of a release starts: 5 s into the window, then
// every 3 s.
func (d dataset) uploadStart(rel release, k int) time.Time {
	return d.m2T().Add(rel.offset + 5*time.Second + time.Duration(k)*3*time.Second)
}

// failed says whether upload k fails: f per 1000 means k%100 < f/10.
func (rel release) failed(k int) bool { return k%100 < rel.failures/10 }

// refStack is the StorageError stack trace. Line numbers differ between
// failures; errors-v1 must ignore them.
func refStack(k int) string {
	return fmt.Sprintf("storage.Upload(0xc0000b4000, 0x%x)\n\t/app/storage/upload.go:%d +0x1d4\nworker.Run()\n\t/app/worker/run.go:31 +0x55", k, 42+k%3)
}

// referenceSpans are the requests of one release: per trace the API request,
// the selected upload and its storage dependency.
func (d dataset) referenceSpans(ri int) []*collectortracepb.ExportTraceServiceRequest {
	rel := releases[ri]
	var reqs []*collectortracepb.ExportTraceServiceRequest
	for batch := 0; batch < 10; batch++ {
		var api, worker []spanSpec
		for k := batch * 100; k < (batch+1)*100; k++ {
			trace := d.traceID(scM2Ref+ri, k+1)
			s := d.uploadStart(rel, k)
			api = append(api, spanSpec{trace: trace, id: idRoot, name: "start_recording", kind: tracepb.Span_SPAN_KIND_SERVER,
				start: s, dur: 30 * time.Millisecond, status: tracepb.Status_STATUS_CODE_OK})
			up := spanSpec{trace: trace, id: idChild1, parent: idRoot, name: "upload_recording", kind: tracepb.Span_SPAN_KIND_CONSUMER,
				start: s.Add(40 * time.Millisecond), dur: rel.duration, status: tracepb.Status_STATUS_CODE_OK}
			if rel.failed(k) {
				up.status, up.message = tracepb.Status_STATUS_CODE_ERROR, "Upload failed"
				up.events = []*tracepb.Span_Event{{
					TimeUnixNano: uint64(up.start.Add(rel.duration - 5*time.Millisecond).UnixNano()), Name: "exception",
					Attributes: kvs(map[string]string{"exception.type": "StorageError", "exception.message": "Upload failed", "exception.stacktrace": refStack(k)}),
				}}
			}
			worker = append(worker, up, spanSpec{trace: trace, id: idChild2, parent: idChild1, name: "storage.PutObject", kind: tracepb.Span_SPAN_KIND_CLIENT,
				start: s.Add(45 * time.Millisecond), dur: rel.duration - 10*time.Millisecond, status: tracepb.Status_STATUS_CODE_OK})
		}
		reqs = append(reqs, merge(
			request(d.service("recording-api"), map[string]string{"service.version": m2APIVersion, "deployment.environment.name": m2Env,
				"troubleshooting.installation": m2Installation, "troubleshooting.sampling_policy": "unsampled"}, api...),
			request(d.service("recording-worker"), workerResource(rel.version), worker...)))
	}
	return reqs
}

func m2LogAttrs(operation, version string) map[string]string {
	a := map[string]string{"operation.name": operation, "deployment.environment.name": m2Env,
		"troubleshooting.installation": m2Installation, "troubleshooting.workload_segment": m2Segment}
	if version != "" {
		a["service.version"] = version
	}
	return a
}

func (d dataset) m2Log(alias string, at time.Time, severity, service, message, trace, span string, attrs map[string]string) sender.LogRecord {
	return sender.LogRecord{RecordID: d.recordID(alias), Time: at, Severity: severity, Message: message,
		TraceID: trace, SpanID: span, ServiceName: d.service(service), Attributes: attrs}
}

// referenceLogs: every trace has request-accepted and upload-start; failed
// uploads add a retry WARN and the 403 ERROR, which is the only place the
// AccessDenied detail is recorded; successful ones an upload-completed INFO.
func (d dataset) referenceLogs(ri int) []sender.LogRecord {
	rel := releases[ri]
	var logs []sender.LogRecord
	for k := 0; k < 1000; k++ {
		trace := d.traceID(scM2Ref+ri, k+1)
		s := d.uploadStart(rel, k)
		alias := fmt.Sprintf("ref%d-%04d", ri, k)
		logs = append(logs,
			d.m2Log(alias+"-accepted", s.Add(5*time.Millisecond), "INFO", "recording-api", "Recording request accepted", trace, idRoot, m2LogAttrs("start_recording", m2APIVersion)),
			d.m2Log(alias+"-start", s.Add(41*time.Millisecond), "INFO", "recording-worker", "Upload started", trace, idChild1, m2LogAttrs("upload_recording", rel.version)))
		if rel.failed(k) {
			logs = append(logs,
				d.m2Log(alias+"-retry", s.Add(60*time.Millisecond), "WARN", "recording-worker", "Upload retry 1 of 3", trace, idChild1, m2LogAttrs("upload_recording", rel.version)),
				d.m2Log(alias+"-denied", s.Add(40*time.Millisecond+rel.duration-2*time.Millisecond), "ERROR", "recording-worker", "Storage returned 403 AccessDenied", trace, idChild1, m2LogAttrs("upload_recording", rel.version)))
		} else {
			logs = append(logs, d.m2Log(alias+"-done", s.Add(40*time.Millisecond+rel.duration), "INFO", "recording-worker", "Upload completed", trace, idChild1, m2LogAttrs("upload_recording", rel.version)))
		}
	}
	return logs
}

// call is one single-span trace of a small case.
type call struct {
	at      time.Time
	dur     time.Duration
	status  tracepb.Status_StatusCode
	message string
	noEnd   bool
	events  []*tracepb.Span_Event
}

// calls builds a request of single-span traces of one service: scenario sc,
// traces first..first+len-1, span id idRoot.
func (d dataset) calls(sc, first int, service, name string, kind tracepb.Span_SpanKind, resource map[string]string, cs []call) *collectortracepb.ExportTraceServiceRequest {
	specs := make([]spanSpec, len(cs))
	for i, c := range cs {
		specs[i] = spanSpec{trace: d.traceID(sc, first+i), id: idRoot, name: name, kind: kind, start: c.at, dur: c.dur,
			status: c.status, message: c.message, noEnd: c.noEnd, events: c.events}
	}
	return request(d.service(service), resource, specs...)
}

// series makes n calls starting at from, step apart, with duration dur(i).
func series(from time.Time, n int, step time.Duration, dur func(i int) time.Duration) []call {
	out := make([]call, n)
	for i := range out {
		out[i] = call{at: from.Add(time.Duration(i) * step), dur: dur(i), status: tracepb.Status_STATUS_CODE_OK}
	}
	return out
}

func constant(d time.Duration) func(int) time.Duration { return func(int) time.Duration { return d } }

func versioned(version string) map[string]string {
	r := map[string]string{"deployment.environment.name": m2Env, "troubleshooting.sampling_policy": "unsampled"}
	if version != "" {
		r["service.version"] = version
	}
	return r
}

const server = tracepb.Span_SPAN_KIND_SERVER

// m2SmallSpans are the isolated cases (spans).
func (d dataset) m2SmallSpans() []*collectortracepb.ExportTraceServiceRequest {
	ms := time.Millisecond
	h5 := d.m2Hour(5)
	var reqs []*collectortracepb.ExportTraceServiceRequest

	// CAL-001: 5 SERVER calls (2 ERROR, 2 OK, 1 UNSET; the last has a recorded
	// parent) and 12 CLIENT dependency spans in their traces.
	statuses := []tracepb.Status_StatusCode{tracepb.Status_STATUS_CODE_ERROR, tracepb.Status_STATUS_CODE_ERROR,
		tracepb.Status_STATUS_CODE_OK, tracepb.Status_STATUS_CODE_OK, tracepb.Status_STATUS_CODE_UNSET}
	var kinds []spanSpec
	for i, st := range statuses {
		trace := d.traceID(scM2Kinds, i+1)
		at := h5.Add(time.Minute + time.Duration(i)*time.Second)
		s := spanSpec{trace: trace, id: idRoot, name: "start_recording", kind: server, start: at, dur: 50 * ms, status: st}
		if i == 4 {
			s.parent = "00000000000000f1"
		}
		kinds = append(kinds, s)
		for j := 0; j < 3 && i < 4; j++ {
			kinds = append(kinds, spanSpec{trace: trace, id: fmt.Sprintf("00000000000000c%d", j), parent: idRoot, name: "db.query",
				kind: tracepb.Span_SPAN_KIND_CLIENT, start: at.Add(5 * ms), dur: 10 * ms, status: tracepb.Status_STATUS_CODE_OK})
		}
	}
	reqs = append(reqs, request(d.service("m2-kinds"), versioned("1.0.0"), kinds...))

	// CAL-002: environments and versions, one call without a version whose
	// trace has a versioned span of another service.
	envCall := func(n int, env, version string) *collectortracepb.ExportTraceServiceRequest {
		r := map[string]string{"deployment.environment.name": env, "troubleshooting.sampling_policy": "unsampled"}
		if version != "" {
			r["service.version"] = version
		}
		return d.calls(scM2Env, n, "m2-env", "handle", server, r, []call{{at: h5.Add(3*time.Minute + time.Duration(n)*time.Second), dur: 20 * ms, status: tracepb.Status_STATUS_CODE_OK}})
	}
	n := 1
	for _, g := range []struct {
		env, version string
		count        int
	}{{"production", "1.0.0", 2}, {"production", "1.1.0", 3}, {"production", "", 1}, {"test", "1.0.0", 4}} {
		for i := 0; i < g.count; i++ {
			reqs = append(reqs, envCall(n, g.env, g.version))
			if g.version == "" {
				reqs = append(reqs, request(d.service("m2-env-api"), map[string]string{"service.version": "9.9.9", "deployment.environment.name": "production"},
					spanSpec{trace: d.traceID(scM2Env, n), id: idChild1, name: "proxy", kind: server, start: h5.Add(3*time.Minute + time.Duration(n)*time.Second), dur: 30 * ms}))
			}
			n++
		}
	}

	// CAL-003: calls at S, S+60s and S+120s.
	s := d.m2WindowStart()
	reqs = append(reqs, d.calls(scM2Window, 1, "m2-window", "tick", server, versioned("1.0.0"), []call{
		{at: s, dur: 5 * ms}, {at: s.Add(60 * time.Second), dur: 5 * ms}, {at: s.Add(120 * time.Second), dur: 5 * ms}}))

	// CAL-004: two traces, one span id.
	reqs = append(reqs, request(d.service("m2-collide"), versioned("1.0.0"),
		spanSpec{trace: d.traceID(scM2Collide, 1), id: idShared, name: "collide", kind: server, start: h5.Add(11 * time.Minute), dur: 10 * ms, status: tracepb.Status_STATUS_CODE_OK},
		spanSpec{trace: d.traceID(scM2Collide, 2), id: idShared, name: "collide", kind: server, start: h5.Add(11*time.Minute + time.Second), dur: 10 * ms, status: tracepb.Status_STATUS_CODE_OK}))

	// CAL-007: one identity, two payloads that disagree on duration and status.
	reqs = append(reqs, d.calls(scM2Conflict, 1, "m2-conflict", "conflict", server, versioned("1.0.0"), []call{{at: h5.Add(12 * time.Minute), dur: 100 * ms, status: tracepb.Status_STATUS_CODE_OK}}))
	reqs = append(reqs, d.calls(scM2Conflict, 1, "m2-conflict", "conflict", server, versioned("1.0.0"), []call{{at: h5.Add(12 * time.Minute), dur: 200 * ms, status: tracepb.Status_STATUS_CODE_ERROR, message: "conflicting copy"}}))

	// LAT-002: 900 x 100 ms, 80 x 300 ms, 20 x 1000 ms.
	reqs = append(reqs, d.calls(scM2Dist, 1, "m2-dist", "render", server, versioned("1.0.0"), series(h5, 1000, 2*time.Second, func(i int) time.Duration {
		switch {
		case i < 900:
			return 100 * ms
		case i < 980:
			return 300 * ms
		}
		return 1000 * ms
	})))

	// LAT-005: 1000 x 100 ms, one zero duration, one ending before it starts, one without end.
	inv := series(h5.Add(time.Second), 1000, 2*time.Second, constant(100*ms))
	inv = append(inv, call{at: h5.Add(40 * time.Minute), dur: 0, status: tracepb.Status_STATUS_CODE_OK},
		call{at: h5.Add(40*time.Minute + time.Second), dur: -5 * ms, status: tracepb.Status_STATUS_CODE_OK},
		call{at: h5.Add(40*time.Minute + 2*time.Second), noEnd: true, status: tracepb.Status_STATUS_CODE_OK})
	reqs = append(reqs, d.calls(scM2Invalid, 1, "m2-invalid", "render", server, versioned("1.0.0"), inv))

	// ERR-001: two failures in storage.Upload (different request ids and
	// lines), one in auth.Authorize.
	exc := func(at time.Time, request, stack string) call {
		return call{at: at, dur: 20 * ms, status: tracepb.Status_STATUS_CODE_ERROR, message: "Upload failed", events: []*tracepb.Span_Event{{
			TimeUnixNano: uint64(at.Add(10 * ms).UnixNano()), Name: "exception",
			Attributes: kvs(map[string]string{"exception.type": "StorageError", "exception.message": "Upload failed for " + request, "exception.stacktrace": stack}),
		}}}
	}
	reqs = append(reqs, d.calls(scM2Exc, 1, "m2-exc", "upload", server, versioned("1.0.0"), []call{
		exc(h5.Add(13*time.Minute), "request-A", "storage.Upload(0x1)\n\t/app/storage/upload.go:42 +0x1d"),
		exc(h5.Add(13*time.Minute+time.Second), "request-B", "storage.Upload(0x2)\n\t/app/storage/upload.go:57 +0x2e"),
		exc(h5.Add(13*time.Minute+2*time.Second), "request-C", "auth.Authorize(0x3)\n\t/app/auth/authorize.go:12 +0x10"),
	}))

	// ERR-003: one OK call (its three ERROR logs are in m2SmallLogs).
	reqs = append(reqs, d.calls(scM2OkErr, 1, "m2-okerr", "charge", server, versioned("1.0.0"), []call{{at: h5.Add(14 * time.Minute), dur: 300 * ms, status: tracepb.Status_STATUS_CODE_OK}}))

	// ERR-004: version 1.1.0 calls around an unlinked error log.
	reqs = append(reqs, d.calls(scM2Unlinked, 1, "m2-unlinked", "handle", server, versioned("1.1.0"), series(h5.Add(20*time.Minute), 3, time.Second, constant(10*ms))))

	// ERR-006: one ERROR span with one exception event.
	reqs = append(reqs, d.calls(scM2Dups, 1, "m2-dups", "handle", server, versioned("1.0.0"), []call{
		exc(h5.Add(21*time.Minute), "request-D", "dups.Handle(0x1)\n\t/app/dups/handle.go:9 +0x1")}))

	// INV-003: 960 x 100 ms and 40 x 900 ms.
	reqs = append(reqs, d.calls(scM2Slow, 1, "m2-slow", "render", server, versioned("1.0.0"), series(h5.Add(500*ms), 1000, 2*time.Second, func(i int) time.Duration {
		if i%25 == 0 {
			return 900 * ms
		}
		return 100 * ms
	})))

	// CMP-004: 2000 calls in [T+1h, T+2h), 4000 in [T+3h, T+5h).
	reqs = append(reqs, d.calls(scM2Rate, 1, "m2-rate", "poll", server, versioned("1.1.0"), series(d.m2Hour(1), 2000, 1800*ms, constant(50*ms))))
	reqs = append(reqs, d.calls(scM2Rate, 2001, "m2-rate", "poll", server, versioned("1.1.0"), series(d.m2Hour(3), 4000, 1800*ms, constant(50*ms))))

	// CMP-008: version b has publish_recording and upload, version c only upload.
	reqs = append(reqs, d.calls(scM2Ops, 1, "m2-ops", "publish_recording", server, versioned("b"), series(h5.Add(100*ms), 1000, 3*time.Second, constant(40*ms))))
	reqs = append(reqs, d.calls(scM2Ops, 1001, "m2-ops", "upload", server, versioned("b"), series(h5.Add(200*ms), 1000, 3*time.Second, constant(40*ms))))
	reqs = append(reqs, d.calls(scM2Ops, 2001, "m2-ops", "upload", server, versioned("c"), series(h5.Add(300*ms), 1000, 3*time.Second, constant(40*ms))))

	// LAT-003: 990 x 100 ms in T+4h, 10 x 1000 ms in T+5h.
	reqs = append(reqs, d.calls(scM2Buckets, 1, "m2-buckets", "render", server, versioned("1.0.0"), series(d.m2Hour(4), 990, 3*time.Second, constant(100*ms))))
	reqs = append(reqs, d.calls(scM2Buckets, 991, "m2-buckets", "render", server, versioned("1.0.0"), series(h5.Add(30*time.Minute), 10, time.Second, constant(1000*ms))))

	// RET-005, RET-008: 100 calls each in T+5h (one more each is sent later).
	reqs = append(reqs, d.calls(scM2Prov, 1, "m2-prov", "tick", server, versioned("1.0.0"), series(h5.Add(2*time.Second), 100, 10*time.Second, constant(10*ms))))
	reqs = append(reqs, d.calls(scM2Fail, 1, "m2-fail", "tick", server, versioned("1.0.0"), series(h5.Add(3*time.Second), 100, 10*time.Second, constant(10*ms))))

	// RET-007: 100 calls in T+4h, 50 in T+5h.
	reqs = append(reqs, d.calls(scM2Mix, 1, "m2-mix", "tick", server, versioned("1.0.0"), series(d.m2Hour(4).Add(time.Second), 100, 10*time.Second, constant(10*ms))))
	reqs = append(reqs, d.calls(scM2Mix, 101, "m2-mix", "tick", server, versioned("1.0.0"), series(h5.Add(4*time.Second), 50, 10*time.Second, constant(10*ms))))

	// CAL-008: 100 calls in T-3h, none in T-2h; T-1h is a declared gap.
	reqs = append(reqs, d.calls(scM2Series, 1, "m2-series", "tick", server, versioned("1.0.0"), series(d.m2Hour(-3).Add(time.Second), 100, 10*time.Second, constant(10*ms))))

	// CAL-006: 200 calls of a 20 % head-sampled source, in the gap hour.
	sampled := map[string]string{"deployment.environment.name": m2Env, "service.version": "1.0.0", "troubleshooting.sampling_policy": "head:0.2"}
	reqs = append(reqs, d.calls(scM2Sampled, 1, "m2-sampled", "tick", server, sampled, series(d.m2Hour(-1).Add(time.Second), 200, 10*time.Second, constant(10*ms))))

	// REP-002: 1000 calls of version b in T+1h and of version c in T+3h.
	reqs = append(reqs, d.calls(scM2Rep, 1, "m2-rep", "tick", server, versioned("b"), series(d.m2Hour(1).Add(time.Second), 1000, 3*time.Second, constant(20*ms))))
	reqs = append(reqs, d.calls(scM2Rep, 1001, "m2-rep", "tick", server, versioned("c"), series(d.m2Hour(3).Add(time.Second), 1000, 3*time.Second, constant(20*ms))))
	return reqs
}

// m2WindowStart is S of CAL-003.
func (d dataset) m2WindowStart() time.Time { return d.m2Hour(5).Add(10 * time.Minute) }

// m2SmallLogs are the isolated cases (logs).
func (d dataset) m2SmallLogs() []sender.LogRecord {
	h5 := d.m2Hour(5)
	v := func(version string) map[string]string {
		a := map[string]string{"deployment.environment.name": m2Env, "operation.name": "handle"}
		if version != "" {
			a["service.version"] = version
		}
		return a
	}
	var logs []sender.LogRecord
	// ERR-002: message groups; request ids vary, status codes must not merge.
	for i, m := range []string{"Request request-A failed with status 403", "Request request-B failed with status 403", "Request request-C failed with status 500"} {
		logs = append(logs, d.m2Log(fmt.Sprintf("msg-%c", 'A'+i), h5.Add(15*time.Minute+time.Duration(i)*time.Second), "ERROR", "m2-msg", m, "", "", v("1.0.0")))
	}
	// ERR-003: three ERROR logs of one group in the OK trace.
	okTrace := d.traceID(scM2OkErr, 1)
	for i := 0; i < 3; i++ {
		logs = append(logs, d.m2Log(fmt.Sprintf("okerr-%d", i), h5.Add(14*time.Minute+time.Duration(50+i*50)*time.Millisecond), "ERROR", "m2-okerr",
			"Payment provider rejected the card", okTrace, idRoot, map[string]string{"deployment.environment.name": m2Env, "service.version": "1.0.0", "operation.name": "charge"}))
	}
	logs = append(logs, d.m2Log("okerr-info", h5.Add(14*time.Minute+10*time.Millisecond), "INFO", "m2-okerr", "Charge requested", okTrace, idRoot,
		map[string]string{"deployment.environment.name": m2Env, "service.version": "1.0.0", "operation.name": "charge"}))
	// ERR-004: no trace, no span, no version.
	logs = append(logs, d.m2Log("unlinked", h5.Add(20*time.Minute+time.Second), "ERROR", "m2-unlinked", "Disk quota exceeded on /var/spool", "", "",
		map[string]string{"deployment.environment.name": m2Env}))
	// ERR-006: two distinct records with the same message and time.
	at := h5.Add(21 * time.Minute)
	logs = append(logs,
		d.m2Log("dup-1", at, "ERROR", "m2-dups", "Connection reset by peer", "", "", v("1.0.0")),
		d.m2Log("dup-2", at, "ERROR", "m2-dups", "Connection reset by peer", "", "", v("1.0.0")))
	// ERR-005: groups A (10 -> 3), B (0 -> 2), C (4 -> 0) between versions b and c.
	add := func(version, message string, count int, minute int) {
		for i := 0; i < count; i++ {
			logs = append(logs, d.m2Log(fmt.Sprintf("grp-%s-%d-%d", version, minute, i), h5.Add(time.Duration(minute)*time.Minute+time.Duration(i)*time.Second),
				"ERROR", "m2-groups", message, "", "", v(version)))
		}
	}
	add("b", "Cache lookup timed out", 10, 22)
	add("b", "Thumbnail generation failed", 4, 23)
	add("c", "Cache lookup timed out", 3, 24)
	add("c", "Webhook signature invalid", 2, 25)
	return logs
}

// m2Late is RET-006: a call of release 1.0.0 that was never sent before,
// delivered after its bucket was finalized.
func (d dataset) m2Late() *collectortracepb.ExportTraceServiceRequest {
	return request(d.service("recording-worker"), workerResource("1.0.0"), spanSpec{trace: d.traceID(scM2Late, 1), id: idChild1, name: "upload_recording",
		kind: tracepb.Span_SPAN_KIND_CONSUMER, start: d.m2T().Add(30*time.Minute + 500*time.Millisecond), dur: 100 * time.Millisecond, status: tracepb.Status_STATUS_CODE_OK})
}

// m2Extra are the later calls: RET-005 (prov), RET-008 (fail), REP-002 (10 x rep).
func (d dataset) m2ProvExtra() *collectortracepb.ExportTraceServiceRequest {
	return d.calls(scM2Prov, 101, "m2-prov", "tick", server, versioned("1.0.0"), []call{{at: d.m2Hour(5).Add(50 * time.Minute), dur: 10 * time.Millisecond, status: tracepb.Status_STATUS_CODE_OK}})
}

func (d dataset) m2FailExtra() *collectortracepb.ExportTraceServiceRequest {
	return d.calls(scM2Fail, 101, "m2-fail", "tick", server, versioned("1.0.0"), []call{{at: d.m2Hour(5).Add(51 * time.Minute), dur: 10 * time.Millisecond, status: tracepb.Status_STATUS_CODE_OK}})
}

func (d dataset) m2RepExtra() *collectortracepb.ExportTraceServiceRequest {
	return d.calls(scM2Rep, 2001, "m2-rep", "tick", server, versioned("c"), series(d.m2Hour(3).Add(55*time.Minute), 10, time.Second, constant(20*time.Millisecond)))
}

// m2LateLog is INV-004: a diagnostic log that arrives after a comparison was
// saved, for the first failed upload of 1.1.0.
func (d dataset) m2LateLog() sender.LogRecord {
	k := 0
	s := d.uploadStart(releases[1], k)
	return d.m2Log("inv4-late", s.Add(70*time.Millisecond), "INFO", "recording-worker", "Upload diagnostics: bucket policy denies PutObject",
		d.traceID(scM2Ref+1, k+1), idChild1, m2LogAttrs("upload_recording", "1.1.0"))
}

// m2Spans and m2Logs are everything sent at load (and again on replay).
func (d dataset) m2Spans() []*collectortracepb.ExportTraceServiceRequest {
	var reqs []*collectortracepb.ExportTraceServiceRequest
	for ri := range releases {
		reqs = append(reqs, d.referenceSpans(ri)...)
	}
	reqs = append(reqs, d.m2SmallSpans()...)
	reqs = append(reqs, d.pwSpans()...)
	return append(reqs, d.seqSpans()...)
}

func (d dataset) m2Logs() []sender.LogRecord {
	var logs []sender.LogRecord
	for ri := range releases {
		logs = append(logs, d.referenceLogs(ri)...)
	}
	logs = append(logs, d.m2SmallLogs()...)
	logs = append(logs, d.pwLogs()...)
	return append(logs, d.seqLogs()...)
}

// sendLogsChunked sends logs in batches the receiver confirms one by one.
func sendLogsChunked(ctx context.Context, d dataset, addr string, records []sender.LogRecord) error {
	for start := 0; start < len(records); start += 1000 {
		end := min(start+1000, len(records))
		if err := sendLogs(ctx, d, addr, records[start:end]); err != nil {
			return fmt.Errorf("logs %d-%d: %w", start, end, err)
		}
	}
	return nil
}

// m2Coverage are the coverage declarations of the fixture's hours: complete
// collection except the gap hour T-1h (CAL-006, CAL-008).
func (d dataset) m2Coverage() []coverageDeclaration {
	const why = "regression fixture: synthetic records generated and confirmed by the receiver"
	return []coverageDeclaration{
		{Source: "all", TimeFrom: ns(d.m2Hour(-3)), TimeTo: ns(d.m2Hour(-1)), Status: "complete", Reason: why},
		{Source: "all", TimeFrom: ns(d.m2Hour(-1)), TimeTo: ns(d.m2Hour(0)), Status: "gap", Reason: "regression fixture: simulated collection outage"},
		{Source: "all", TimeFrom: ns(d.m2Hour(0)), TimeTo: ns(d.m2Hour(6)), Status: "complete", Reason: why},
	}
}
