package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	sender "github.com/agerimex/troubleshooting/log-sender"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// Sequence investigation (troubleshooting-sequence-agent): the package's base
// trace with real CLIENT/SERVER pairs across three services, sent by OTLP, and
// its 12 logs (one trace-level). Services seq-api, seq-storage, seq-thumbs;
// at T+5h40m, apart from the problem workspace cases.

const scSeq = 0x0701

func (d dataset) seqStart() time.Time { return d.m2Hour(5).Add(40 * time.Minute) }

func (d dataset) seqSpans() []*collectortracepb.ExportTraceServiceRequest {
	at := d.seqStart()
	trace := d.traceID(scSeq, 1)
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	errs, ok := tracepb.Status_STATUS_CODE_ERROR, tracepb.Status_STATUS_CODE_OK
	res := map[string]string{"service.version": "1.1.0", "deployment.environment.name": "demo", "troubleshooting.sampling_policy": "unsampled"}
	api := []spanSpec{
		{trace: trace, id: "0000000000000001", name: "publish_recording", kind: tracepb.Span_SPAN_KIND_SERVER, start: at, dur: ms(240), status: errs},
		{trace: trace, id: "0000000000000002", parent: "0000000000000001", name: "upload_recording", kind: tracepb.Span_SPAN_KIND_INTERNAL, start: at.Add(ms(25)), dur: ms(200), status: errs},
		{trace: trace, id: "0000000000000003", parent: "0000000000000002", name: "PutObject", kind: tracepb.Span_SPAN_KIND_CLIENT, start: at.Add(ms(40)), dur: ms(172), status: errs,
			attrs: map[string]string{"server.address": "object-storage", "http.response.status_code": "403"}},
		{trace: trace, id: "0000000000000005", parent: "0000000000000001", name: "GenerateThumbnail", kind: tracepb.Span_SPAN_KIND_CLIENT, start: at.Add(ms(30)), dur: ms(122), status: ok},
	}
	storage := spanSpec{trace: trace, id: "0000000000000004", parent: "0000000000000003", name: "PUT /recordings", kind: tracepb.Span_SPAN_KIND_SERVER,
		start: at.Add(ms(42)), dur: ms(168), status: errs, attrs: map[string]string{"http.response.status_code": "403"}}
	thumbs := spanSpec{trace: trace, id: "0000000000000006", parent: "0000000000000005", name: "generate_thumbnail", kind: tracepb.Span_SPAN_KIND_SERVER,
		start: at.Add(ms(32)), dur: ms(118), status: ok}
	return []*collectortracepb.ExportTraceServiceRequest{merge(
		request(d.service("seq-api"), res, api...),
		request(d.service("seq-storage"), res, storage),
		request(d.service("seq-thumbs"), res, thumbs))}
}

func (d dataset) seqLogs() []sender.LogRecord {
	at := d.seqStart()
	trace := d.traceID(scSeq, 1)
	l := func(alias string, n int, sev, svc, op, span, task, msg string) sender.LogRecord {
		return d.m2Log("seq-"+alias, at.Add(time.Duration(n)*time.Millisecond), sev, svc, msg, trace, span, pwLogAttrs(op, task))
	}
	return []sender.LogRecord{
		l("01", 3, "INFO", "seq-api", "publish_recording", "0000000000000001", "", "Recording request accepted"),
		l("02", 26, "INFO", "seq-api", "upload_recording", "0000000000000002", "upload", "Upload started"),
		l("03", 35, "INFO", "seq-api", "upload_recording", "0000000000000002", "metadata", "Preparing metadata"),
		l("04", 41, "INFO", "seq-api", "PutObject", "0000000000000003", "", "Sending object to storage"),
		l("05", 205, "ERROR", "seq-storage", "PUT /recordings", "0000000000000004", "", "AccessDenied: bucket policy rejected the request"),
		l("06", 211, "ERROR", "seq-api", "PutObject", "0000000000000003", "", "Storage returned HTTP 403"),
		l("07", 215, "ERROR", "seq-api", "upload_recording", "0000000000000002", "upload", "Upload failed: AccessDenied"),
		l("08", 218, "ERROR", "seq-api", "upload_recording", "0000000000000002", "upload", "Could not persist recording"),
		l("09", 80, "INFO", "seq-api", "upload_recording", "0000000000000002", "metadata", "Metadata prepared"),
		l("10", 149, "INFO", "seq-thumbs", "generate_thumbnail", "0000000000000006", "", "Thumbnail generated"),
		l("11", 238, "WARN", "seq-api", "publish_recording", "0000000000000001", "", "Recording was not published"),
		l("12", 239, "WARN", "seq-api", "", "", "", "Trace-level audit event: publication incomplete"),
	}
}

// SEQ-SHAPE-001: the sequence library groups the sequence trace as one
// execution of its own shape: entry publish_recording, the 5 distinct calls
// (api->api, api->storage, api->thumbs pairs), 3 services, failed, with the
// trace as its slowest example and no percentiles from one execution. A few
// minutes (computed from raw spans) and the whole hour (stored when the hour
// has a revision) give the same shape.
func (c *checker) seqShape(ctx context.Context) error {
	at := c.d.seqStart()
	trace := c.d.traceID(scSeq, 1)
	hour := at.Truncate(time.Hour)
	for _, w := range [][2]time.Time{{at.Add(-time.Minute), at.Add(time.Minute)}, {hour, hour.Add(time.Hour)}} {
		var data struct {
			Library struct {
				Status string `json:"status"`
				Error  string `json:"error"`
				Shapes []struct {
					EntryService     string              `json:"entryService"`
					EntryOperation   string              `json:"entryOperation"`
					Services         []string            `json:"services"`
					DistinctCalls    int                 `json:"distinctCalls"`
					Executions       int                 `json:"executions"`
					FailedExecutions int                 `json:"failedExecutions"`
					Percentiles      map[string]*float64 `json:"percentiles"`
					Examples         map[string]*struct {
						TraceID    string `json:"traceId"`
						DurationUs int64  `json:"durationUs"`
					} `json:"examples"`
				} `json:"shapes"`
			} `json:"Library"`
		}
		req := map[string]any{"time_from": ns(w[0]), "time_to": ns(w[1]), "service": c.d.service("seq-")}
		if err := c.api.post(ctx, "/api/v1/stats/shapes", req, &data); err != nil {
			return err
		}
		lib := data.Library
		if lib.Status != "ok" || len(lib.Shapes) != 1 {
			return fmt.Errorf("window %s: status %s %s, %d shapes, want 1", w[0].Format("15:04"), lib.Status, lib.Error, len(lib.Shapes))
		}
		s := lib.Shapes[0]
		got := fmt.Sprintf("%s %s calls=%d services=%d executions=%d failed=%d", strings.TrimPrefix(s.EntryService, "fx"+c.d.run+"-"),
			s.EntryOperation, s.DistinctCalls, len(s.Services), s.Executions, s.FailedExecutions)
		if want := "seq-api publish_recording calls=5 services=3 executions=1 failed=1"; got != want {
			return fmt.Errorf("window %s: %s, want %s", w[0].Format("15:04"), got, want)
		}
		if s.Percentiles["p50"] != nil || s.Examples["slowest"] == nil || s.Examples["slowest"].TraceID != trace || s.Examples["slowest"].DurationUs != 240000 {
			return fmt.Errorf("window %s: percentiles %v, slowest %+v", w[0].Format("15:04"), s.Percentiles, s.Examples["slowest"])
		}
	}
	return nil
}

type apiSpanKind struct {
	SpanID   string `json:"spanId"`
	SpanKind string `json:"spanKind"`
	Service  string `json:"service"`
}

// SEQ-REAL-001: the combined report of the sequence trace carries what the
// sequence needs: span kinds of a CLIENT/SERVER pair across services, the
// server-side ERROR log on its own span, and the trace-level log.
func (c *checker) seqReport(ctx context.Context) error {
	at := c.d.seqStart()
	trace := c.d.traceID(scSeq, 1)
	var data struct {
		Report struct {
			Spans struct {
				Status  string        `json:"status"`
				Records []apiSpanKind `json:"records"`
			} `json:"spans"`
			Logs struct {
				Status  string   `json:"status"`
				Records []apiLog `json:"records"`
			} `json:"logs"`
		} `json:"Report"`
	}
	if err := c.api.post(ctx, "/api/v1/trace-report", reportRequest{TraceID: trace, TimeFrom: ns(at.Add(-time.Minute)), TimeTo: ns(at.Add(time.Minute))}, &data); err != nil {
		return err
	}
	r := data.Report
	if r.Spans.Status != "ok" || r.Logs.Status != "ok" {
		return fmt.Errorf("sections %s/%s", r.Spans.Status, r.Logs.Status)
	}
	var kinds []string
	for _, s := range r.Spans.Records {
		kinds = append(kinds, s.SpanID[14:]+"="+s.SpanKind+"@"+strings.TrimPrefix(s.Service, "fx"+c.d.run+"-"))
	}
	sort.Strings(kinds)
	want := []string{"01=server@seq-api", "02=internal@seq-api", "03=client@seq-api", "04=server@seq-storage", "05=client@seq-api", "06=server@seq-thumbs"}
	if strings.Join(kinds, " ") != strings.Join(want, " ") {
		return fmt.Errorf("spans %v, want %v", kinds, want)
	}
	if len(r.Logs.Records) != 12 {
		return fmt.Errorf("%d logs, want 12", len(r.Logs.Records))
	}
	var serverError, traceLevel bool
	for _, l := range r.Logs.Records {
		serverError = serverError || (l.SpanID == "0000000000000004" && l.Severity == "ERROR")
		traceLevel = traceLevel || (l.SpanID == "" && strings.HasPrefix(l.Message, "Trace-level"))
	}
	if !serverError || !traceLevel {
		return fmt.Errorf("server ERROR log %v, trace-level log %v", serverError, traceLevel)
	}
	return nil
}
