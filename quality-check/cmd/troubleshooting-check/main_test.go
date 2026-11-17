package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const rulesYAML = `version: 1
entry:
  service: recording-api
  operation: start_session
  span_kind: server
checks:
  - id: startup-p95
    type: latency
    max_ms: 800
    target_ms: 500
  - id: db-calls
    type: db_calls
    max_per_execution: 3
  - id: no-directory
    type: forbidden_call
    to: directory-api
`

func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

var fixedNow = func() time.Time { return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC) }

func server(t *testing.T, status int, answer string, got *request) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/trace-report" && r.Method == http.MethodPost {
			io.WriteString(w, snapshotAnswer)
			return
		}
		if r.URL.Path != "/api/v1/quality/evaluate" || r.Method != http.MethodPost {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		if got != nil {
			if err := json.Unmarshal(b, got); err != nil {
				t.Errorf("body: %v", err)
			}
		}
		w.WriteHeader(status)
		io.WriteString(w, answer)
	}))
	t.Cleanup(s.Close)
	return s
}

func answer(gate string) string {
	ev := Evaluation{Contract: "quality-gate-v1", Run: "pr-42", Baseline: "main", Gate: gate, Checks: []CheckResult{{
		ID: "db-calls", Type: "db_calls", Entry: Entry{Service: "recording-api", Operation: "start_session"},
		Status: "fail", Summary: "up to 21 database calls per execution, limit 3, 200 executions",
		Unit: "calls per execution", Details: []string{"permission_lookup × 20"},
		Baseline: &Baseline{Status: "pass", Value: func() *float64 { v := 2.0; return &v }(), Executions: 200, Summary: "up to 2"},
		Evidence: []Evidence{{TraceID: "abc", SpanID: "0000000000000001", TimeFrom: "1", TimeTo: "2", Note: "execution with the most database calls: 21"}},
	}, {
		ID: "no-directory", Type: "forbidden_call", Status: "fail", Summary: "call",
		Evidence: []Evidence{{TraceID: "abc", SpanID: "0000000000000002", TimeFrom: "1", TimeTo: "2", Note: "recording-api → directory-api"}},
	}}}
	b, _ := json.Marshal(map[string]any{"error": false, "message": "success", "data": map[string]any{"Evaluation": ev}})
	return string(b)
}

func TestExitCodesFollowTheGate(t *testing.T) {
	rules := writeFile(t, "quality.yaml", rulesYAML)
	for gate, want := range map[string]int{"passed": 0, "failed": 1, "unresolved": 2, "bogus": 3} {
		s := server(t, 200, answer(gate), nil)
		var out, errOut bytes.Buffer
		if code := run([]string{"--url", s.URL, "--rules", rules, "--run", "pr-42"}, &out, &errOut, fixedNow); code != want {
			t.Errorf("gate %s: exit %d, want %d (%s)", gate, code, want, errOut.String())
		}
	}
}

func TestRequestReportAndJSON(t *testing.T) {
	rules := writeFile(t, "quality.yaml", rulesYAML)
	var got request
	s := server(t, 200, answer("failed"), &got)
	jsonPath := filepath.Join(t.TempDir(), "result.json")
	var out, errOut bytes.Buffer
	code := run([]string{"--url", s.URL + "/", "--ui", "http://ui.example:8095/#/old", "--rules", rules, "--run", "pr-42",
		"--baseline", "main", "--since", "30m", "--json", jsonPath}, &out, &errOut, fixedNow)
	if code != 1 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	now := fixedNow()
	if got.Run != "pr-42" || got.Baseline != "main" || got.TimeTo != itoa(now.UnixNano()) || got.TimeFrom != itoa(now.Add(-30*time.Minute).UnixNano()) {
		t.Errorf("request: %+v", got)
	}
	if got.BaselineFrom != "" || len(got.Rules.Checks) != 3 || *got.Rules.Checks[0].MaxMs != 800 || got.Rules.Entry.SpanKind != "server" {
		t.Errorf("rules: %+v", got.Rules)
	}
	text := out.String()
	for _, want := range []string{
		"Quality gate: FAILED\nrun pr-42, baseline main, 2 checks",
		"FAIL    db-calls  recording-api / start_session",
		"baseline: 2 (200 executions)",
		"permission_lookup × 20",
		"http://ui.example:8095/#/executions?from=1&span=0000000000000001&to=2&trace=abc&view=timeline",
		"view=sequence",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report is missing %q:\n%s", want, text)
		}
	}
	b, err := os.ReadFile(jsonPath)
	if err != nil || !strings.Contains(string(b), `"link": "http://ui.example:8095/#/executions?`) {
		t.Errorf("json file: %v %s", err, b)
	}
}

func TestBaselineWindow(t *testing.T) {
	rules := writeFile(t, "quality.yaml", rulesYAML)
	var got request
	s := server(t, 200, answer("passed"), &got)
	run([]string{"--url", s.URL, "--rules", rules, "--run", "pr-42", "--baseline", "main", "--baseline-since", "24h"},
		io.Discard, io.Discard, fixedNow)
	if got.BaselineFrom != itoa(fixedNow().Add(-24*time.Hour).UnixNano()) || got.BaselineTo != itoa(fixedNow().UnixNano()) {
		t.Errorf("baseline window: %+v", got)
	}
}

// Errors never look like a result: exit 3, nothing on stdout.
func TestErrorsExitThree(t *testing.T) {
	rules := writeFile(t, "quality.yaml", rulesYAML)
	typo := writeFile(t, "typo.yaml", strings.Replace(rulesYAML, "target_ms", "targt_ms", 1))
	dup := writeFile(t, "dup.yaml", strings.Replace(rulesYAML, "    max_ms: 800\n", "    max_ms: 800\n    max_ms: 900\n", 1))
	bad := server(t, 400, `{"error":true,"message":"check startup-p95: max_ms must be a positive number"}`, nil)
	html := server(t, 502, `<html>bad gateway</html>`, nil)
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	for name, args := range map[string][]string{
		"no run":        {"--url", bad.URL, "--rules", rules},
		"missing rules": {"--url", bad.URL, "--rules", "nope.yaml", "--run", "r"},
		"unknown key":   {"--url", bad.URL, "--rules", typo, "--run", "r"},
		"duplicate key": {"--url", bad.URL, "--rules", dup, "--run", "r"},
		"invalid rules": {"--url", bad.URL, "--rules", rules, "--run", "r"},
		"not the API":   {"--url", html.URL, "--rules", rules, "--run", "r"},
		"no server":     {"--url", closed.URL, "--rules", rules, "--run", "r"},
		"bad window":    {"--url", bad.URL, "--rules", rules, "--run", "r", "--from", "2026-11-16T13:00:00Z"},
		"unknown flag":  {"--nope"},
	} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut, fixedNow); code != 3 || out.Len() != 0 {
			t.Errorf("%s: exit %d, stdout %q", name, code, out.String())
		}
		if name == "invalid rules" && !strings.Contains(errOut.String(), "max_ms must be a positive number") {
			t.Errorf("the API's message is not shown: %s", errOut.String())
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// The shipped example must stay valid for the strict parser.
func TestExampleRulesParse(t *testing.T) {
	r, _, err := loadRules(filepath.Join("..", "..", "quality.example.yaml"))
	if err != nil || r.Version != 1 || len(r.Checks) != 3 || r.Entry == nil || r.Checks[1].MaxPerExecution == nil {
		t.Fatalf("example: %v %+v", err, r)
	}
}

// The result names what was tested and which rules decided: commit, ref, CI
// run, the rules file and the sha256 of its bytes.
func TestSourceRecorded(t *testing.T) {
	rules := writeFile(t, "quality.yaml", rulesYAML)
	s := server(t, 200, answer("failed"), nil)
	dir := t.TempDir()
	var out bytes.Buffer
	code := run([]string{"--url", s.URL, "--rules", rules, "--run", "pr-42",
		"--commit", "0123456789abcdef0123456789abcdef01234567", "--ref", "feat/x", "--ci-run", "https://ci.example/runs/7",
		"--rules-ref", "origin/main:quality.yaml",
		"--json", filepath.Join(dir, "r.json"), "--markdown", filepath.Join(dir, "s.md"), "--html", filepath.Join(dir, "r.html")}, &out, io.Discard, fixedNow)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	// sha256 of rulesYAML's bytes.
	sum := sha256.Sum256([]byte(rulesYAML))
	digest := hex.EncodeToString(sum[:])
	var ev Evaluation
	b, _ := os.ReadFile(filepath.Join(dir, "r.json"))
	if err := json.Unmarshal(b, &ev); err != nil || ev.Source == nil {
		t.Fatalf("json: %v %s", err, b)
	}
	want := Source{Commit: "0123456789abcdef0123456789abcdef01234567", Ref: "feat/x", CIRun: "https://ci.example/runs/7",
		Rules: rules, RulesRef: "origin/main:quality.yaml", RulesSHA256: digest}
	if *ev.Source != want {
		t.Errorf("source: %+v", *ev.Source)
	}
	line := "commit 0123456789ab (feat/x) · rules origin/main:quality.yaml, sha256 " + digest[:12]
	if !strings.Contains(out.String(), "2 checks\n"+line+"\n") {
		t.Errorf("text report:\n%s", out.String())
	}
	md, _ := os.ReadFile(filepath.Join(dir, "s.md"))
	if !strings.Contains(string(md), "commit 0123456789ab (feat/x)") || !strings.Contains(string(md), "[CI run](https://ci.example/runs/7)") {
		t.Errorf("markdown:\n%s", md)
	}
	h, _ := os.ReadFile(filepath.Join(dir, "r.html"))
	if !strings.Contains(string(h), `<a href="https://ci.example/runs/7">CI run</a>`) || !strings.Contains(string(h), "sha256 "+digest[:12]) {
		t.Errorf("html:\n%s", h)
	}
	// Without flags the rules are still identified.
	out.Reset()
	run([]string{"--url", s.URL, "--rules", rules, "--run", "pr-42"}, &out, io.Discard, fixedNow)
	if !strings.Contains(out.String(), "rules "+rules+", sha256 "+digest[:12]) || strings.Contains(out.String(), "commit") {
		t.Errorf("without flags:\n%s", out.String())
	}
}

// The evidence trace: an entry, a database call and a span with markup in
// its name (recorded text is never markup).
const snapshotAnswer = `{"error":false,"message":"success","data":{"Report":{"spans":{"status":"ok","truncated":false,"records":[
{"timeStamp":"1000000000","name":"start_session","spanKind":"server","service":"recording-api","spanId":"0000000000000001","parentSpanId":"0000000000000000","status":"unset","duration":2000,"tags":[]},
{"timeStamp":"1001000000","name":"<script>alert(1)</script>","spanKind":"client","service":"recording-api","spanId":"0000000000000003","parentSpanId":"0000000000000002","status":"error","duration":-1,"tags":[]},
{"timeStamp":"1000500000","name":"permission_lookup","spanKind":"client","service":"recording-api","spanId":"0000000000000002","parentSpanId":"0000000000000001","status":"unset","duration":500,"tags":[{"key":"db.system","value":"postgresql"}]}
]}}}}`

// Each evidence keeps its trace's spans in result.json and report.html, as
// a tree; a failed read is noted and changes nothing; --no-snapshot skips it.
func TestEvidenceSnapshot(t *testing.T) {
	rules := writeFile(t, "quality.yaml", rulesYAML)
	s := server(t, 200, answer("failed"), nil)
	dir := t.TempDir()
	code := run([]string{"--url", s.URL, "--rules", rules, "--run", "pr-42",
		"--json", filepath.Join(dir, "r.json"), "--html", filepath.Join(dir, "r.html")}, io.Discard, io.Discard, fixedNow)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	var ev Evaluation
	b, _ := os.ReadFile(filepath.Join(dir, "r.json"))
	if err := json.Unmarshal(b, &ev); err != nil {
		t.Fatal(err)
	}
	snap := ev.Checks[0].Evidence[0].Snapshot
	if snap == nil || len(snap.Spans) != 3 || snap.Spans[2].Peer != "postgresql" || snap.Error != "" {
		t.Fatalf("snapshot: %+v", snap)
	}
	rows, depth := snapshotRows(snap.Spans)
	if rows[0].SpanID != "0000000000000001" || rows[1].SpanID != "0000000000000002" || rows[2].SpanID != "0000000000000003" ||
		depth[0] != 0 || depth[1] != 1 || depth[2] != 2 {
		t.Errorf("tree: %+v %v", rows, depth)
	}
	h, _ := os.ReadFile(filepath.Join(dir, "r.html"))
	for _, want := range []string{"Snapshot of the execution: 3 spans", "&lt;script&gt;alert(1)&lt;/script&gt;", `<tr class="hl">`,
		"→ postgresql", "+0.5 ms", "invalid", `<span class="fail">error</span>`} {
		if !strings.Contains(string(h), want) {
			t.Errorf("html is missing %q", want)
		}
	}
	if strings.Contains(strings.ToLower(string(h)), "<script") {
		t.Error("html contains a script tag")
	}

	// A failed read: noted, same gate.
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/trace-report" {
			w.WriteHeader(http.StatusGatewayTimeout)
			io.WriteString(w, `{"error":true,"message":"query limit"}`)
			return
		}
		io.WriteString(w, answer("failed"))
	}))
	defer broken.Close()
	if code := run([]string{"--url", broken.URL, "--rules", rules, "--run", "pr-42", "--html", filepath.Join(dir, "b.html")}, io.Discard, io.Discard, fixedNow); code != 1 {
		t.Errorf("exit %d with a failed snapshot", code)
	}
	h, _ = os.ReadFile(filepath.Join(dir, "b.html"))
	if !strings.Contains(string(h), "Snapshot of the execution unavailable: HTTP 504 query limit") {
		t.Errorf("failed snapshot not noted")
	}
	run([]string{"--url", s.URL, "--rules", rules, "--run", "pr-42", "--no-snapshot", "--json", filepath.Join(dir, "n.json")}, io.Discard, io.Discard, fixedNow)
	b, _ = os.ReadFile(filepath.Join(dir, "n.json"))
	if strings.Contains(string(b), "snapshot") {
		t.Errorf("--no-snapshot kept a snapshot")
	}
}
