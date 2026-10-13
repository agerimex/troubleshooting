package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	sender "github.com/agerimex/troubleshooting/log-sender"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// Problem workspace (troubleshooting-ui-agent): the reference traces of its
// UI fixture (an upload that fails next to a parallel thumbnail, and the same
// call with all spans OK but an ERROR log), with task.id lanes, and small
// cases for the problem counts. Services pw-api and pw-worker; all records in
// [T+5h30m, T+6h).

const (
	scPWFailed   = 0x0601 // the failed upload (PW-INV-001/002)
	scPWLogOnly  = 0x0602 // OK spans, ERROR log (PW-INV-008)
	scPWRepeat   = 0x0603 // 3 ERROR logs of one span (PW-GRP-002)
	scPWTwoCalls = 0x0604 // 2 failing calls in one trace (PW-GRP-003)
	scPWSlow     = 0x0605 // slow calls (rule: > 500 ms, at least 5)
)

const (
	idPWRoot   = "0000000000000001"
	idPWUpload = "0000000000000002"
	idPWThumb  = "0000000000000003"
	idPWStore  = "0000000000000004"
)

func (d dataset) pwStart() time.Time { return d.m2Hour(5).Add(30 * time.Minute) }

func pwResource() map[string]string {
	return map[string]string{"service.version": "1.1.0", "deployment.environment.name": "production", "troubleshooting.sampling_policy": "unsampled"}
}

func pwLogAttrs(operation, task string) map[string]string {
	a := map[string]string{"operation.name": operation, "service.version": "1.1.0", "deployment.environment.name": "production"}
	if task != "" {
		a["task.id"] = task
	}
	return a
}

// pwTrace is one reference trace: publish [0,240) ms, upload [25,225),
// storage [40,210) under upload, thumbnail [30,150) next to upload.
func (d dataset) pwTrace(sc int, at time.Time, failed bool) *collectortracepb.ExportTraceServiceRequest {
	trace := d.traceID(sc, 1)
	st := func(err bool) tracepb.Status_StatusCode {
		if err {
			return tracepb.Status_STATUS_CODE_ERROR
		}
		return tracepb.Status_STATUS_CODE_OK
	}
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	api := spanSpec{trace: trace, id: idPWRoot, name: "publish_recording", kind: tracepb.Span_SPAN_KIND_SERVER, start: at, dur: ms(240), status: st(failed)}
	worker := []spanSpec{
		{trace: trace, id: idPWUpload, parent: idPWRoot, name: "upload_recording", kind: tracepb.Span_SPAN_KIND_CONSUMER, start: at.Add(ms(25)), dur: ms(200), status: st(failed)},
		{trace: trace, id: idPWStore, parent: idPWUpload, name: "storage.PutObject", kind: tracepb.Span_SPAN_KIND_CLIENT, start: at.Add(ms(40)), dur: ms(170), status: st(failed)},
		{trace: trace, id: idPWThumb, parent: idPWRoot, name: "create_thumbnail", start: at.Add(ms(30)), dur: ms(120), status: tracepb.Status_STATUS_CODE_OK},
	}
	return merge(request(d.service("pw-api"), pwResource(), api), request(d.service("pw-worker"), pwResource(), worker...))
}

// pwTraceLogs are the reference trace's 12 logs: two lanes (transfer,
// checksum) and one log without a task in upload, the 403 ERROR in transfer.
func (d dataset) pwTraceLogs(sc int, at time.Time, prefix string) []sender.LogRecord {
	trace := d.traceID(sc, 1)
	ms := func(n int) time.Time { return at.Add(time.Duration(n) * time.Millisecond) }
	l := func(alias string, n int, sev, svc, op, span, task, msg string) sender.LogRecord {
		return d.m2Log(prefix+"-"+alias, ms(n), sev, svc, msg, trace, span, pwLogAttrs(op, task))
	}
	return []sender.LogRecord{
		l("r1", 1, "INFO", "pw-api", "publish_recording", idPWRoot, "", "Publish request received"),
		l("r2", 238, "ERROR", "pw-api", "publish_recording", idPWRoot, "", "Recording publish failed"),
		l("u1", 26, "INFO", "pw-worker", "upload_recording", idPWUpload, "", "Upload scheduled"),
		l("u2", 35, "INFO", "pw-worker", "upload_recording", idPWUpload, "transfer", "Transfer started"),
		l("u3", 60, "INFO", "pw-worker", "upload_recording", idPWUpload, "checksum", "Checksum started"),
		l("u4", 100, "INFO", "pw-worker", "upload_recording", idPWUpload, "checksum", "Checksum done"),
		l("u5", 125, "WARN", "pw-worker", "upload_recording", idPWUpload, "transfer", "Transfer retry 1 of 3"),
		l("u6", 205, "ERROR", "pw-worker", "upload_recording", idPWUpload, "transfer", "Storage returned 403 AccessDenied"),
		l("s1", 41, "INFO", "pw-worker", "storage.PutObject", idPWStore, "", "PUT recordings/object"),
		l("s2", 209, "ERROR", "pw-worker", "storage.PutObject", idPWStore, "", "HTTP 403 from storage"),
		l("t1", 31, "INFO", "pw-worker", "create_thumbnail", idPWThumb, "thumbnail", "Thumbnail generation started"),
		l("t2", 149, "INFO", "pw-worker", "create_thumbnail", idPWThumb, "thumbnail", "Thumbnail ready"),
	}
}

func (d dataset) pwSpans() []*collectortracepb.ExportTraceServiceRequest {
	at := d.pwStart()
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	repeat := spanSpec{trace: d.traceID(scPWRepeat, 1), id: idRoot, name: "verify_recording", kind: tracepb.Span_SPAN_KIND_CONSUMER,
		start: at.Add(time.Minute), dur: ms(50), status: tracepb.Status_STATUS_CODE_OK}
	two := d.traceID(scPWTwoCalls, 1)
	twoCalls := []spanSpec{
		{trace: two, id: idRoot, name: "batch_upload", kind: tracepb.Span_SPAN_KIND_CONSUMER, start: at.Add(2 * time.Minute), dur: ms(300), status: tracepb.Status_STATUS_CODE_OK},
		{trace: two, id: idChild1, parent: idRoot, name: "store_part", start: at.Add(2*time.Minute + ms(10)), dur: ms(100), status: tracepb.Status_STATUS_CODE_ERROR},
		{trace: two, id: idChild2, parent: idRoot, name: "store_part", start: at.Add(2*time.Minute + ms(120)), dur: ms(100), status: tracepb.Status_STATUS_CODE_ERROR},
	}
	var slow []spanSpec
	for i := 0; i < 6; i++ {
		slow = append(slow, spanSpec{trace: d.traceID(scPWSlow, i+1), id: idRoot, name: "render_preview", kind: tracepb.Span_SPAN_KIND_SERVER,
			start: at.Add(3*time.Minute + time.Duration(i)*time.Second), dur: ms(800 + 10*i), status: tracepb.Status_STATUS_CODE_OK})
	}
	return []*collectortracepb.ExportTraceServiceRequest{
		d.pwTrace(scPWFailed, at, true),
		d.pwTrace(scPWLogOnly, at.Add(30*time.Second), false),
		request(d.service("pw-worker"), pwResource(), append(append([]spanSpec{repeat}, twoCalls...), slow...)...),
	}
}

func (d dataset) pwLogs() []sender.LogRecord {
	at := d.pwStart()
	logs := append(d.pwTraceLogs(scPWFailed, at, "pw-failed"), d.pwTraceLogs(scPWLogOnly, at.Add(30*time.Second), "pw-logonly")...)
	repeat := d.traceID(scPWRepeat, 1)
	for i := 0; i < 3; i++ {
		logs = append(logs, d.m2Log(fmt.Sprintf("pw-repeat-%d", i), at.Add(time.Minute+time.Duration(10+i)*time.Millisecond), "ERROR", "pw-worker",
			"Checksum mismatch on uploaded part", repeat, idRoot, pwLogAttrs("verify_recording", "")))
	}
	two := d.traceID(scPWTwoCalls, 1)
	for i, span := range []string{idChild1, idChild2} {
		logs = append(logs, d.m2Log(fmt.Sprintf("pw-two-%d", i), at.Add(2*time.Minute+time.Duration(50+120*i)*time.Millisecond), "ERROR", "pw-worker",
			"Storage quota exceeded", two, span, pwLogAttrs("store_part", "")))
	}
	logs = append(logs, d.m2Log("pw-unlinked", at.Add(4*time.Minute), "ERROR", "pw-worker", "Cache volume is full", "", "", pwLogAttrs("cache_cleanup", "")))
	return logs
}

// --- checks

type apiProblem struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Operation string         `json:"operation"`
	Qualified bool           `json:"qualified"`
	Attrib    string         `json:"attribution"`
	Error     *apiErrorGroup `json:"error"`
	Slow      *struct {
		Calls int `json:"calls"`
	} `json:"slow"`
	Regression *struct {
		AnalysisID string    `json:"analysisId"`
		Metric     apiMetric `json:"metric"`
		Baseline   struct {
			Version *string `json:"version"`
		} `json:"baseline"`
		Candidate struct {
			Version *string `json:"version"`
		} `json:"candidate"`
	} `json:"regression"`
	Explanation struct {
		RuleRevision int64 `json:"ruleRevision"`
	} `json:"explanation"`
}

type apiProblemList struct {
	Status   string                      `json:"status"`
	Problems []apiProblem                `json:"problems"`
	Total    int                         `json:"total"`
	Next     string                      `json:"next"`
	Waiting  string                      `json:"waiting"`
	Sources  map[string]apiProblemSource `json:"sources"`
	Rules    apiRules                    `json:"rules"`
}

type apiProblemSource struct {
	Status    string `json:"status"`
	Condition string `json:"condition"`
}

type apiRules struct {
	Scope    string `json:"scope"`
	Revision int64  `json:"revision"`
	Errors   struct {
		Enabled          bool   `json:"enabled"`
		MinAffectedCalls int    `json:"minAffectedCalls"`
		Grouping         string `json:"grouping"`
	} `json:"errors"`
	Slow struct {
		Enabled    bool    `json:"enabled"`
		DurationMs float64 `json:"durationMs"`
		MinCalls   int     `json:"minCalls"`
	} `json:"slow"`
	Regressions struct {
		Enabled        bool    `json:"enabled"`
		P95RelativePct float64 `json:"p95RelativePct"`
		P95AbsoluteMs  float64 `json:"p95AbsoluteMs"`
	} `json:"regressions"`
}

func (a *api) problems(ctx context.Context, body map[string]any) (apiProblemList, error) {
	var data struct {
		Problems apiProblemList `json:"Problems"`
	}
	err := a.post(ctx, "/api/v1/problems", body, &data)
	return data.Problems, err
}

func (a *api) rules(ctx context.Context) (apiRules, error) {
	var data struct {
		Rules apiRules `json:"Rules"`
	}
	err := a.get(ctx, "/api/v1/detection-rules", &data)
	return data.Rules, err
}

func (a *api) saveRules(ctx context.Context, r apiRules, expected int64) (apiRules, error) {
	var data struct {
		Rules apiRules `json:"Rules"`
	}
	err := a.post(ctx, "/api/v1/detection-rules", map[string]any{"rules": r, "expected_revision": expected}, &data)
	return data.Rules, err
}

func (c *checker) pwList(ctx context.Context, service string, from, to time.Time) (apiProblemList, error) {
	return c.api.problems(ctx, map[string]any{"service": c.d.service(service), "time_from": ns(from), "time_to": ns(to)})
}

func findProblem(l apiProblemList, match func(apiProblem) bool) (apiProblem, error) {
	for _, p := range l.Problems {
		if match(p) {
			return p, nil
		}
	}
	return apiProblem{}, fmt.Errorf("no such problem among %d", len(l.Problems))
}

func logGroup(signature string) func(apiProblem) bool {
	return func(p apiProblem) bool {
		return p.Error != nil && p.Error.SourceKind == "log" && p.Error.Signature == signature
	}
}

func calls(p apiProblem) string {
	if p.Error == nil || p.Error.AffectedCalls == nil {
		return "unavailable"
	}
	return strconv.Itoa(*p.Error.AffectedCalls)
}

// PW-GRP-002/003/004/005 and slow calls: counts from the prepared error
// statistics of pw-worker.
func (c *checker) pwGroups(ctx context.Context) error {
	l, err := c.pwList(ctx, "pw-worker", c.d.m2Hour(5), c.d.m2Hour(6))
	if err != nil {
		return err
	}
	if l.Status != "available" {
		return fmt.Errorf("list status %s", l.Status)
	}
	repeat, err := findProblem(l, logGroup("Checksum mismatch on uploaded part"))
	if err != nil {
		return fmt.Errorf("PW-GRP-002: %w", err)
	}
	if repeat.Error.Occurrences != 3 || calls(repeat) != "1" {
		return fmt.Errorf("PW-GRP-002: 3 logs of one span: occurrences %d, calls %s; want 3 and 1", repeat.Error.Occurrences, calls(repeat))
	}
	two, err := findProblem(l, logGroup("Storage quota exceeded"))
	if err != nil {
		return fmt.Errorf("PW-GRP-003: %w", err)
	}
	if calls(two) != "2" || two.Error.AffectedTraces != 1 {
		return fmt.Errorf("PW-GRP-003: calls %s traces %d, want 2 and 1", calls(two), two.Error.AffectedTraces)
	}
	unlinked, err := findProblem(l, logGroup("Cache volume is full"))
	if err != nil {
		return fmt.Errorf("PW-GRP-004: %w", err)
	}
	if unlinked.Qualified || unlinked.Attrib == "" || calls(unlinked) != "0" {
		return fmt.Errorf("PW-GRP-004: a log without TraceId must be listed unqualified with an attribution limitation: %+v", unlinked)
	}
	// PW-GRP-005 / INV-008: the AccessDenied logs (one on an ERROR span, one
	// on an OK span) and the ERROR-status uploads are separate problems.
	denied, err := findProblem(l, logGroup("Storage returned 403 AccessDenied"))
	if err != nil {
		return fmt.Errorf("PW-INV-008: %w", err)
	}
	if calls(denied) != "2" || denied.Error.AffectedTraces != 2 {
		return fmt.Errorf("PW-INV-008: AccessDenied calls %s traces %d, want 2 and 2 (one with all spans OK)", calls(denied), denied.Error.AffectedTraces)
	}
	status, err := findProblem(l, func(p apiProblem) bool {
		return p.Error != nil && p.Error.SourceKind == "span_status" && p.Operation == "upload_recording"
	})
	if err != nil {
		return fmt.Errorf("PW-GRP-005: %w", err)
	}
	if status.ID == denied.ID || calls(status) != "1" {
		return fmt.Errorf("PW-GRP-005: ERROR-status uploads %s with %s call(s)", status.ID, calls(status))
	}
	slow, err := findProblem(l, func(p apiProblem) bool { return p.Type == "slow_calls" && p.Operation == "render_preview" })
	if err != nil {
		return fmt.Errorf("slow calls: %w", err)
	}
	if slow.Slow == nil || slow.Slow.Calls != 6 {
		return fmt.Errorf("slow calls: %+v, want 6 calls over 500 ms", slow.Slow)
	}
	return nil
}

// PW-RUL-007 / regression problems come from stored comparisons only.
func (c *checker) pwRegression(ctx context.Context) error {
	l, err := c.pwList(ctx, "recording-worker", c.d.m2T(), c.d.m2Hour(6))
	if err != nil {
		return err
	}
	p, err := findProblem(l, func(p apiProblem) bool {
		return p.Regression != nil && p.Regression.Baseline.Version != nil && *p.Regression.Baseline.Version == "1.0.0" &&
			p.Regression.Candidate.Version != nil && *p.Regression.Candidate.Version == "1.1.0"
	})
	if err != nil {
		return fmt.Errorf("the 1.0.0 → 1.1.0 regression: %w", err)
	}
	if err := firstErr(near("p95 baseline", p.Regression.Metric.Baseline, 100, msTol), near("p95 candidate", p.Regression.Metric.Candidate, 160, msTol)); err != nil {
		return err
	}
	for _, q := range l.Problems {
		if q.Regression != nil && q.Regression.Candidate.Version != nil && *q.Regression.Candidate.Version == "1.1.1" {
			return fmt.Errorf("the fix 1.1.1 is listed as a regression (%s)", q.ID)
		}
	}
	none, err := c.pwList(ctx, "pw-worker", c.d.m2Hour(5), c.d.m2Hour(6))
	if err != nil {
		return err
	}
	if none.Waiting == "" {
		return fmt.Errorf("PW-RUL-007: a service without comparisons must wait for comparable release data")
	}
	return nil
}

// PW-RUL-001/004: saves are durable revisions with optimistic concurrency.
// Each phase starts from what the previous phase stored, so a restart or
// recreation in between shows that the rules persisted.
func (c *checker) pwRules(ctx context.Context) error {
	current, err := c.api.rules(ctx)
	if err != nil {
		return err
	}
	if current.Revision > 0 && current.Slow.DurationMs != 500 {
		return fmt.Errorf("revision %d has slow duration %g; the previous phase stored 500", current.Revision, current.Slow.DurationMs)
	}
	edited := current
	edited.Slow.DurationMs = 700
	saved, err := c.api.saveRules(ctx, edited, current.Revision)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}
	if saved.Revision != current.Revision+1 || saved.Slow.DurationMs != 700 {
		return fmt.Errorf("saved revision %d duration %g, want %d and 700", saved.Revision, saved.Slow.DurationMs, current.Revision+1)
	}
	if read, err := c.api.rules(ctx); err != nil || read.Revision != saved.Revision || read.Slow.DurationMs != 700 {
		return fmt.Errorf("read back: %+v, %v", read, err)
	}
	_, err = c.api.saveRules(ctx, edited, current.Revision)
	var ae *apiError
	if !errors.As(err, &ae) || ae.Status != 409 {
		return fmt.Errorf("a save based on revision %d after %d must conflict (409), got %v", current.Revision, saved.Revision, err)
	}
	invalid := saved
	invalid.Slow.DurationMs = -5
	if _, err := c.api.saveRules(ctx, invalid, saved.Revision); !errors.As(err, &ae) || ae.Status != 400 {
		return fmt.Errorf("a negative duration must be rejected (400), got %v", err)
	}
	restored := saved
	restored.Slow.DurationMs = 500
	if _, err := c.api.saveRules(ctx, restored, saved.Revision); err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	return nil
}

func (c *checker) pwChecks() []namedCheck {
	return []namedCheck{
		{"SEQ-REAL-001", "Sequence trace: CLIENT/SERVER kinds across services, server and trace-level logs", c.seqReport},
		{"SEQ-SHAPE-001", "Sequence library: the sequence trace is one execution of its own shape, raw and stored", c.seqShape},
		{"PW-RUL-001", "Detection rules: durable revisions, conflict, validation (with PW-RUL-002/004)", c.pwRules},
		{"PW-GRP-002", "Problem counts: calls vs records vs traces (with PW-GRP-003/004/005, PW-INV-008, slow calls)", c.pwGroups},
		{"PW-RUL-007", "Release regressions only from stored comparisons", c.pwRegression},
	}
}
