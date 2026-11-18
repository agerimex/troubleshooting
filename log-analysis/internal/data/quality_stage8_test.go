package data

import (
	"strings"
	"testing"
	"time"
)

// Stage 8: the findings of the web agent's review of b7c2e87.

// mixedCoverageRun is the review's reproduction: 200 executions, one complete
// with child counts (log-sender), 199 without counts (OTLP) that lost 19
// database calls and the directory branch each.
func mixedCoverageRun() []qSpan {
	counted := withChildCounts(referenceRun("candidate", "fixed", 560))
	first := counted[0].TraceID
	var out []qSpan
	for _, s := range counted {
		if s.TraceID == first {
			out = append(out, s)
		}
	}
	for _, s := range dropLost(referenceRun("candidate", "candidate", 560)) {
		if s.TraceID != first {
			out = append(out, s)
		}
	}
	return out
}

// Finding 1: one span with a child count selected "lost child spans are
// detected" for the whole run. Coverage is per execution now.
func TestQualityMixedChildCoverage(t *testing.T) {
	rules := referenceRules()
	rules.Run = &QualityRun{Executions: 200}
	req := validRequest(t, rules, "candidate", "")
	res := buildQualityResult(req, runSpans{Spans: mixedCoverageRun()}, runSpans{}, time.Unix(0, qBase))
	if res.Gate != GatePassed {
		t.Fatalf("without the policy the pass stands on the recorded spans: gate %s", res.Gate)
	}
	last := res.Assumptions[len(res.Assumptions)-1]
	if last == AssumptionChildren || !strings.HasPrefix(last, "Lost child spans are detected in 1 of 200 executions only") {
		t.Errorf("assumption %q", last)
	}
	if c := res.ChildCounts; c == nil || c.Executions != 200 || c.Counted != 1 || c.Status != "partial" {
		t.Errorf("coverage %+v", res.ChildCounts)
	}

	// run.child_counts: required makes it unresolved, latency unaffected.
	rules.Run.ChildCounts = ChildCountsRequired
	strict := buildQualityResult(validRequest(t, rules, "candidate", ""), runSpans{Spans: mixedCoverageRun()}, runSpans{}, time.Unix(0, qBase))
	if strict.Gate != GateUnresolved {
		t.Errorf("required: gate %s", strict.Gate)
	}
	for _, id := range []string{"db-calls", "no-directory"} {
		c := checkByID(t, strict, id)
		if c.Status != CheckInsufficient || c.Reason != "unverified_child_counts" || !strings.HasPrefix(c.Summary, "199 of 200 executions don't record child counts") {
			t.Errorf("%s: %s %s %q", id, c.Status, c.Reason, c.Summary)
		}
		if len(c.Evidence) == 0 || !strings.Contains(c.Evidence[0].Note, "without child counts") {
			t.Errorf("%s evidence %+v", id, c.Evidence)
		}
	}
	if c := checkByID(t, strict, "startup-p95"); c.Status != CheckPass {
		t.Errorf("latency doesn't depend on child counts: %s", c.Status)
	}

	// Required, every execution counted: passes. A failure stands either way.
	all := buildQualityResult(validRequest(t, rules, "candidate", ""), runSpans{Spans: withChildCounts(referenceRun("candidate", "fixed", 560))}, runSpans{}, time.Unix(0, qBase))
	if all.Gate != GatePassed || all.ChildCounts.Status != "all" || all.Assumptions[len(all.Assumptions)-1] != AssumptionChildren {
		t.Errorf("all counted: gate %s, %+v", all.Gate, all.ChildCounts)
	}
	failed := buildQualityResult(validRequest(t, rules, "candidate", ""), runSpans{Spans: referenceRun("candidate", "candidate", 910)}, runSpans{}, time.Unix(0, qBase))
	if c := checkByID(t, failed, "db-calls"); c.Status != CheckFail {
		t.Errorf("a failure stands without counts: %s %s", c.Status, c.Reason)
	}
	if failed.ChildCounts.Status != "none" || failed.Assumptions[len(failed.Assumptions)-1] != AssumptionNoCounts {
		t.Errorf("none counted: %+v", failed.ChildCounts)
	}

	// An entry without a recorded count, or a parent span without one, isn't
	// counted: a recorded 0 and no count look the same.
	spans := withChildCounts(referenceRun("candidate", "fixed", 560))
	for i := range spans {
		if spans[i].TraceID == spans[0].TraceID && spans[i].Name == "check_permissions" {
			spans[i].Children = 0
		}
	}
	part := buildQualityResult(req, runSpans{Spans: spans}, runSpans{}, time.Unix(0, qBase))
	if part.ChildCounts.Counted != 199 {
		t.Errorf("a parent without a count: %+v", part.ChildCounts)
	}

	bad := QualityRequest{RulesYAML: "version: 1\nrun: {child_counts: always}\nentry: {service: a, operation: b}\nchecks:\n  - {id: c, type: db_calls, max_per_execution: 1}\n", Run: "r", TimeFrom: num(qBase), TimeTo: num(qBase + 1)}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), `run.child_counts must be "required"`) {
		t.Errorf("validation: %v", err)
	}
	// Setting the policy changes the requirements' criteria; leaving it out
	// keeps the criteria recorded before it existed.
	reqs := requirementsRequest(t, requirementsYAML)
	plain := buildQualityResult(reqs, runSpans{Spans: referenceRun("candidate", "fixed", 560)}, runSpans{}, time.Unix(0, qBase))
	withPolicy := requirementsRequest(t, strings.Replace(requirementsYAML, "checks:\n", "run: {child_counts: required}\nchecks:\n", 1))
	pol := buildQualityResult(withPolicy, runSpans{Spans: referenceRun("candidate", "fixed", 560)}, runSpans{}, time.Unix(0, qBase))
	if requirementByID(t, plain, "REQ-START").Criteria == requirementByID(t, pol, "REQ-START").Criteria {
		t.Error("child_counts: required must change the criteria")
	}
}
