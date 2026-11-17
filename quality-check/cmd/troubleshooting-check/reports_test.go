package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hostileEvaluation() Evaluation {
	v, l := 21.0, 3.0
	return Evaluation{Contract: "quality-gate-v1", Method: "m", Run: "pr`42", Baseline: "main", Gate: "failed", EvaluatedAt: "2026-11-17T00:00:00Z",
		Checks: []CheckResult{{
			ID: "db", Type: "db_calls", Status: "fail", Unit: "calls per execution", Value: &v, Limit: &l, Executions: 200,
			Entry:   Entry{Service: "api", Operation: `<script>alert(1)</script>|x`},
			Summary: "up to 21 | calls", Details: []string{"permission_lookup × 20", `<img src=x onerror=alert(1)>`},
			Baseline: &Baseline{Status: "pass", Value: func() *float64 { x := 2.0; return &x }(), Executions: 200},
			Evidence: []Evidence{
				{TraceID: "abc", Note: "worst <b>", Link: "http://ui:8095/#/executions?trace=abc&view=timeline"},
				{TraceID: "def", Note: "bad link", Link: "javascript:alert(1)"},
			},
		}, {ID: "p95", Type: "latency", Status: "insufficient", Unit: "ms", Summary: "19 executions", Advisory: true}},
	}
}

// Recorded names never become markup: no script, no tag, no table break.
func TestReportsEscapeRecordedText(t *testing.T) {
	ev := hostileEvaluation()
	h := htmlReport(ev)
	for _, bad := range []string{"<script", "<img", "javascript:"} {
		if strings.Contains(strings.ToLower(h), bad) {
			t.Errorf("html contains %q", bad)
		}
	}
	for _, want := range []string{
		`content="default-src 'none'; style-src 'unsafe-inline'"`,
		"&lt;script&gt;alert(1)&lt;/script&gt;|x",
		`<a href="http://ui:8095/#/executions?trace=abc&amp;view=timeline">worst &lt;b&gt;</a>`,
		"bad link (trace <code>def</code>)",
		"<b>p95</b> (advisory)",
		"Quality gate: Failed",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("html is missing %q", want)
		}
	}
	m := markdownReport(ev)
	for _, want := range []string{
		"### ❌ Quality gate: Failed",
		"Run `pr'42` against baseline `main`, 2 checks.",
		"| | Check | Value | Limit | Baseline | Executions |",
		`| ❌ | **db**<br>api / &lt;script&gt;alert(1)&lt;/script&gt;\|x | 21 | 3 | 2 | 200 |`,
		"| ⚠️ | **p95 (advisory)**<br> /  | - | - |  | 0 |",
		"- Evidence: bad link (trace `def`)",
		"- &lt;img src=x onerror=alert(1)&gt;",
		"- Evidence: [worst &lt;b&gt;](http://ui:8095/#/executions?trace=abc&view=timeline)",
		`**db** FAIL: up to 21 \| calls`,
	} {
		if !strings.Contains(m, want) {
			t.Errorf("markdown is missing %q:\n%s", want, m)
		}
	}
}

func TestReportFilesWritten(t *testing.T) {
	rules := writeFile(t, "quality.yaml", rulesYAML)
	s := server(t, 200, answer("failed"), nil)
	dir := t.TempDir()
	h, m := filepath.Join(dir, "r.html"), filepath.Join(dir, "s.md")
	var out, errOut bytes.Buffer
	if code := run([]string{"--url", s.URL, "--rules", rules, "--run", "pr-42", "--html", h, "--markdown", m}, &out, &errOut, fixedNow); code != 1 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	hb, err1 := os.ReadFile(h)
	mb, err2 := os.ReadFile(m)
	if err1 != nil || err2 != nil || !strings.Contains(string(hb), "Quality gate: Failed") || !strings.Contains(string(mb), `permission\_lookup × 20`) {
		t.Errorf("files: %v %v\n%s\n%s", err1, err2, hb, mb)
	}
}

func TestMarkdownNeverLinksScripts(t *testing.T) {
	if m := markdownReport(hostileEvaluation()); strings.Contains(m, "javascript:") {
		t.Errorf("markdown links a script:\n%s", m)
	}
}
