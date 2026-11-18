package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Without --from, --to or --since the API takes the run's own window, so a
// retried CI step evaluates (and records) the same window; --since still
// works, and --baseline-* still sets the baseline's window.
func TestWindowFromRun(t *testing.T) {
	rules := writeFile(t, "quality.yaml", rulesYAML)
	var got request
	s := server(t, 200, answer("passed"), &got)
	run([]string{"--url", s.URL, "--rules", rules, "--run", "pr-42", "--baseline", "main"}, io.Discard, io.Discard, fixedNow)
	if got.TimeFrom != "" || got.TimeTo != "" || got.BaselineFrom != "" || got.BaselineTo != "" {
		t.Errorf("no window flags: %+v", got)
	}
	got = request{}
	run([]string{"--url", s.URL, "--rules", rules, "--run", "pr-42", "--since", "2h"}, io.Discard, io.Discard, fixedNow)
	if got.TimeFrom != itoa(fixedNow().Add(-2*time.Hour).UnixNano()) || got.TimeTo != itoa(fixedNow().UnixNano()) {
		t.Errorf("--since: %+v", got)
	}
	got = request{}
	run([]string{"--url", s.URL, "--rules", rules, "--run", "pr-42", "--baseline", "main", "--baseline-since", "24h"}, io.Discard, io.Discard, fixedNow)
	if got.TimeFrom != "" || got.BaselineFrom != itoa(fixedNow().Add(-24*time.Hour).UnixNano()) {
		t.Errorf("baseline window only: %+v", got)
	}
}

// A Troubleshooting from before windows from the run answers 400 to a
// request without one: the CLI says so and checks the last --since instead.
func TestWindowFromRunOldAPI(t *testing.T) {
	rules := writeFile(t, "quality.yaml", rulesYAML)
	var bodies []request
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)
		bodies = append(bodies, req)
		if req.TimeFrom == "" {
			w.WriteHeader(400)
			io.WriteString(w, `{"error":true,"message":"time_from must be Unix nanoseconds"}`)
			return
		}
		io.WriteString(w, answer("passed"))
	}))
	defer s.Close()
	var errOut bytes.Buffer
	if code := run([]string{"--url", s.URL, "--rules", rules, "--run", "pr-42"}, io.Discard, &errOut, fixedNow); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if len(bodies) != 2 || bodies[1].TimeFrom != itoa(fixedNow().Add(-time.Hour).UnixNano()) {
		t.Errorf("requests %+v", bodies)
	}
	if !strings.Contains(errOut.String(), "this Troubleshooting needs a window; checking executions of the last 1h0m0s") {
		t.Errorf("stderr %q", errOut.String())
	}
}

// The HTML report says when a window is the run's own.
func TestWindowFromRunReport(t *testing.T) {
	ev := Evaluation{Contract: "quality-gate-v1", Run: "pr-42", Gate: "passed", TimeFrom: "1794859200000000000", TimeTo: "1794859260000000001", WindowFromRun: true}
	if h := htmlReport(ev); !strings.Contains(h, "UTC (the run's window)") {
		t.Errorf("html: %s", h[:min(len(h), 600)])
	}
}
