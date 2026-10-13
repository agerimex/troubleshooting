package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	sender "github.com/agerimex/troubleshooting/log-sender"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// Milestone 1 (features/milestone_1): one trace's spans and its correlated
// logs in one report. Spans go through OTLP, logs through log-sender's
// LogExporter (SendLogs), the supported log path. Log aliases are those of
// the feature files; record ids are "fx<run>-<alias>", so a replay repeats
// them and the report must still count each record once.

const (
	scM1Rec    = 0x0401 // recording-A: OK request, failed upload, the RPT-007 logs
	scM1Ok     = 0x0402 // recording-ok: every span OK, an ERROR log (RPT-003/005)
	scM1Shared = 0x0403 // COR-001: two traces with the same span id
	scM1Ctx    = 0x0404 // COR-003, REL-002
	scM1Lim    = 0x0405 // CMP-003: 1 = 3 spans/3 logs, 2 = 4 spans/4 logs
	scM1Async  = 0x0406 // CMP-004
	scM1Logs   = 0x0407 // COR-006: logs without spans
	scM1Late   = 0x0408 // REL-003: parent and referenced span arrive later
	scM1Bg     = 0x0409 // RPT-008: background-B, linked from recording-A
	scM1Out    = 0x040a // REF-003: a log sent during an outage
)

const idMissing = "00000000000000ee" // recorded by a log, never sent as a span

// m1Base is where the milestone's records sit: inside the default window.
func (d dataset) m1Base() time.Time { return d.at(-8 * time.Minute) }

func (d dataset) recordID(alias string) string { return "fx" + d.run + "-" + alias }

// alias strips the run prefix of a record id.
func (d dataset) alias(recordID string) string {
	return strings.TrimPrefix(recordID, "fx"+d.run+"-")
}

// m1Spans are the milestone's spans, part of deliveries (so replays resend them).
func (d dataset) m1Spans() []*collectortracepb.ExportTraceServiceRequest {
	t := d.m1Base()
	ok, failed := tracepb.Status_STATUS_CODE_OK, tracepb.Status_STATUS_CODE_ERROR
	rec, bg := d.traceID(scM1Rec, 1), d.traceID(scM1Bg, 1)
	okTrace := d.traceID(scM1Ok, 1)
	business := map[string]string{"business.operation": "recording-42"}
	var reqs []*collectortracepb.ExportTraceServiceRequest

	reqs = append(reqs,
		request(d.service("recording-api"), map[string]string{"service.version": "1.4.0"},
			spanSpec{trace: rec, id: idRoot, name: "start_recording", start: t, dur: 150 * time.Millisecond, status: ok, attrs: business},
			spanSpec{trace: okTrace, id: idRoot, name: "start_recording", start: t, dur: 150 * time.Millisecond, status: ok}),
		// No service.version: the report must show it as unavailable. The
		// diagnostic detail is only in the logs, never in span attributes or events.
		request(d.service("recording-worker"), nil,
			spanSpec{trace: rec, id: idChild1, parent: idRoot, name: "upload_recording", start: t.Add(50 * time.Millisecond), dur: 900 * time.Millisecond,
				status: failed, message: "upload failed", attrs: business,
				links: []*tracepb.Span_Link{{TraceId: mustHex(bg), SpanId: mustHex(idRoot)}}},
			spanSpec{trace: okTrace, id: idChild1, parent: idRoot, name: "publish_recording", start: t.Add(50 * time.Millisecond), dur: 600 * time.Millisecond, status: ok}),
		request(d.service("background"), nil,
			spanSpec{trace: bg, id: idRoot, name: "background_publish", start: t.Add(2 * time.Second), dur: 100 * time.Millisecond, status: ok, attrs: business}),
		request(d.service("m1-shared"), nil,
			spanSpec{trace: d.traceID(scM1Shared, 1), id: idShared, name: "shared_op", start: t, dur: 10 * time.Millisecond},
			spanSpec{trace: d.traceID(scM1Shared, 2), id: idShared, name: "shared_op", start: t, dur: 10 * time.Millisecond}),
		request(d.service("m1-context"), nil,
			spanSpec{trace: d.traceID(scM1Ctx, 1), id: idRoot, name: "context_op", start: t, dur: 10 * time.Millisecond}),
		request(d.service("m1-async"), nil,
			spanSpec{trace: d.traceID(scM1Async, 1), id: idRoot, name: "async_parent", start: t, dur: 100 * time.Millisecond},
			spanSpec{trace: d.traceID(scM1Async, 1), id: idChild1, parent: idRoot, name: "async_child", start: t.Add(500 * time.Millisecond), dur: 50 * time.Millisecond}),
		request(d.service("m1-late"), nil,
			spanSpec{trace: d.traceID(scM1Late, 1), id: idLateKid, parent: idLateRoot, name: "late_child", start: t.Add(10 * time.Millisecond), dur: 10 * time.Millisecond}),
	)
	for n, count := range map[int]int{1: 3, 2: 4} {
		trace := d.traceID(scM1Lim, n)
		specs := []spanSpec{{trace: trace, id: idRoot, name: "limit_root", start: t, dur: time.Second}}
		for i := 1; i < count; i++ {
			specs = append(specs, spanSpec{trace: trace, id: fmt.Sprintf("%016x", 0x2000+i), parent: idRoot, name: fmt.Sprintf("limit_child_%d", i),
				start: t.Add(time.Duration(i) * time.Millisecond), dur: time.Millisecond})
		}
		reqs = append(reqs, request(d.service("m1-limit"), nil, specs...))
	}
	return reqs
}

// m1Late is the REL-003 parent, which is also the span the late log references.
func (d dataset) m1Late() *collectortracepb.ExportTraceServiceRequest {
	return request(d.service("m1-late"), nil,
		spanSpec{trace: d.traceID(scM1Late, 1), id: idLateRoot, name: "late_parent", start: d.m1Base(), dur: 100 * time.Millisecond})
}

func (d dataset) logRecord(alias, trace, span, severity, service, message string, offset time.Duration) sender.LogRecord {
	return sender.LogRecord{
		RecordID: d.recordID(alias), Time: d.m1Base().Add(offset), Severity: severity, Message: message,
		TraceID: trace, SpanID: span, ServiceName: d.service(service),
	}
}

// m1Logs are sent in this (not chronological) order.
func (d dataset) m1Logs() []sender.LogRecord {
	ms := time.Millisecond
	rec, okTrace := d.traceID(scM1Rec, 1), d.traceID(scM1Ok, 1)
	lim3, lim4 := d.traceID(scM1Lim, 1), d.traceID(scM1Lim, 2)
	logs := []sender.LogRecord{
		// RPT-001, RPT-007, COR-002, REF-001
		d.logRecord("log-accepted", rec, idRoot, "INFO", "recording-api", "Recording request accepted", 10*ms),
		d.logRecord("log-queued", rec, "", "INFO", "recording-api", "Recording queued for processing", 20*ms),
		d.logRecord("log-denied", rec, idChild1, "ERROR", "recording-worker", "Storage returned 403 AccessDenied", 1000*ms),
		d.logRecord("log-upload-start", rec, idChild1, "INFO", "recording-worker", "Upload started", 200*ms),
		d.logRecord("log-retry", rec, idChild1, "WARN", "recording-worker", "Upload retry 1 of 3", 400*ms),
		// RPT-003, RPT-005, RPT-006, COR-004
		d.logRecord("ok-accepted", okTrace, idRoot, "INFO", "recording-api", "Recording request accepted", 10*ms),
		d.logRecord("ok-retry", okTrace, idChild1, "WARN", "recording-worker", "Publication retry 1 of 3", 300*ms),
		d.logRecord("ok-unpublished", okTrace, idChild1, "ERROR", "recording-worker", "Recording could not be published", 600*ms),
		d.logRecord("ok-unlinked", "", idChild1, "ERROR", "recording-worker", "Recording could not be published", 600*ms),
		// RPT-008: the other trace of business operation recording-42
		d.logRecord("bg-started", d.traceID(scM1Bg, 1), idRoot, "INFO", "background", "Background publication started", 2*time.Second+10*ms),
		// COR-001
		d.logRecord("shared-a", d.traceID(scM1Shared, 1), idShared, "INFO", "m1-shared", "from trace A", 5*ms),
		d.logRecord("shared-b", d.traceID(scM1Shared, 2), idShared, "INFO", "m1-shared", "from trace B", 5*ms),
		// COR-003, REL-002
		d.logRecord("ctx-no-span", d.traceID(scM1Ctx, 1), "", "INFO", "m1-context", "no span recorded", 1*ms),
		d.logRecord("ctx-missing-span", d.traceID(scM1Ctx, 1), idMissing, "WARN", "m1-context", "span not sent", 2*ms),
		d.logRecord("ctx-same-1", d.traceID(scM1Ctx, 1), idRoot, "WARN", "m1-context", "identical event", 3*ms),
		d.logRecord("ctx-same-2", d.traceID(scM1Ctx, 1), idRoot, "WARN", "m1-context", "identical event", 3*ms),
		// CMP-004: after the parent ended
		d.logRecord("async-log", d.traceID(scM1Async, 1), idRoot, "INFO", "m1-async", "logged after the parent ended", 300*ms),
		// COR-006
		d.logRecord("nospans-error", d.traceID(scM1Logs, 1), idRoot, "ERROR", "m1-logs-only", "failed without spans", 1*ms),
		d.logRecord("nospans-info", d.traceID(scM1Logs, 1), "", "INFO", "m1-logs-only", "context without spans", 2*ms),
		// REL-003: references the late parent
		d.logRecord("late-ref", d.traceID(scM1Late, 1), idLateRoot, "INFO", "m1-late", "references the late parent", 50*ms),
	}
	for i := 1; i <= 3; i++ {
		logs = append(logs, d.logRecord(fmt.Sprintf("lim3-%d", i), lim3, idRoot, "INFO", "m1-limit", "limit", time.Duration(i)*ms))
	}
	for i := 1; i <= 4; i++ {
		logs = append(logs, d.logRecord(fmt.Sprintf("lim4-%d", i), lim4, idRoot, "INFO", "m1-limit", "limit", time.Duration(i)*ms))
	}
	return logs
}

// outageLog is sent while the test ClickHouse is stopped (REF-003).
func (d dataset) outageLog() sender.LogRecord {
	return d.logRecord("outage-log", d.traceID(scM1Out, 1), "", "ERROR", "m1-outage", "sent during an outage", time.Millisecond)
}

func sendLogs(ctx context.Context, d dataset, addr string, records []sender.LogRecord) error {
	exporter, err := sender.NewLogExporter(d.service("logs"), sender.WithAddress(addr))
	if err != nil {
		return err
	}
	defer exporter.Close()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return exporter.Send(ctx, records)
}

// expectedLogAliases is every logical log record of the run that the receiver
// confirmed.
func (c *checker) expectedLogAliases() []string {
	var out []string
	for _, r := range c.d.m1Logs() {
		out = append(out, c.d.alias(r.RecordID))
	}
	if c.state.OutageLogSent {
		out = append(out, "outage-log")
	}
	sort.Strings(out)
	return out
}

// --- checks

func (c *checker) report(ctx context.Context, scenario, n int) (apiReport, error) {
	from, to := c.d.window()
	r, err := c.api.report(ctx, reportRequest{TraceID: c.d.traceID(scenario, n), TimeFrom: ns(from), TimeTo: ns(to)})
	if err != nil {
		return r, err
	}
	return r, reportOK(r)
}

// reportOK checks the parts every successful report has.
func reportOK(r apiReport) error {
	if r.Spans == nil || r.Logs == nil {
		return fmt.Errorf("report without a spans or logs section")
	}
	if r.Spans.Status != "ok" || r.Logs.Status != "ok" {
		return fmt.Errorf("report sections %s/%s: %s %s", r.Spans.Status, r.Logs.Status, r.Spans.Error, r.Logs.Error)
	}
	for _, s := range r.Spans.Records {
		if s.TraceID != r.TraceID {
			return fmt.Errorf("span %s of trace %s in the report of %s", s.SpanID, s.TraceID, r.TraceID)
		}
	}
	for _, l := range r.Logs.Records {
		if l.TraceID != r.TraceID {
			return fmt.Errorf("log %s of trace %q in the report of %s", l.RecordID, l.TraceID, r.TraceID)
		}
	}
	return nil
}

// logAliases lists the aliases in the API's order (recorded time, record id).
func (c *checker) logAliases(logs []apiLog) []string {
	out := make([]string, len(logs))
	for i, l := range logs {
		out[i] = c.d.alias(l.RecordID)
	}
	return out
}

func expectList(what string, got, want []string) error {
	if len(got) == 0 && len(want) == 0 {
		return nil
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("%s: got %v, want %v", what, got, want)
	}
	return nil
}

func (c *checker) logByAlias(logs []apiLog, alias string) apiLog {
	for _, l := range logs {
		if c.d.alias(l.RecordID) == alias {
			return l
		}
	}
	return apiLog{}
}

// M1-RPT-001, -004, -007, COR-002, REL-001: one report has both spans and all
// five logs, once each, in recorded time order, with their recorded span ids.
func (c *checker) m1Recording(ctx context.Context) error {
	r, err := c.report(ctx, scM1Rec, 1)
	if err != nil {
		return err
	}
	if err := expectNames("recording-A spans", r.Spans.Records, "start_recording", "upload_recording"); err != nil {
		return err
	}
	req, _ := byName(r.Spans.Records, "start_recording")
	up, _ := byName(r.Spans.Records, "upload_recording")
	if req.Status != "ok" || up.Status != "error" || req.Duration != 150000 || up.Duration != 900000 || up.ParentSpanID != idRoot {
		return fmt.Errorf("spans: request %s %dµs, upload %s %dµs parent %s", req.Status, req.Duration, up.Status, up.Duration, up.ParentSpanID)
	}
	if v := tagValue(req.ServiceTags, "service.version"); v != "1.4.0" {
		return fmt.Errorf("request version %q, want 1.4.0", v)
	}
	if v := tagValue(up.ServiceTags, "service.version"); v != "" {
		return fmt.Errorf("upload version %q, want none recorded", v)
	}
	if err := expectList("recording-A logs", c.logAliases(r.Logs.Records),
		[]string{"log-accepted", "log-queued", "log-upload-start", "log-retry", "log-denied"}); err != nil {
		return err
	}
	wantSpan := map[string]string{"log-accepted": idRoot, "log-queued": "", "log-upload-start": idChild1, "log-retry": idChild1, "log-denied": idChild1}
	wantSeverity := map[string]string{"log-accepted": "INFO", "log-queued": "INFO", "log-upload-start": "INFO", "log-retry": "WARN", "log-denied": "ERROR"}
	services := map[string]bool{}
	for _, l := range r.Logs.Records {
		a := c.d.alias(l.RecordID)
		if l.SpanID != wantSpan[a] || l.Severity != wantSeverity[a] {
			return fmt.Errorf("%s: span %q severity %s, want span %q severity %s", a, l.SpanID, l.Severity, wantSpan[a], wantSeverity[a])
		}
		services[l.Service] = true
	}
	if !services[c.d.service("recording-api")] || !services[c.d.service("recording-worker")] || len(services) != 2 {
		return fmt.Errorf("log services %v, want recording-api and recording-worker", services)
	}
	if !r.Complete || r.Spans.Shown != 2 || r.Logs.Shown != 5 || r.Spans.Truncated || r.Logs.Truncated ||
		r.Spans.Total == nil || *r.Spans.Total != 2 || r.Logs.Total == nil || *r.Logs.Total != 5 {
		return fmt.Errorf("completeness: complete %v, spans %d/%v, logs %d/%v", r.Complete, r.Spans.Shown, r.Spans.Total, r.Logs.Shown, r.Logs.Total)
	}
	return nil
}

// M1-RPT-002: an error span search at any depth, then the report of its trace
// keeps the OK root and logs of every severity.
func (c *checker) m1ErrorSpanEntry(ctx context.Context) error {
	f := c.service(c.window(), "recording-worker", "all")
	f.Status = "error"
	found, err := c.api.all(ctx, f)
	if err != nil {
		return err
	}
	if err := expectNames("error spans of recording-worker", found, "upload_recording"); err != nil {
		return err
	}
	r, err := c.api.report(ctx, reportRequest{TraceID: found[0].TraceID, TimeFrom: f.TimeFrom, TimeTo: f.TimeTo})
	if err != nil {
		return err
	}
	if err := reportOK(r); err != nil {
		return err
	}
	if root, ok := byName(r.Spans.Records, "start_recording"); !ok || root.Status != "ok" {
		return fmt.Errorf("the report lacks the OK root: %v", names(r.Spans.Records))
	}
	severities := map[string]bool{}
	for _, l := range r.Logs.Records {
		severities[l.Severity] = true
	}
	if !severities["INFO"] || !severities["WARN"] || !severities["ERROR"] {
		return fmt.Errorf("severities %v, want INFO, WARN and ERROR", severities)
	}
	return nil
}

// M1-RPT-003, -005, -006, COR-004: an ERROR log search finds the error log of
// an all-OK trace and the log without a TraceId; the report follows only the
// recorded TraceId and gets no log filter.
func (c *checker) m1ErrorLogEntry(ctx context.Context) error {
	from, to := c.d.window()
	found, err := c.api.allLogs(ctx, logFilter{TimeFrom: ns(from), TimeTo: ns(to), ServiceName: c.d.service("recording-worker"),
		Severity: "ERROR", Message: "could not be published"})
	if err != nil {
		return err
	}
	got := c.logAliases(found)
	sort.Strings(got)
	if err := expectList("error log search", got, []string{"ok-unlinked", "ok-unpublished"}); err != nil {
		return err
	}
	unlinked, unpublished := c.logByAlias(found, "ok-unlinked"), c.logByAlias(found, "ok-unpublished")
	if unlinked.TraceID != "" || unlinked.SpanID != idChild1 {
		return fmt.Errorf("ok-unlinked: trace %q span %q, want no trace and span %s", unlinked.TraceID, unlinked.SpanID, idChild1)
	}
	if unpublished.TraceID != c.d.traceID(scM1Ok, 1) || unpublished.SpanID != idChild1 {
		return fmt.Errorf("ok-unpublished: trace %s span %s", unpublished.TraceID, unpublished.SpanID)
	}

	r, err := c.api.report(ctx, reportRequest{TraceID: unpublished.TraceID, TimeFrom: ns(from), TimeTo: ns(to)})
	if err != nil {
		return err
	}
	if err := reportOK(r); err != nil {
		return err
	}
	for _, s := range r.Spans.Records {
		if s.Status != "ok" {
			return fmt.Errorf("span %s has status %s, want every span OK", s.Name, s.Status)
		}
	}
	return expectList("report of the error log's trace", c.logAliases(r.Logs.Records), []string{"ok-accepted", "ok-retry", "ok-unpublished"})
}

// M1-RPT-008: the report covers recording-A only; the link to background-B
// is a reference and the shared business attribute merges nothing.
func (c *checker) m1Boundary(ctx context.Context) error {
	r, err := c.report(ctx, scM1Rec, 1)
	if err != nil {
		return err
	}
	bg := c.d.traceID(scM1Bg, 1)
	up, _ := byName(r.Spans.Records, "upload_recording")
	if len(up.Links) != 1 || up.Links[0].TraceID != bg || up.Links[0].SpanID != idRoot {
		return fmt.Errorf("upload links %v, want one to %s/%s", up.Links, bg, idRoot)
	}
	other, err := c.report(ctx, scM1Bg, 1)
	if err != nil {
		return err
	}
	if err := expectNames("background-B spans", other.Spans.Records, "background_publish"); err != nil {
		return err
	}
	return expectList("background-B logs", c.logAliases(other.Logs.Records), []string{"bg-started"})
}

// M1-COR-001: a log follows (TraceId, SpanId), not the SpanId alone.
func (c *checker) m1SharedSpanID(ctx context.Context) error {
	for n, want := range map[int]string{1: "shared-a", 2: "shared-b"} {
		r, err := c.report(ctx, scM1Shared, n)
		if err != nil {
			return err
		}
		if len(r.Spans.Records) != 1 || r.Spans.Records[0].SpanID != idShared {
			return fmt.Errorf("trace %d: spans %v", n, names(r.Spans.Records))
		}
		if err := expectList(fmt.Sprintf("logs of trace %d", n), c.logAliases(r.Logs.Records), []string{want}); err != nil {
			return err
		}
	}
	return nil
}

// M1-COR-003, REL-002: trace-level logs keep their recorded span id (none, or
// one not in the report); identical events with separate ids both appear.
func (c *checker) m1Context(ctx context.Context) error {
	r, err := c.report(ctx, scM1Ctx, 1)
	if err != nil {
		return err
	}
	if err := expectList("context logs", c.logAliases(r.Logs.Records), []string{"ctx-no-span", "ctx-missing-span", "ctx-same-1", "ctx-same-2"}); err != nil {
		return err
	}
	if l := c.logByAlias(r.Logs.Records, "ctx-no-span"); l.SpanID != "" {
		return fmt.Errorf("ctx-no-span got span %q", l.SpanID)
	}
	if l := c.logByAlias(r.Logs.Records, "ctx-missing-span"); l.SpanID != idMissing {
		return fmt.Errorf("ctx-missing-span: span %q, want the recorded %s", l.SpanID, idMissing)
	}
	for _, s := range r.Spans.Records {
		if s.SpanID == idMissing {
			return fmt.Errorf("a span %s exists; the scenario needs it absent", idMissing)
		}
	}
	a, b := c.logByAlias(r.Logs.Records, "ctx-same-1"), c.logByAlias(r.Logs.Records, "ctx-same-2")
	if a.TimeUnixNano != b.TimeUnixNano || a.Message != b.Message || a.Severity != b.Severity {
		return fmt.Errorf("the identical events differ: %+v %+v", a, b)
	}
	return nil
}

// M1-CMP-003: with a limit of 3, exactly 3 records are complete, 4 truncated.
func (c *checker) m1Limit(ctx context.Context) error {
	from, to := c.d.window()
	for n, available := range map[int]int{1: 3, 2: 4} {
		r, err := c.api.report(ctx, reportRequest{TraceID: c.d.traceID(scM1Lim, n), TimeFrom: ns(from), TimeTo: ns(to), SpanLimit: 3, LogLimit: 3})
		if err != nil {
			return err
		}
		if err := reportOK(r); err != nil {
			return err
		}
		truncated := available > 3
		for name, s := range map[string]struct {
			shown, limit int
			truncated    bool
			total        *int
		}{
			"spans": {r.Spans.Shown, r.Spans.Limit, r.Spans.Truncated, r.Spans.Total},
			"logs":  {r.Logs.Shown, r.Logs.Limit, r.Logs.Truncated, r.Logs.Total},
		} {
			if s.shown != 3 || s.limit != 3 || s.truncated != truncated || s.total == nil || *s.total != available {
				return fmt.Errorf("%d available %s: shown %d limit %d truncated %v total %v", available, name, s.shown, s.limit, s.truncated, s.total)
			}
		}
		if r.Complete == truncated {
			return fmt.Errorf("%d available: complete = %v", available, r.Complete)
		}
	}
	return nil
}

// M1-CMP-004: the window, not the parent's duration, bounds the evidence.
func (c *checker) m1Async(ctx context.Context) error {
	r, err := c.report(ctx, scM1Async, 1)
	if err != nil {
		return err
	}
	if err := expectNames("async spans", r.Spans.Records, "async_parent", "async_child"); err != nil {
		return err
	}
	return expectList("async logs", c.logAliases(r.Logs.Records), []string{"async-log"})
}

// M1-CMP-005 (report side of RF06): a parent outside the window is absent,
// and present once the window is widened.
func (c *checker) m1Widen(ctx context.Context) error {
	from, to := c.d.window()
	trace := c.d.traceID(scRF06, 1)
	narrow, err := c.api.report(ctx, reportRequest{TraceID: trace, TimeFrom: ns(from), TimeTo: ns(to)})
	if err != nil {
		return err
	}
	if err := expectNames("report in the default window", narrow.Spans.Records, "rf06.child"); err != nil {
		return err
	}
	wide, err := c.api.report(ctx, reportRequest{TraceID: trace, TimeFrom: ns(from.Add(-time.Hour)), TimeTo: ns(to.Add(time.Hour))})
	if err != nil {
		return err
	}
	return expectNames("report in the widened window", wide.Spans.Records, "rf06.child", "rf06.parent")
}

// M1-COR-006: logs stay available when the trace has no spans.
func (c *checker) m1LogsOnly(ctx context.Context) error {
	r, err := c.report(ctx, scM1Logs, 1)
	if err != nil {
		return err
	}
	if len(r.Spans.Records) != 0 || r.Spans.Total == nil || *r.Spans.Total != 0 {
		return fmt.Errorf("spans %v, want a successful empty section", names(r.Spans.Records))
	}
	return expectList("logs without spans", c.logAliases(r.Logs.Records), []string{"nospans-error", "nospans-info"})
}

// m1Late is REL-003 before (delivered false) and after the parent arrived.
func (c *checker) m1Late(ctx context.Context, delivered bool) error {
	r, err := c.report(ctx, scM1Late, 1)
	if err != nil {
		return err
	}
	if err := expectList("late logs", c.logAliases(r.Logs.Records), []string{"late-ref"}); err != nil {
		return err
	}
	if c.logByAlias(r.Logs.Records, "late-ref").SpanID != idLateRoot {
		return fmt.Errorf("late-ref lost its recorded span id")
	}
	if !delivered {
		return expectNames("late trace before the parent", r.Spans.Records, "late_child")
	}
	if err := expectNames("late trace after the parent", r.Spans.Records, "late_child", "late_parent"); err != nil {
		return err
	}
	kid, _ := byName(r.Spans.Records, "late_child")
	parent, _ := byName(r.Spans.Records, "late_parent")
	if kid.ParentSpanID != parent.SpanID || parent.SpanID != idLateRoot {
		return fmt.Errorf("the late parent is not attached")
	}
	return nil
}

// M1-REF-001: find the ERROR log by service and message, open its report
// without entering a TraceId; the detail is only in the log.
func (c *checker) m1Reference(ctx context.Context) error {
	from, to := c.d.window()
	found, err := c.api.allLogs(ctx, logFilter{TimeFrom: ns(from), TimeTo: ns(to), ServiceName: c.d.service("recording-worker"),
		Severity: "ERROR", Message: "AccessDenied"})
	if err != nil {
		return err
	}
	if err := expectList("AccessDenied search", c.logAliases(found), []string{"log-denied"}); err != nil {
		return err
	}
	r, err := c.api.report(ctx, reportRequest{TraceID: found[0].TraceID, TimeFrom: ns(from), TimeTo: ns(to)})
	if err != nil {
		return err
	}
	if err := reportOK(r); err != nil {
		return err
	}
	up, ok := byName(r.Spans.Records, "upload_recording")
	if !ok || up.Status != "error" {
		return fmt.Errorf("the report lacks the failed upload")
	}
	const detail = "Storage returned 403 AccessDenied"
	if c.logByAlias(r.Logs.Records, "log-denied").Message != detail {
		return fmt.Errorf("the report lacks the log detail %q", detail)
	}
	for _, s := range r.Spans.Records {
		for _, t := range append(append([]map[string]string{}, s.Tags...), s.ServiceTags...) {
			if strings.Contains(t["value"], "AccessDenied") {
				return fmt.Errorf("span %s attribute %s contains the detail", s.Name, t["key"])
			}
		}
		for _, e := range s.Events {
			for _, v := range e.Attributes {
				if strings.Contains(v, "AccessDenied") {
					return fmt.Errorf("span %s event %s contains the detail", s.Name, e.Name)
				}
			}
		}
	}
	return nil
}

// m1LogTotal: every confirmed record of the run exactly once; replays and
// retries add none.
func (c *checker) m1LogTotal(ctx context.Context) error {
	logs, err := c.api.allLogs(ctx, logFilter{TimeFrom: ns(c.d.at(-90 * time.Minute)), TimeTo: ns(c.d.at(5 * time.Minute)),
		ServiceName: "fx" + c.d.run, RowsPerPage: 7})
	if err != nil {
		return err
	}
	got := c.logAliases(logs)
	seen := map[string]bool{}
	for _, a := range got {
		if seen[a] {
			return fmt.Errorf("record %s appears twice across pages", a)
		}
		seen[a] = true
	}
	sort.Strings(got)
	return expectList("all log records of the run", got, c.expectedLogAliases())
}

// M1-CMP-006 (API side): a window older than the log retention warns.
func (c *checker) m1Retention(ctx context.Context) error {
	start := time.Now().Add(-80 * time.Hour)
	r, err := c.api.report(ctx, reportRequest{TraceID: c.d.traceID(scM1Rec, 1), TimeFrom: ns(start), TimeTo: ns(start.Add(time.Hour))})
	if err != nil {
		return err
	}
	for _, w := range r.Warnings {
		if w.Section == "logs" && w.Code == "retention" && strings.Contains(w.Message, "not proof") {
			return nil
		}
	}
	return fmt.Errorf("warnings %v, want a log retention warning", r.Warnings)
}

// m1Outage is REF-003, run while the test ClickHouse is stopped: the sender
// must not get a confirmation. retry sends the same record after recovery.
func (c *checker) m1OutageSend(ctx context.Context, o options) error {
	err := sendLogs(ctx, c.d, o.sender, []sender.LogRecord{c.d.outageLog()})
	if err == nil {
		return fmt.Errorf("the log sender got a confirmation while ClickHouse was stopped")
	}
	c.notes["outage log error"] = err.Error()
	return nil
}

func (c *checker) m1OutageRetry(ctx context.Context, o options) error {
	// Retry until confirmed, then once more (a retry after an unconfirmed
	// result): the report must still contain the record once.
	if err := waitFor(ctx, "log delivery after the outage", o.readyTimeout, func() error {
		return sendLogs(ctx, c.d, o.sender, []sender.LogRecord{c.d.outageLog()})
	}); err != nil {
		return err
	}
	c.state.OutageLogSent = true
	if err := c.state.save(o.statePath); err != nil {
		return err
	}
	if err := sendLogs(ctx, c.d, o.sender, []sender.LogRecord{c.d.outageLog()}); err != nil {
		return err
	}
	r, err := c.report(ctx, scM1Out, 1)
	if err != nil {
		return err
	}
	return expectList("logs after the outage", c.logAliases(r.Logs.Records), []string{"outage-log"})
}

// m1WindowEdges: log windows are [from, to) too: a record at exactly "from" is
// in, one at exactly "to" is out.
func (c *checker) m1WindowEdges(ctx context.Context) error {
	at := c.d.m1Base().Add(1000 * time.Millisecond) // log-denied
	for _, tc := range []struct {
		from, to time.Time
		want     []string
	}{
		{at, at.Add(time.Nanosecond), []string{"log-denied"}},
		{at.Add(-time.Nanosecond), at, nil},
	} {
		logs, err := c.api.allLogs(ctx, logFilter{TimeFrom: ns(tc.from), TimeTo: ns(tc.to), ServiceName: c.d.service("recording-worker")})
		if err != nil {
			return err
		}
		if err := expectList(fmt.Sprintf("logs in [%s, %s)", ns(tc.from), ns(tc.to)), c.logAliases(logs), tc.want); err != nil {
			return err
		}
	}
	return nil
}

var errNotSent = errors.New("not sent")

// m1Checks run on every pass (initial, after replay, after recreation).
func (c *checker) m1Checks() []namedCheck {
	return []namedCheck{
		{"M1-RPT-001", "Spans and all related logs in one report", c.m1Recording},
		{"M1-RPT-002", "Error span search, then the trace's full context", c.m1ErrorSpanEntry},
		{"M1-RPT-005", "Error log search without a TraceId opens its trace", c.m1ErrorLogEntry},
		{"M1-RPT-008", "Single-trace scope; linked trace kept apart", c.m1Boundary},
		{"M1-COR-001", "Same SpanId in two traces", c.m1SharedSpanID},
		{"M1-COR-003", "Trace-level logs; identical events kept apart", c.m1Context},
		{"M1-COR-006", "Logs of a trace without spans", c.m1LogsOnly},
		{"M1-CMP-003", "Limit of 3: complete at 3, truncated at 4", c.m1Limit},
		{"M1-CMP-004", "Async child and late log inside the window", c.m1Async},
		{"M1-CMP-005", "Report widens to a parent outside the window", c.m1Widen},
		{"M1-CMP-006", "Old window warns about log retention", c.m1Retention},
		{"M1-REF-001", "Reference investigation from the error log", c.m1Reference},
		{"M1-REL-001", "Every log record once, across pages and replays", c.m1LogTotal},
		{"M1-WIN", "Log window edges are [from, to)", c.m1WindowEdges},
		{"M1-REL-003", "Late parent attached after refresh", func(ctx context.Context) error {
			if !c.state.M1LateSent {
				return errNotSent
			}
			return c.m1Late(ctx, true)
		}},
	}
}
