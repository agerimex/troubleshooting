package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const requirementsRules = `version: 1
project: recording
entry: {service: recording-api, operation: start_session}
checks:
  - {id: db-calls, type: db_calls, max_per_execution: 3}
  - {id: no-directory, type: forbidden_call, to: directory-api}
requirements:
  - id: REQ-START
    title: A session starts with few database calls
    owner: recording-team
    why: Every call adds latency.
    checks: [db-calls]
    guardrails: [no-directory]
  - id: REQ-OUTAGE
    title: Recording survives an analytics outage
`

// recordedAnswer is answer("failed") as the API gives it with record: an id,
// requirement statuses and the db-calls evidence's snapshot.
func recordedAnswer(t *testing.T, recordErr string) string {
	t.Helper()
	var env struct {
		Data struct {
			Evaluation Evaluation `json:"Evaluation"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(answer("failed")), &env); err != nil {
		t.Fatal(err)
	}
	ev := env.Data.Evaluation
	ev.Project = "recording"
	ev.Requirements = []RequirementResult{
		{ID: "REQ-START", Title: "A session starts with few database calls", Owner: "recording-team", Why: "Every call adds latency.",
			Checks: []string{"db-calls"}, Status: "violated", Summary: "failed: db-calls",
			Guardrails: []string{"no-directory"}, GuardrailSummary: "guardrail failed: no-directory (guardrails decide the gate, not this requirement)"},
		{ID: "REQ-OUTAGE", Title: "Recording survives an analytics outage", Checks: []string{}, Status: "not_evaluated", Summary: "no check enforces it yet"},
	}
	if recordErr != "" {
		ev.RecordError = recordErr
	} else {
		ev.ID, ev.RecordedAt = "0123456789abcdef01234567", "2026-11-17T12:00:00Z"
	}
	ev.Checks[0].Evidence[0].Snapshot = &Snapshot{Spans: []SnapSpan{{SpanID: "0000000000000001", ParentID: "0000000000000000",
		Service: "recording-api", Name: "start_session", Kind: "server", Start: "1000000000", Duration: 2000, Status: "unset"}}}
	b, _ := json.Marshal(map[string]any{"error": false, "message": "success", "data": map[string]any{"Evaluation": ev}})
	return string(b)
}

// --record asks the API to keep the evaluation; requirements are shown in
// every report; evidence the API already saved is not read again.
func TestRequirementsAndRecord(t *testing.T) {
	var got request
	var reports int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/trace-report" {
			atomic.AddInt32(&reports, 1)
			io.WriteString(w, snapshotAnswer)
			return
		}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &got)
		io.WriteString(w, recordedAnswer(t, ""))
	}))
	defer s.Close()
	rules := writeFile(t, "quality.yaml", requirementsRules)
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	code := run([]string{"--url", s.URL, "--ui", "http://ui.example", "--rules", rules, "--run", "pr-42", "--record", "--commit", "abc", "--ref", "main",
		"--json", filepath.Join(dir, "r.json"), "--markdown", filepath.Join(dir, "s.md"), "--html", filepath.Join(dir, "r.html")}, &out, &errOut, fixedNow)
	if code != 1 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !got.Record || got.RulesYAML != requirementsRules || got.Source == nil || got.Source.Commit != "abc" || got.Source.Ref != "main" {
		t.Errorf("request: record %v, source %+v", got.Record, got.Source)
	}
	// Only the evidence without the API's snapshot is read.
	if n := atomic.LoadInt32(&reports); n != 1 {
		t.Errorf("%d trace reports read, want 1", n)
	}
	text := out.String()
	for _, want := range []string{
		"Requirements: 1 violated, 1 not evaluated",
		"VIOLATED       REQ-START  A session starts with few database calls (owner recording-team)\n               failed: db-calls\n               guardrail failed: no-directory (guardrails decide the gate, not this requirement)",
		"NOT EVALUATED  REQ-OUTAGE  Recording survives an analytics outage\n               no check enforces it yet",
		"Recorded as evaluation 0123456789abcdef01234567: http://ui.example/#/quality?evaluation=0123456789abcdef01234567",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text report is missing %q:\n%s", want, text)
		}
	}
	md, _ := os.ReadFile(filepath.Join(dir, "s.md"))
	for _, want := range []string{
		"**Requirements:** 1 violated, 1 not evaluated",
		"| ❌ | **REQ-START** A session starts with few database calls | recording-team | violated: failed: db-calls; guardrail failed: no-directory (guardrails decide the gate, not this requirement) |",
		"| ➖ | **REQ-OUTAGE** Recording survives an analytics outage |  | not evaluated: no check enforces it yet |",
	} {
		if !strings.Contains(string(md), want) {
			t.Errorf("markdown is missing %q:\n%s", want, md)
		}
	}
	h, _ := os.ReadFile(filepath.Join(dir, "r.html"))
	for _, want := range []string{"<h2>Requirements: 1 violated, 1 not evaluated</h2>", `<td class="fail">violated</td>`, "Every call adds latency.", "<h2>Checks</h2>", "Guardrails: no-directory"} {
		if !strings.Contains(string(h), want) {
			t.Errorf("html is missing %q", want)
		}
	}
	var saved Evaluation
	b, _ := os.ReadFile(filepath.Join(dir, "r.json"))
	if err := json.Unmarshal(b, &saved); err != nil || saved.ID == "" || len(saved.Requirements) != 2 || saved.Checks[0].Evidence[0].Snapshot.Spans[0].Name != "start_session" {
		t.Errorf("json: %v %+v", err, saved)
	}
}

// A failed record is a warning: the gate still decides the exit code.
func TestRecordErrorWarns(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, recordedAnswer(t, "the evaluation was not recorded: table missing"))
	}))
	defer s.Close()
	rules := writeFile(t, "quality.yaml", requirementsRules)
	var out, errOut bytes.Buffer
	if code := run([]string{"--url", s.URL, "--rules", rules, "--run", "pr-42", "--record", "--ui", "none"}, &out, &errOut, fixedNow); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(errOut.String(), "warning: the evaluation was not recorded: table missing") || strings.Contains(out.String(), "Recorded as") {
		t.Errorf("stderr %q\nstdout %s", errOut.String(), out.String())
	}
}

func TestRecordLink(t *testing.T) {
	for ui, want := range map[string]string{
		"none":                        "",
		"http://h:8095/":              ": http://h:8095/#/quality?evaluation=x",
		"http://h:8095/#/executions?": ": http://h:8095/#/quality?evaluation=x",
	} {
		if got := recordLink(ui, "x"); got != want {
			t.Errorf("%q: %q, want %q", ui, got, want)
		}
	}
}
