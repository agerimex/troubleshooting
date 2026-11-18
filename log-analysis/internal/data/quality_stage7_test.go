package data

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// Stage 7: the findings of the web agent's review of 0011a2d.

// Finding 1: two concurrent records of one id could both miss the lookup and
// insert two verdicts. In one process they are serialized; across processes
// the store keeps one insert and every request answers with it.
func TestQualityConcurrentRecordsAreOneRecord(t *testing.T) {
	req := requirementsRequest(t, requirementsYAML)
	req.Record, req.Source = true, &QualitySource{Commit: "c1", CIRun: "ci/1/attempts/1"}
	at := time.Unix(0, qBase).UTC()
	// The two requests read different data (late spans arrived between
	// them), so their fresh verdicts differ.
	results := func() [2]QualityResult {
		return [2]QualityResult{
			buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "fixed", 560)}, runSpans{}, at),
			buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "candidate", 910)}, runSpans{}, at),
		}
	}
	for name, separate := range map[string]bool{"one instance": false, "two instances": true} {
		t.Run(name, func(t *testing.T) {
			store := &fakeQualityStore{bodies: map[string]string{}}
			shared := &recordLocks{}
			if separate {
				// Both lookups miss before either inserts.
				store.missed = &sync.WaitGroup{}
				store.missed.Add(2)
			}
			res := results()
			var wg sync.WaitGroup
			for i := range res {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					locks := shared
					if separate {
						locks = &recordLocks{}
					}
					recordQuality(context.Background(), store, locks, req, &res[i], at.Add(time.Duration(i)*time.Second))
				}(i)
			}
			wg.Wait()
			if len(store.inserted) != 1 {
				t.Fatalf("%d records kept", len(store.inserted))
			}
			if separate && store.dropped != 1 {
				t.Errorf("dropped %d inserts, want 1", store.dropped)
			}
			a, b := res[0], res[1]
			if a.RecordError != "" || b.RecordError != "" || a.ID == "" || a.ID != b.ID {
				t.Fatalf("ids %q %q, errors %q %q", a.ID, b.ID, a.RecordError, b.RecordError)
			}
			if a.Gate != b.Gate || a.RecordedAt != b.RecordedAt || jsonOf(a.Checks) != jsonOf(b.Checks) {
				t.Errorf("two verdicts for one id: %s at %s, %s at %s", a.Gate, a.RecordedAt, b.Gate, b.RecordedAt)
			}
			if a.RecordedBefore == b.RecordedBefore {
				t.Errorf("exactly one answer is the new record: %v %v", a.RecordedBefore, b.RecordedBefore)
			}
			if n := len(shared.locks); n != 0 {
				t.Errorf("%d locks left", n)
			}
		})
	}
}

// The read back after an insert can fail: the answer then says so instead of
// claiming its own verdict was recorded.
func TestQualityRecordReadBackFails(t *testing.T) {
	req := requirementsRequest(t, requirementsYAML)
	req.Record = true
	store := &readBackFails{fakeQualityStore{bodies: map[string]string{}}, 0}
	res := buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "fixed", 560)}, runSpans{}, time.Unix(0, qBase))
	recordQuality(context.Background(), store, &recordLocks{}, req, &res, time.Unix(0, qBase))
	if res.ID == "" || !strings.Contains(res.RecordError, "reading it back failed") {
		t.Errorf("read back failure: id %q, %q", res.ID, res.RecordError)
	}
}

type readBackFails struct {
	fakeQualityStore
	calls int
}

func (r *readBackFails) lookup(ctx context.Context, project, id string) (string, bool, error) {
	if r.calls++; r.calls > 1 {
		return "", false, context.DeadlineExceeded
	}
	return r.fakeQualityStore.lookup(ctx, project, id)
}

const guardrailYAML = `version: 1
project: recording
entry: {service: recording-api, operation: start_session, span_kind: server}
checks:
  - {id: startup-p95, type: latency, max_ms: 800}
  - {id: db-calls, type: db_calls, max_per_execution: 3}
  - {id: no-directory, type: forbidden_call, to: directory-api}
requirements:
  - id: REQ-START
    title: A session starts within 800 ms
    checks: [startup-p95]
    guardrails: [db-calls]
  - id: REQ-DIRECTORY
    title: Starting a session must not call the directory
    checks: [no-directory]
`

// Finding 3: a query budget supports a latency requirement but doesn't show
// that a start was slow. As a guardrail it is listed with the requirement and
// decides the gate, never the requirement's status.
func TestQualityGuardrails(t *testing.T) {
	req := requirementsRequest(t, guardrailYAML)
	// Fast starts with 20 lookups each: latency passes, the budget fails.
	fast := buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "candidate", 400)}, runSpans{}, time.Unix(0, qBase))
	if c := checkByID(t, fast, "db-calls"); c.Status != CheckFail {
		t.Fatalf("db-calls: %+v", c)
	}
	start := requirementByID(t, fast, "REQ-START")
	if start.Status != RequirementMet || start.Summary != "its check passed" || strings.Join(start.Guardrails, ",") != "db-calls" ||
		start.GuardrailSummary != "guardrail failed: db-calls (guardrails decide the gate, not this requirement)" {
		t.Errorf("REQ-START: %+v", start)
	}
	if fast.Gate != GateFailed {
		t.Errorf("a failed guardrail still decides the gate: %s", fast.Gate)
	}
	fixed := buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "fixed", 560)}, runSpans{}, time.Unix(0, qBase))
	if r := requirementByID(t, fixed, "REQ-START"); r.GuardrailSummary != "guardrail passed: db-calls" {
		t.Errorf("fixed: %+v", r)
	}
	none := buildQualityResult(req, runSpans{Problem: "outside_retention", Detail: "gone"}, runSpans{}, time.Unix(0, qBase))
	if r := requirementByID(t, none, "REQ-START"); r.Status != RequirementInsufficient ||
		r.GuardrailSummary != "guardrail without enough data: db-calls (guardrails decide the gate, not this requirement)" {
		t.Errorf("no data: %+v", r)
	}
	// Without guardrails the field is left out.
	if r := requirementByID(t, fixed, "REQ-DIRECTORY"); r.Guardrails != nil || r.GuardrailSummary != "" {
		t.Errorf("REQ-DIRECTORY: %+v", r)
	}
	// Guardrails link their checks: none is "without a requirement".
	if c := buildCatalogue("recording", "", fixed, []QualityEvaluationSummary{summaryOf(fixed)}); len(c.Unlinked) != 0 {
		t.Errorf("unlinked %v", c.Unlinked)
	}

	for name, tc := range map[string]struct{ edit, want string }{
		"unknown":  {"guardrails: [db-call]", `requirement REQ-START: guardrail "db-call" is not in the rules`},
		"twice":    {"guardrails: [db-calls, db-calls]", "requirement REQ-START: guardrail db-calls is listed twice"},
		"as check": {"guardrails: [startup-p95]", "requirement REQ-START: check startup-p95 is both a check and a guardrail"},
	} {
		bad := QualityRequest{RulesYAML: strings.Replace(guardrailYAML, "guardrails: [db-calls]", tc.edit, 1), Run: "r", TimeFrom: num(qBase), TimeTo: num(qBase + 1)}
		if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
}

// Finding 2: the streak compared statuses only, so two passes against
// different limits read as one "met since". The criteria of a requirement
// (its checks' definitions) are recorded, and a change ends the streak.
func TestQualityStreakEndsWhenCriteriaChange(t *testing.T) {
	criteria := func(text string) string {
		res := buildQualityResult(requirementsRequest(t, text), runSpans{Spans: referenceRun("candidate", "fixed", 560)}, runSpans{}, time.Unix(0, qBase))
		return requirementByID(t, res, "REQ-START").Criteria
	}
	base := criteria(guardrailYAML)
	if len(base) != 64 {
		t.Fatalf("criteria %q", base)
	}
	// What doesn't change a status doesn't change the criteria: wording,
	// targets, advisory, check ids and order, guardrails, other requirements.
	for name, edit := range map[string][2]string{
		"title":      {"title: A session starts within 800 ms", "title: Sessions start fast"},
		"target":     {"max_ms: 800}", "max_ms: 800, target_ms: 500}"},
		"advisory":   {"max_ms: 800}", "max_ms: 800, advisory: true}"},
		"guardrails": {"    guardrails: [db-calls]\n", ""},
		"other":      {"to: directory-api}", "to: directory-service}"},
	} {
		if c := criteria(strings.Replace(guardrailYAML, edit[0], edit[1], 1)); c != base {
			t.Errorf("%s changed the criteria", name)
		}
	}
	for name, edit := range map[string][2]string{
		"limit":      {"max_ms: 800}", "max_ms: 1200}"},
		"percentile": {"max_ms: 800}", "max_ms: 800, percentile: 99}"},
		"executions": {"entry: {", "run: {executions: 200}\nentry: {"},
		"checks":     {"checks: [startup-p95]", "checks: [startup-p95, db-calls]"},
	} {
		text := strings.Replace(guardrailYAML, edit[0], edit[1], 1)
		if name == "checks" {
			text = strings.Replace(text, "    guardrails: [db-calls]\n", "", 1)
		}
		if c := criteria(text); c == base {
			t.Errorf("%s didn't change the criteria", name)
		}
	}

	point := func(id, status, crit, sha string) QualityHistoryPoint {
		return QualityHistoryPoint{EvaluationID: id, Status: status, Criteria: crit, RulesSHA256: sha}
	}
	// Met under 1200 ms, then met under 800 ms: two streaks.
	s := streakOf([]QualityHistoryPoint{point("e3", "met", "k800", "s2"), point("e2", "met", "k1200", "s1"), point("e1", "met", "k1200", "s1")})
	if s.Evaluations != 1 || s.Since.EvaluationID != "e3" || s.StatusBefore != "met" || !s.CriteriaChanged || s.AtLeast {
		t.Errorf("criteria change: %+v", s)
	}
	// Another requirement's edit changes the rules but not these criteria.
	s = streakOf([]QualityHistoryPoint{point("e2", "met", "k", "s2"), point("e1", "met", "k", "s1")})
	if s.Evaluations != 2 || !s.AtLeast || s.CriteriaChanged {
		t.Errorf("same criteria: %+v", s)
	}
	// Records from before criteria existed: the rules sha256 decides.
	s = streakOf([]QualityHistoryPoint{point("e3", "met", "k", "s2"), point("e2", "met", "", "s2"), point("e1", "met", "", "s1")})
	if s.Evaluations != 2 || s.Since.EvaluationID != "e2" || !s.CriteriaChanged {
		t.Errorf("old records: %+v", s)
	}
	// A status change with changed criteria says both.
	s = streakOf([]QualityHistoryPoint{point("e2", "met", "a", "s2"), point("e1", "violated", "b", "s1")})
	if s.Evaluations != 1 || s.StatusBefore != "violated" || !s.CriteriaChanged {
		t.Errorf("status and criteria: %+v", s)
	}
	// Not evaluated and absent have no criteria: rule edits don't end them.
	s = streakOf([]QualityHistoryPoint{point("e2", "not_evaluated", "", "s2"), point("e1", "not_evaluated", "", "s1")})
	if s.Evaluations != 2 || s.CriteriaChanged {
		t.Errorf("not evaluated: %+v", s)
	}
	s = streakOf([]QualityHistoryPoint{point("e2", "met", "a", "s2"), point("e1", "absent", "", "s1")})
	if s.StatusBefore != "absent" || s.CriteriaChanged {
		t.Errorf("absent before: %+v", s)
	}

	// The catalogue carries the criteria from the summaries.
	req := requirementsRequest(t, guardrailYAML)
	latest := buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "fixed", 560)}, runSpans{}, time.Unix(0, qBase))
	older := summaryOf(latest)
	older.ID = "old"
	older.Requirements[0].Criteria = "other"
	latestSum := summaryOf(latest)
	latestSum.ID = "new"
	cat := buildCatalogue("recording", "", latest, []QualityEvaluationSummary{latestSum, older})
	if st := cat.Requirements[0].Streak; st.Evaluations != 1 || !st.CriteriaChanged || cat.Requirements[0].History[0].Criteria != base {
		t.Errorf("catalogue streak: %+v", st)
	}
}
