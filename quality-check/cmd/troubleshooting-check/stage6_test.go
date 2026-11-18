package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A retried record is answered with the record; --reevaluate asks for a new
// one; the assumptions reach every report.
func TestRecordedBeforeAndAssumptions(t *testing.T) {
	var got request
	answerWith := func(before bool) string {
		var env struct {
			Data struct {
				Evaluation Evaluation `json:"Evaluation"`
			} `json:"data"`
		}
		_ = json.Unmarshal([]byte(recordedAnswer(t, "")), &env)
		ev := env.Data.Evaluation
		ev.RecordedBefore = before
		ev.Source = &Source{Commit: "first-commit", Ref: "main"}
		ev.Assumptions = []string{"Verdicts hold for the recorded spans.", "Child spans lost without a trace can't be detected."}
		b, _ := json.Marshal(map[string]any{"data": map[string]any{"Evaluation": ev}})
		return string(b)
	}
	before := true
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/trace-report" {
			io.WriteString(w, snapshotAnswer)
			return
		}
		b, _ := io.ReadAll(r.Body)
		got = request{}
		json.Unmarshal(b, &got)
		io.WriteString(w, answerWith(before))
	}))
	defer s.Close()
	rules := writeFile(t, "quality.yaml", requirementsRules)
	dir := t.TempDir()
	var out bytes.Buffer
	run([]string{"--url", s.URL, "--ui", "none", "--rules", rules, "--run", "pr-42", "--record", "--commit", "second-commit",
		"--markdown", filepath.Join(dir, "s.md"), "--html", filepath.Join(dir, "r.html")}, &out, io.Discard, fixedNow)
	text := out.String()
	if !strings.Contains(text, "Already recorded as evaluation 0123456789abcdef01234567 (2026-11-17T12:00:00Z): this is that record, not a new evaluation; --reevaluate records a new one") {
		t.Errorf("retry notice missing:\n%s", text)
	}
	// The record names what it recorded, not what this call sent.
	if !strings.Contains(text, "commit first-commit (main)") || got.Reevaluate {
		t.Errorf("source or reevaluate: %v\n%s", got.Reevaluate, text)
	}
	if !strings.Contains(text, "Note: Verdicts hold for the recorded spans.\nNote: Child spans lost without a trace can't be detected.") {
		t.Errorf("assumptions in the text report:\n%s", text)
	}
	md, _ := os.ReadFile(filepath.Join(dir, "s.md"))
	h, _ := os.ReadFile(filepath.Join(dir, "r.html"))
	if !strings.Contains(string(md), "Verdicts hold for the recorded spans.") || !strings.Contains(string(h), "<li>Child spans lost without a trace can&#39;t be detected.</li>") {
		t.Errorf("assumptions in markdown/html:\n%s\n%s", md, h)
	}

	before = false
	out.Reset()
	run([]string{"--url", s.URL, "--ui", "none", "--rules", rules, "--run", "pr-42", "--record", "--reevaluate"}, &out, io.Discard, fixedNow)
	if !got.Reevaluate || !got.Record || !strings.Contains(out.String(), "Recorded as evaluation") {
		t.Errorf("reevaluate: %+v\n%s", got, out.String())
	}
	// --reevaluate without --record records nothing.
	run([]string{"--url", s.URL, "--ui", "none", "--rules", rules, "--run", "pr-42", "--reevaluate"}, io.Discard, io.Discard, fixedNow)
	if got.Reevaluate || got.Record {
		t.Errorf("reevaluate without record: %+v", got)
	}
}

// A trace with 220 spans before the counterexample: the snapshot keeps the
// evidence span, its ancestors and what is below it, and a db_calls
// evidence its database calls.
func TestSnapshotKeepsCounterexample(t *testing.T) {
	var recs []string
	rec := func(id, parent, name, kind, start string, tags string) {
		recs = append(recs, fmt.Sprintf(`{"timeStamp":%q,"name":%q,"spanKind":%q,"service":"svc","spanId":%q,"parentSpanId":%q,"status":"unset","duration":1,"tags":[%s]}`,
			start, name, kind, id, parent, tags))
	}
	rec("0000000000000001", "0000000000000000", "entry", "server", "1000", "")
	for i := 0; i < 220; i++ {
		rec(fmt.Sprintf("%016x", 0x1000+i), "0000000000000001", "prepare", "internal", fmt.Sprint(1001+i), "")
	}
	rec("00000000000000c1", "0000000000000001", "GET /users", "client", "5000", `{"key":"peer.service","value":"directory-api"}`)
	rec("00000000000000c2", "00000000000000c1", "GET /users/{id}", "server", "5001", "")
	for i := 0; i < 5; i++ {
		rec(fmt.Sprintf("%016x", 0xd0+i), "0000000000000001", "lookup", "client", fmt.Sprint(6000+i), `{"key":"db.system","value":"postgresql"}`)
	}
	body := `{"data":{"Report":{"spans":{"status":"ok","truncated":false,"records":[` + strings.Join(recs, ",") + `]}}}}`
	var limit float64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		limit, _ = req["span_limit"].(float64)
		io.WriteString(w, body)
	}))
	defer s.Close()
	tr := readTrace(options{url: s.URL, timeout: 5e9}, "t", "1", "2")
	if limit != readSpanLimit || len(tr.spans) != 228 {
		t.Fatalf("read %d spans with limit %v", len(tr.spans), limit)
	}
	names := func(snap *Snapshot) map[string]int {
		m := map[string]int{}
		for _, sp := range snap.Spans {
			m[sp.Name]++
		}
		return m
	}
	call := snapshotFor(tr, "00000000000000c1", false)
	n := names(call)
	if len(call.Spans) != maxSnapshotSpans || !call.Truncated || n["entry"] != 1 || n["GET /users"] != 1 || n["GET /users/{id}"] != 1 {
		t.Errorf("forbidden-call snapshot: %d spans, %v", len(call.Spans), n)
	}
	if call.Spans[0].Name != "entry" {
		t.Errorf("not in start order: first %s", call.Spans[0].Name)
	}
	db := snapshotFor(tr, "0000000000000001", true)
	if names(db)["lookup"] != 5 {
		t.Errorf("db-calls snapshot: %v", names(db))
	}
	if got, cut := snapshotSelection([]string{"a"}, []string{"0"}, []string{"a"}, 200); len(got) != 1 || cut {
		t.Errorf("small trace: %v %v", got, cut)
	}
}
