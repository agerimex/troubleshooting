package data

import (
	"strings"
	"testing"
	"time"
)

// failingRun is the reference baseline run with the first `failed` entry
// spans ending in error after 50 ms (a failure that ends early).
func failingRun(failed int) []qSpan {
	spans := referenceRun("candidate", "baseline", 420)
	n := 0
	for i := range spans {
		if spans[i].Parent == RootParentSpanID && n < failed {
			spans[i].Status, spans[i].Duration = statusError, 50*1000
			n++
		}
	}
	return spans
}

func errorRules(maxFailed int, excludeFailed bool) QualityRules {
	return QualityRules{Version: 1, Entry: entryRS(), Run: &QualityRun{Executions: 200}, Checks: []QualityCheck{
		{ID: "no-failures", Type: CheckErrors, MaxFailed: ip(maxFailed)},
		{ID: "startup-p95", Type: CheckLatency, MaxMs: fp(800), ExcludeFailed: excludeFailed},
	}}
}

func TestQualityErrorsCheck(t *testing.T) {
	now := time.Unix(0, qBase)
	clean := buildQualityResult(validRequest(t, errorRules(0, false), "candidate", ""), runSpans{Spans: failingRun(0)}, runSpans{}, now)
	if c := checkByID(t, clean, "no-failures"); c.Status != CheckPass || *c.Value != 0 || c.Summary != "0 of 200 executions failed, limit 0" {
		t.Errorf("clean: %s %v %q", c.Status, c.Value, c.Summary)
	}
	res := buildQualityResult(validRequest(t, errorRules(0, false), "candidate", ""), runSpans{Spans: failingRun(3)}, runSpans{}, now)
	c := checkByID(t, res, "no-failures")
	if c.Status != CheckFail || c.Reason != "limit_exceeded" || *c.Value != 3 || c.Violations != 3 || len(c.Evidence) != 1 ||
		!strings.Contains(c.Evidence[0].Note, "status error") || res.Gate != GateFailed {
		t.Errorf("3 failed: %+v gate %s", c, res.Gate)
	}
	if c := checkByID(t, buildQualityResult(validRequest(t, errorRules(3, false), "candidate", ""), runSpans{Spans: failingRun(3)}, runSpans{}, now), "no-failures"); c.Status != CheckPass {
		t.Errorf("3 failed, 3 allowed: %s", c.Status)
	}
	// Lost executions are not a pass.
	if c := checkByID(t, buildQualityResult(validRequest(t, errorRules(0, false), "candidate", ""), runSpans{Spans: failingRun(0)[:300]}, runSpans{}, now), "no-failures"); c.Status != CheckInsufficient || c.Reason != "missing_executions" {
		t.Errorf("lost executions: %s %s", c.Status, c.Reason)
	}
}

// Failures that end early make a run look faster; exclude_failed leaves them
// out of the percentile.
func TestQualityLatencyExcludeFailed(t *testing.T) {
	now := time.Unix(0, qBase)
	with := checkByID(t, buildQualityResult(validRequest(t, errorRules(20, false), "candidate", ""), runSpans{Spans: failingRun(20)}, runSpans{}, now), "startup-p95")
	without := checkByID(t, buildQualityResult(validRequest(t, errorRules(20, true), "candidate", ""), runSpans{Spans: failingRun(20)}, runSpans{}, now), "startup-p95")
	if with.Value == nil || without.Value == nil || *without.Value <= *with.Value {
		t.Fatalf("p95 with failures %v, without %v: leaving the fast failures out must raise it", with.Value, without.Value)
	}
	if without.Status != CheckPass || !strings.Contains(strings.Join(without.Details, "|"), "20 failed executions left out (exclude_failed)") {
		t.Errorf("exclude_failed: %s %v", without.Status, without.Details)
	}
	// Too few successful executions for a percentile: not enough data.
	few := checkByID(t, buildQualityResult(validRequest(t, errorRules(200, true), "candidate", ""), runSpans{Spans: failingRun(190)}, runSpans{}, now), "startup-p95")
	if few.Status != CheckInsufficient || few.Reason != "too_few_executions" || !strings.HasPrefix(few.Summary, "10 successful executions of 200") {
		t.Errorf("10 successful: %s %s %q", few.Status, few.Reason, few.Summary)
	}
}

func TestQualityErrorsValidation(t *testing.T) {
	for name, c := range map[string]QualityCheck{
		"no max_failed":         {ID: "e", Type: CheckErrors},
		"negative":              {ID: "e", Type: CheckErrors, MaxFailed: ip(-1)},
		"foreign field":         {ID: "e", Type: CheckErrors, MaxFailed: ip(0), MaxMs: fp(5)},
		"max_failed on latency": {ID: "l", Type: CheckLatency, MaxMs: fp(5), MaxFailed: ip(0)},
		"exclude on db_calls":   {ID: "d", Type: CheckDBCalls, MaxPerExecution: ip(1), ExcludeFailed: true},
	} {
		req := QualityRequest{Rules: QualityRules{Version: 1, Entry: entryRS(), Checks: []QualityCheck{c}}, Run: "r", TimeFrom: "1", TimeTo: "2"}
		if err := req.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	rules, err := ParseQualityRules("version: 1\nentry: {service: a, operation: b}\nchecks:\n  - {id: e, type: errors, max_failed: 0}\n  - {id: l, type: latency, max_ms: 5, exclude_failed: true}\n")
	if err != nil || *rules.Checks[0].MaxFailed != 0 || !rules.Checks[1].ExcludeFailed {
		t.Errorf("yaml: %v %+v", err, rules.Checks)
	}
}

// A check's own min_executions also applies to the successful executions
// (found on a real run: 11 of 14 successful, min_executions 5).
func TestQualityExcludeFailedOwnMinimum(t *testing.T) {
	rules := QualityRules{Version: 1, Entry: entryRS(), Run: &QualityRun{Executions: 200}, Checks: []QualityCheck{
		{ID: "p95", Type: CheckLatency, MaxMs: fp(800), MinExecutions: 5, ExcludeFailed: true}}}
	c := checkByID(t, buildQualityResult(validRequest(t, rules, "candidate", ""), runSpans{Spans: failingRun(190)}, runSpans{}, time.Unix(0, qBase)), "p95")
	if c.Status != CheckPass || c.Executions != 200 {
		t.Errorf("10 successful, min 5: %s %s %q", c.Status, c.Reason, c.Summary)
	}
}
