package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// Quality gate (milestone 3, stage 1; docs/quality-gate.md): the reference
// case as four test runs of recording-api start_session, sent by OTLP at
// T+6h (after the milestone 2 data, before the RF/M1 windows):
//
//	baseline  200 executions, p95 420 ms, 1 permission lookup + 1 insert
//	candidate 200 executions, p95 910 ms, 20 lookups + 1 insert, and a call
//	          to the directory service
//	fixed     200 executions, p95 560 ms, 1 batched lookup + 1 insert
//	few       19 executions shaped like fixed (too few for a p95)
//
// Only qg-api spans carry troubleshooting.run (on the resource); the
// directory service's span belongs to the run through its parent.
// The expected answers below are literal, not computed with the API's code.

const scQG = 0x0801

var qgRuns = []struct {
	name  string
	p95   float64
	count int
}{{"baseline", 420, 200}, {"candidate", 910, 200}, {"fixed", 560, 200}, {"few", 560, 19}}

func (d dataset) qgStart(run int) time.Time {
	return d.m2Hour(6).Add(time.Duration(run) * 5 * time.Minute)
}

func (d dataset) qgRun(name string) string { return "fx" + d.run + "-qg-" + name }

// qgDuration gives execution i of 200 a duration whose nearest-rank p95 (the
// 190th smallest) is exactly p95: 189 values in [p95-60, p95-11], p95 itself,
// then 10 values of p95+100.
func qgDuration(i int, p95 float64) time.Duration {
	ms := p95 + 100
	switch {
	case i < 189:
		ms = p95 - 60 + float64(i%50)
	case i == 189:
		ms = p95
	}
	return time.Duration(ms * float64(time.Millisecond))
}

// qgClientID is the id of the candidate's CLIENT call to the directory.
const qgClientID = "00000000000000d1"

func (d dataset) qgSpans() []*collectortracepb.ExportTraceServiceRequest {
	var reqs []*collectortracepb.ExportTraceServiceRequest
	ok := tracepb.Status_STATUS_CODE_OK
	server, client, internal := tracepb.Span_SPAN_KIND_SERVER, tracepb.Span_SPAN_KIND_CLIENT, tracepb.Span_SPAN_KIND_INTERNAL
	db := map[string]string{"db.system": "postgresql"}
	for ri, r := range qgRuns {
		res := map[string]string{"service.version": r.name, "deployment.environment.name": "qg", "troubleshooting.run": d.qgRun(r.name)}
		var api, dir []spanSpec
		for i := 0; i < r.count; i++ {
			trace := d.traceID(scQG+ri, i+1)
			at := d.qgStart(ri).Add(time.Duration(i) * 600 * time.Millisecond)
			ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
			// The recorded calls account for the difference between the
			// versions: a lookup takes 22 ms (the candidate makes twenty), the
			// batched one 140 ms, the directory call 45 ms. The rest of each
			// execution (about 300-500 ms) is work outside recorded spans,
			// alike in every version.
			lookups, name, step := 1, "permission_lookup", 23
			switch r.name {
			case "candidate":
				lookups = 20
			case "fixed", "few":
				name, step = "permission_batch_lookup", 141
			}
			permEnd := 2 + lookups*step
			api = append(api,
				spanSpec{trace: trace, id: idRoot, name: "start_session", kind: server, start: at, dur: qgDuration(i, r.p95), status: ok},
				spanSpec{trace: trace, id: idChild1, parent: idRoot, name: "check_permissions", kind: internal, start: at.Add(ms(1)), dur: ms(permEnd - 1), status: ok})
			for k := 0; k < lookups; k++ {
				api = append(api, spanSpec{trace: trace, id: fmt.Sprintf("00000000000001%02x", k), parent: idChild1, name: name, kind: client,
					start: at.Add(ms(2 + k*step)), dur: ms(step - 1), status: ok, attrs: db})
			}
			api = append(api, spanSpec{trace: trace, id: idChild2, parent: idRoot, name: "insert_session", kind: client,
				start: at.Add(ms(permEnd + 3)), dur: ms(4), status: ok, attrs: db})
			if r.name == "candidate" {
				api = append(api, spanSpec{trace: trace, id: qgClientID, parent: idRoot, name: "GET /users", kind: client,
					start: at.Add(ms(permEnd + 10)), dur: ms(45), status: ok})
				dir = append(dir, spanSpec{trace: trace, id: "00000000000000d2", parent: qgClientID, name: "GET /users/{id}", kind: server,
					start: at.Add(ms(permEnd + 11)), dur: ms(43), status: ok})
			}
		}
		parts := []*collectortracepb.ExportTraceServiceRequest{request(d.service("qg-api"), res, api...)}
		if len(dir) > 0 {
			parts = append(parts, request(d.service("qg-directory"), map[string]string{"deployment.environment.name": "qg"}, dir...))
		}
		reqs = append(reqs, merge(parts...))
	}
	return reqs
}

// The API's answer, as the fixture reads it.
type qgEvidence struct {
	TraceID  string `json:"trace_id"`
	SpanID   string `json:"span_id"`
	TimeFrom string `json:"time_from"`
	TimeTo   string `json:"time_to"`
}

type qgCheck struct {
	ID           string       `json:"id"`
	Status       string       `json:"status"`
	Reason       string       `json:"reason"`
	Value        *float64     `json:"value"`
	TargetStatus string       `json:"target_status"`
	Executions   int          `json:"executions"`
	Violations   int          `json:"violations"`
	Details      []string     `json:"details"`
	Evidence     []qgEvidence `json:"evidence"`
	Baseline     *struct {
		Status string   `json:"status"`
		Value  *float64 `json:"value"`
	} `json:"baseline"`
}

type qgEvaluation struct {
	Contract string    `json:"contract"`
	Gate     string    `json:"gate"`
	Checks   []qgCheck `json:"checks"`
}

func (c *checker) qgRules() map[string]any {
	return map[string]any{
		"version": 1,
		"entry":   map[string]any{"service": c.d.service("qg-api"), "operation": "start_session", "span_kind": "server"},
		"checks": []any{
			map[string]any{"id": "startup-p95", "type": "latency", "max_ms": 800, "target_ms": 500},
			map[string]any{"id": "db-calls", "type": "db_calls", "max_per_execution": 3, "target_per_execution": 2},
			map[string]any{"id": "no-directory", "type": "forbidden_call", "to": c.d.service("qg-directory")},
		},
	}
}

func (c *checker) qgEvaluate(ctx context.Context, run, baseline string) (qgEvaluation, error) {
	body := map[string]any{"rules": c.qgRules(), "run": run,
		"time_from": ns(c.d.m2Hour(6)), "time_to": ns(c.d.m2Hour(6).Add(30 * time.Minute))}
	if baseline != "" {
		body["baseline"] = baseline
	}
	var data struct {
		Evaluation qgEvaluation `json:"Evaluation"`
	}
	err := c.api.post(ctx, "/api/v1/quality/evaluate", body, &data)
	return data.Evaluation, err
}

func qgFind(ev qgEvaluation, id string) (qgCheck, error) {
	for _, ch := range ev.Checks {
		if ch.ID == id {
			return ch, nil
		}
	}
	return qgCheck{}, fmt.Errorf("no check %s in %+v", id, ev)
}

func val(v *float64) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprint(*v)
}

// want compares a check with literal expectations; baseline "" means none.
func qgWant(ev qgEvaluation, id, status, reason, value, target string, executions, violations int, baseline string) error {
	ch, err := qgFind(ev, id)
	if err != nil {
		return err
	}
	b := ""
	if ch.Baseline != nil {
		b = ch.Baseline.Status + " " + val(ch.Baseline.Value)
	}
	got := fmt.Sprintf("%s %s %s %s %d %d [%s]", ch.Status, ch.Reason, val(ch.Value), ch.TargetStatus, ch.Executions, ch.Violations, b)
	want := fmt.Sprintf("%s %s %s %s %d %d [%s]", status, reason, value, target, executions, violations, baseline)
	if got != want {
		return fmt.Errorf("%s: got %q, want %q", id, got, want)
	}
	return nil
}

// QG-REF-001: the candidate fails all three checks, with the baseline's
// values beside it and the execution behind each failure.
func (c *checker) qgCandidate(ctx context.Context) error {
	ev, err := c.qgEvaluate(ctx, c.d.qgRun("candidate"), c.d.qgRun("baseline"))
	if err != nil {
		return err
	}
	if ev.Gate != "failed" || ev.Contract != "quality-gate-v1" {
		return fmt.Errorf("gate %q contract %q", ev.Gate, ev.Contract)
	}
	for _, e := range []error{
		qgWant(ev, "startup-p95", "fail", "limit_exceeded", "910", "open", 200, 200, "pass 420"),
		qgWant(ev, "db-calls", "fail", "limit_exceeded", "21", "open", 200, 200, "pass 2"),
		qgWant(ev, "no-directory", "fail", "forbidden_call", "200", "", 200, 200, "pass 0"),
	} {
		if e != nil {
			return e
		}
	}
	db, _ := qgFind(ev, "db-calls")
	if got := strings.Join(db.Details, "|"); got != "target 2 per execution not met yet (advisory)|permission_lookup × 20|insert_session × 1" {
		return fmt.Errorf("db-calls details: %q", got)
	}
	// Evidence: the slowest execution is the first p95+100 one (execution
	// 191, trace sequence 191); the directory call is the CLIENT span.
	lat, _ := qgFind(ev, "startup-p95")
	dir, _ := qgFind(ev, "no-directory")
	slowTrace := c.d.traceID(scQG+1, 191)
	if len(lat.Evidence) != 1 || lat.Evidence[0].TraceID != slowTrace || lat.Evidence[0].SpanID != idRoot {
		return fmt.Errorf("latency evidence %+v, want trace %s", lat.Evidence, slowTrace)
	}
	if len(dir.Evidence) != 1 || dir.Evidence[0].TraceID != c.d.traceID(scQG+1, 1) || dir.Evidence[0].SpanID != qgClientID {
		return fmt.Errorf("directory evidence %+v", dir.Evidence)
	}
	// The evidence window opens the execution: its report has the trace.
	e := lat.Evidence[0]
	rep, err := c.api.report(ctx, reportRequest{TraceID: e.TraceID, TimeFrom: e.TimeFrom, TimeTo: e.TimeTo})
	if err != nil {
		return fmt.Errorf("evidence report: %w", err)
	}
	if rep.Spans == nil || rep.Spans.Status != "ok" || len(rep.Spans.Records) != 25 {
		n := -1
		if rep.Spans != nil {
			n = len(rep.Spans.Records)
		}
		return fmt.Errorf("evidence report: %d spans, want 25 (root, check_permissions, 20 lookups, insert, client, directory)", n)
	}
	return nil
}

// QG-REF-002: the fix passes; its latency target stays open and it is still
// slower than the baseline (a pass is not "every number improved").
func (c *checker) qgFixed(ctx context.Context) error {
	ev, err := c.qgEvaluate(ctx, c.d.qgRun("fixed"), c.d.qgRun("baseline"))
	if err != nil {
		return err
	}
	if ev.Gate != "passed" {
		return fmt.Errorf("gate %q: %+v", ev.Gate, ev.Checks)
	}
	for _, e := range []error{
		qgWant(ev, "startup-p95", "pass", "within_limit", "560", "open", 200, 0, "pass 420"),
		qgWant(ev, "db-calls", "pass", "within_limit", "2", "met", 200, 0, "pass 2"),
		qgWant(ev, "no-directory", "pass", "not_observed", "0", "", 200, 0, "pass 0"),
	} {
		if e != nil {
			return e
		}
	}
	return nil
}

// QG-EVD-001: missing evidence never passes: 19 executions are too few for a
// p95, and an unknown run has no executions at all.
func (c *checker) qgInsufficient(ctx context.Context) error {
	ev, err := c.qgEvaluate(ctx, c.d.qgRun("few"), "")
	if err != nil {
		return err
	}
	if ev.Gate != "unresolved" {
		return fmt.Errorf("19 executions: gate %q", ev.Gate)
	}
	for _, e := range []error{
		qgWant(ev, "startup-p95", "insufficient", "too_few_executions", "518", "", 19, 0, ""),
		qgWant(ev, "db-calls", "pass", "within_limit", "2", "met", 19, 0, ""),
		qgWant(ev, "no-directory", "pass", "not_observed", "0", "", 19, 0, ""),
	} {
		if e != nil {
			return e
		}
	}
	ev, err = c.qgEvaluate(ctx, c.d.qgRun("nobody"), "")
	if err != nil {
		return err
	}
	if ev.Gate != "unresolved" {
		return fmt.Errorf("unknown run: gate %q", ev.Gate)
	}
	for _, ch := range ev.Checks {
		if ch.Status != "insufficient" || ch.Reason != "no_executions" {
			return fmt.Errorf("unknown run: %s %s %s", ch.ID, ch.Status, ch.Reason)
		}
	}
	return nil
}

// QG-VAL-001: invalid rules are a 400 before any query.
func (c *checker) qgInvalid(ctx context.Context) error {
	body := map[string]any{"rules": map[string]any{"version": 1, "checks": []any{
		map[string]any{"id": "p95", "type": "latency", "entry": map[string]any{"service": "s", "operation": "o"}, "max_ms": 800, "target_ms": 900},
	}}, "run": "r", "time_from": "1", "time_to": "2"}
	var data any
	err := c.api.post(ctx, "/api/v1/quality/evaluate", body, &data)
	var ae *apiError
	if !errors.As(err, &ae) || ae.Status != 400 || !strings.Contains(ae.Message, "target_ms") {
		return fmt.Errorf("want HTTP 400 naming target_ms, got %v", err)
	}
	return nil
}

// QG-WIN-001: without a window the API takes each run's own, from its first
// to its last entry span: the candidate gives the same answer as with its
// exact window, and both windows lie inside the runs' half hour.
func (c *checker) qgWindowFromRun(ctx context.Context) error {
	body := map[string]any{"rules": c.qgRules(), "run": c.d.qgRun("candidate"), "baseline": c.d.qgRun("baseline")}
	var data struct {
		Evaluation struct {
			qgEvaluation
			TimeFrom        string `json:"time_from"`
			TimeTo          string `json:"time_to"`
			BaselineFrom    string `json:"baseline_time_from"`
			BaselineTo      string `json:"baseline_time_to"`
			WindowFromRun   bool   `json:"window_from_run"`
			BaselineFromRun bool   `json:"baseline_window_from_run"`
		} `json:"Evaluation"`
	}
	if err := c.api.post(ctx, "/api/v1/quality/evaluate", body, &data); err != nil {
		return err
	}
	ev := data.Evaluation
	if !ev.WindowFromRun || !ev.BaselineFromRun || ev.Gate != "failed" {
		return fmt.Errorf("from run %v, baseline from run %v, gate %s", ev.WindowFromRun, ev.BaselineFromRun, ev.Gate)
	}
	lo, hi := c.d.m2Hour(6).UnixNano(), c.d.m2Hour(6).Add(30*time.Minute).UnixNano()
	for _, w := range [][2]string{{ev.TimeFrom, ev.TimeTo}, {ev.BaselineFrom, ev.BaselineTo}} {
		from, err1 := strconv.ParseInt(w[0], 10, 64)
		to, err2 := strconv.ParseInt(w[1], 10, 64)
		if err1 != nil || err2 != nil || from < lo || to > hi || to <= from {
			return fmt.Errorf("window %s..%s is not inside the runs' half hour", w[0], w[1])
		}
	}
	for _, e := range []error{
		qgWant(ev.qgEvaluation, "startup-p95", "fail", "limit_exceeded", "910", "open", 200, 200, "pass 420"),
		qgWant(ev.qgEvaluation, "db-calls", "fail", "limit_exceeded", "21", "open", 200, 200, "pass 2"),
		qgWant(ev.qgEvaluation, "no-directory", "fail", "forbidden_call", "200", "", 200, 200, "pass 0"),
	} {
		if e != nil {
			return e
		}
	}
	return nil
}

func (c *checker) qgChecks() []namedCheck {
	return []namedCheck{
		{"QG-REF-001", "Quality gate: candidate fails latency, database calls and the forbidden call", c.qgCandidate},
		{"QG-REF-002", "Quality gate: the fix passes, target still open", c.qgFixed},
		{"QG-WIN-001", "Quality gate: without a window, each run's own is used", c.qgWindowFromRun},
		{"QG-EVD-001", "Quality gate: too few or no executions is unresolved", c.qgInsufficient},
		{"QG-VAL-001", "Quality gate: invalid rules are rejected", c.qgInvalid},
		{"QG-HIST-001", "Quality gate: requirements and their recorded history", c.qgHistory},
		{"QG-HIST-005", "Quality gate: concurrent records of one evaluation are one record", c.qgConcurrentRecord},
	}
}
