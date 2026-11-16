package main

import (
	"bytes"
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
	r, err := loadRules(filepath.Join("..", "..", "quality.example.yaml"))
	if err != nil || r.Version != 1 || len(r.Checks) != 3 || r.Entry == nil || r.Checks[1].MaxPerExecution == nil {
		t.Fatalf("example: %v %+v", err, r)
	}
}
