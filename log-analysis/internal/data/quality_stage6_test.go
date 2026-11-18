package data

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// Stage 6: the reproductions of the web agent's review of ede9d2e.

// withChildCounts sets every span's Children to the number of its recorded
// children of the same service, as log-sender reports it.
func withChildCounts(spans []qSpan) []qSpan {
	byID := map[string]*qSpan{}
	for i := range spans {
		byID[spans[i].TraceID+":"+spans[i].SpanID] = &spans[i]
	}
	for i := range spans {
		if p := byID[spans[i].TraceID+":"+spans[i].Parent]; p != nil && p.Service == spans[i].Service {
			p.Children++
		}
	}
	return spans
}

// dropLost removes 19 of the 20 permission lookups and the whole directory
// branch (the client span and the directory's span) of every execution.
func dropLost(spans []qSpan) []qSpan {
	var out []qSpan
	lookups := map[string]int{}
	for _, s := range spans {
		switch {
		case s.Name == "permission_lookup":
			lookups[s.TraceID]++
			if lookups[s.TraceID] > 1 {
				continue
			}
		case s.Name == "GET /users" || s.Service == "directory-api":
			continue
		}
		out = append(out, s)
	}
	return out
}

// Review finding 1: with all 200 entries present, removing child spans
// turned the failing candidate into a pass. With child counts recorded, the
// executions are incomplete instead; without them, the result says it can't
// tell.
func TestQualityLostChildSpans(t *testing.T) {
	rules := referenceRules()
	rules.Run = &QualityRun{Executions: 200}
	req := validRequest(t, rules, "candidate", "")
	full := withChildCounts(referenceRun("candidate", "candidate", 910))
	if res := buildQualityResult(req, runSpans{Spans: full}, runSpans{}, time.Unix(0, qBase)); res.Gate != GateFailed {
		t.Fatalf("complete candidate: gate %s", res.Gate)
	}

	lost := buildQualityResult(req, runSpans{Spans: dropLost(full)}, runSpans{}, time.Unix(0, qBase))
	for _, id := range []string{"db-calls", "no-directory"} {
		c := checkByID(t, lost, id)
		if c.Status != CheckInsufficient || c.Reason != "incomplete_executions" || !strings.Contains(c.Summary, "200 of 200 executions are incomplete") {
			t.Errorf("%s: %s %s %q", id, c.Status, c.Reason, c.Summary)
		}
		if len(c.Evidence) == 0 || !strings.Contains(c.Evidence[0].Note, "started") {
			t.Errorf("%s evidence: %+v", id, c.Evidence)
		}
	}
	if lost.Gate == GatePassed {
		t.Error("lost child spans passed the gate")
	}
	if strings.Join(lost.Assumptions, " ") == "" || lost.Assumptions[len(lost.Assumptions)-1] != AssumptionChildren {
		t.Errorf("assumptions: %q", lost.Assumptions)
	}

	// The fixed run with child counts is complete: it still passes.
	fixed := buildQualityResult(req, runSpans{Spans: withChildCounts(referenceRun("candidate", "fixed", 560))}, runSpans{}, time.Unix(0, qBase))
	if fixed.Gate != GatePassed {
		t.Errorf("fixed with child counts: gate %s", fixed.Gate)
	}

	// Without child counts (OTLP) the loss can't be seen: the pass stands on
	// the recorded spans, and the result says so.
	blind := buildQualityResult(req, runSpans{Spans: dropLost(referenceRun("candidate", "candidate", 910))}, runSpans{}, time.Unix(0, qBase))
	if checkByID(t, blind, "db-calls").Status != CheckPass || blind.Assumptions[len(blind.Assumptions)-1] != AssumptionNoCounts {
		t.Errorf("without child counts: db-calls %s, assumptions %q", checkByID(t, blind, "db-calls").Status, blind.Assumptions)
	}
	if blind.Assumptions[0] != AssumptionRecorded {
		t.Errorf("first assumption %q", blind.Assumptions[0])
	}
	// Children in another service (a remote call) are not counted by the
	// sender: a span whose remote child is missing is not "incomplete".
	remote := withChildCounts(referenceRun("candidate", "fixed", 560))
	if res := buildQualityResult(req, runSpans{Spans: remote}, runSpans{}, time.Unix(0, qBase)); checkByID(t, res, "no-directory").Status != CheckPass {
		t.Errorf("remote children: %+v", checkByID(t, res, "no-directory"))
	}
}

// min_per_execution: an execution with fewer database calls is missing
// spans, so a pass can't stand on it.
func TestQualityMinPerExecution(t *testing.T) {
	rules := QualityRules{Version: 1, Entry: entryRS(), Checks: []QualityCheck{
		{ID: "db-calls", Type: CheckDBCalls, MaxPerExecution: ip(3), MinPerExecution: ip(2)}}}
	req := validRequest(t, rules, "candidate", "")
	fixed := referenceRun("candidate", "fixed", 560)
	if c := checkByID(t, buildQualityResult(req, runSpans{Spans: fixed}, runSpans{}, time.Unix(0, qBase)), "db-calls"); c.Status != CheckPass {
		t.Fatalf("complete: %+v", c)
	}
	// Drop the insert of the first 5 executions: 1 call left in each.
	var part []qSpan
	for _, s := range fixed {
		if s.Name == "insert_session" && s.Start < qBase+5*int64(time.Second) {
			continue
		}
		part = append(part, s)
	}
	c := checkByID(t, buildQualityResult(req, runSpans{Spans: part}, runSpans{}, time.Unix(0, qBase)), "db-calls")
	if c.Status != CheckInsufficient || c.Reason != "missing_database_calls" || !strings.HasPrefix(c.Summary, "5 of 200 executions recorded fewer than 2") ||
		c.Evidence[0].Note != "execution with the fewest database calls: 1" {
		t.Errorf("missing calls: %s %s %q %+v", c.Status, c.Reason, c.Summary, c.Evidence)
	}
	// A failure stands: the candidate's 21 calls break the limit, whatever the
	// executions below the minimum.
	cand := referenceRun("candidate", "candidate", 910)
	var candPart []qSpan
	for _, s := range cand {
		if !(s.DB && s.Start < qBase+int64(time.Second)) {
			candPart = append(candPart, s)
		}
	}
	if c := checkByID(t, buildQualityResult(req, runSpans{Spans: candPart}, runSpans{}, time.Unix(0, qBase)), "db-calls"); c.Status != CheckFail ||
		!strings.Contains(strings.Join(c.Details, "|"), "1 of 200 executions recorded fewer than 2 database calls") {
		t.Errorf("failure with missing calls: %s %q", c.Status, c.Details)
	}
	for name, edit := range map[string]func(*QualityCheck){
		"above max": func(c *QualityCheck) { c.MaxPerExecution, c.MinPerExecution = ip(3), ip(4) },
		"negative":  func(c *QualityCheck) { c.MaxPerExecution, c.MinPerExecution = ip(3), ip(-1) },
		"wrong type": func(c *QualityCheck) {
			c.Type, c.MaxMs, c.MaxPerExecution, c.MinPerExecution = CheckLatency, fp(1), nil, ip(1)
		},
	} {
		r := QualityRules{Version: 1, Entry: entryRS(), Checks: []QualityCheck{{ID: "x", Type: CheckDBCalls}}}
		edit(&r.Checks[0])
		q := QualityRequest{Rules: r, Run: "r", TimeFrom: num(qBase), TimeTo: num(qBase + 1)}
		if err := q.Validate(); err == nil || !strings.Contains(err.Error(), "min_per_execution") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Review finding 3: 220 earlier spans in the trace pushed the offending span
// out of a first-200 snapshot. The evidence span, its ancestors and the
// spans below it come first.
func TestQualitySnapshotKeepsCounterexample(t *testing.T) {
	req := requirementsRequest(t, requirementsYAML)
	req.Record = true
	spans := referenceRun("candidate", "candidate", 910)
	trace := spans[0].TraceID
	for i := 0; i < 220; i++ {
		spans = append(spans, qSpan{TraceID: trace, SpanID: spanHex(1000 + i), Parent: spanHex(1), Service: "recording-api",
			Name: "prepare", Kind: "internal", Start: qBase + int64(i), Duration: 1, Run: "candidate"})
	}
	res := buildQualityResult(req, runSpans{Spans: spans}, runSpans{}, time.Unix(0, qBase))
	ev := checkByID(t, res, "no-directory").Evidence[0]
	if ev.TraceID != trace || ev.Snapshot == nil {
		t.Fatalf("evidence %+v", ev)
	}
	snap := ev.Snapshot
	names := map[string]bool{}
	for _, s := range snap.Spans {
		names[s.Name] = true
	}
	if len(snap.Spans) != maxSnapshotSpans || !snap.Truncated || !names["GET /users"] || !names["GET /users/{id}"] || !names["start_session"] {
		t.Errorf("snapshot: %d spans, truncated %v, has client %v, callee %v, root %v", len(snap.Spans), snap.Truncated, names["GET /users"], names["GET /users/{id}"], names["start_session"])
	}
	for i := 1; i < len(snap.Spans); i++ {
		if snap.Spans[i].Start < snap.Spans[i-1].Start {
			t.Fatal("snapshot spans are not in start order")
		}
	}
	// The db-calls evidence (the entry) keeps its whole execution: all 20
	// lookups, though 220 spans started before them.
	lookups := 0
	for _, s := range checkByID(t, res, "db-calls").Evidence[0].Snapshot.Spans {
		if s.Name == "permission_lookup" {
			lookups++
		}
	}
	if lookups != 20 {
		t.Errorf("db-calls snapshot has %d lookups", lookups)
	}
}

func TestSnapshotSelection(t *testing.T) {
	// 0 root, 1 a, 2 b (child of a), 3 c, 4 d (child of c); focus b, limit 3:
	// b, its ancestors a and root, nothing else.
	ids := []string{"r", "a", "b", "c", "d"}
	parents := []string{"0", "r", "a", "r", "c"}
	if got, trunc := SnapshotSelection(ids, parents, []string{"b"}, 3); jsonOf(got) != "[0,1,2]" || !trunc {
		t.Errorf("focus b: %v %v", got, trunc)
	}
	// Focus c, limit 3: c, root, then d below c.
	if got, _ := SnapshotSelection(ids, parents, []string{"c"}, 3); jsonOf(got) != "[0,3,4]" {
		t.Errorf("focus c: %v", got)
	}
	// Unknown focus: the first spans; everything fits: no truncation.
	if got, trunc := SnapshotSelection(ids, parents, []string{"x"}, 10); jsonOf(got) != "[0,1,2,3,4]" || trunc {
		t.Errorf("all: %v %v", got, trunc)
	}
	// Several focus spans (an evidence span and its database calls): each
	// with its ancestors before the first focus's subtree.
	if got, _ := SnapshotSelection(ids, parents, []string{"a", "d"}, 3); jsonOf(got) != "[0,1,4]" {
		t.Errorf("focus a, d: %v", got)
	}
	// A parent cycle doesn't loop.
	if got, _ := SnapshotSelection([]string{"a", "b"}, []string{"b", "a"}, []string{"a"}, 5); jsonOf(got) != "[0,1]" {
		t.Errorf("cycle: %v", got)
	}
}

func jsonOf(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

type fakeQualityStore struct {
	bodies   map[string]string
	inserted [][]any
	err      error
}

func (f *fakeQualityStore) lookup(_ context.Context, project, id string) (string, bool, error) {
	if f.err != nil {
		return "", false, f.err
	}
	b, ok := f.bodies[project+"/"+id]
	return b, ok, nil
}

func (f *fakeQualityStore) insert(_ context.Context, row []any) error {
	if f.err != nil {
		return f.err
	}
	f.inserted = append(f.inserted, row)
	f.bodies[row[1].(string)+"/"+row[0].(string)] = row[10].(string)
	return nil
}

// Review finding 2: a retry evaluated new data and answered with it under
// the old record's id. A retry is answered with the record itself.
func TestQualityRetryReturnsTheRecord(t *testing.T) {
	store := &fakeQualityStore{bodies: map[string]string{}}
	req := requirementsRequest(t, requirementsYAML)
	req.Record, req.Source = true, &QualitySource{Commit: "c1", CIRun: "ci/1/attempts/1"}
	at := time.Unix(0, qBase).UTC()

	passed := buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "fixed", 560)}, runSpans{}, at)
	recordQuality(context.Background(), store, req, &passed, at)
	if passed.ID == "" || passed.RecordError != "" || passed.RecordedBefore || len(store.inserted) != 1 {
		t.Fatalf("first record: %+v", passed)
	}

	// The same inputs, but the data changed meanwhile (late spans).
	failed := buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "candidate", 910)}, runSpans{}, at.Add(time.Minute))
	recordQuality(context.Background(), store, req, &failed, at.Add(time.Minute))
	if failed.ID != passed.ID || failed.Gate != GatePassed || !failed.RecordedBefore || failed.RecordedAt != passed.RecordedAt || len(store.inserted) != 1 {
		t.Errorf("retry: id %s gate %s before %v at %s, %d inserts", failed.ID, failed.Gate, failed.RecordedBefore, failed.RecordedAt, len(store.inserted))
	}
	if c := checkByID(t, failed, "db-calls"); c.Status != CheckPass {
		t.Errorf("retry answered with the new result: %+v", c)
	}
	var stored QualityResult
	_ = json.Unmarshal([]byte(store.inserted[0][10].(string)), &stored)
	if stored.RecordedBefore || stored.Gate != GatePassed {
		t.Errorf("stored record: %+v", stored)
	}

	// A deliberate re-evaluation is a record of its own.
	req.Reevaluate = true
	again := buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "candidate", 910)}, runSpans{}, at.Add(2*time.Minute))
	recordQuality(context.Background(), store, req, &again, at.Add(2*time.Minute))
	if again.ID == passed.ID || again.Gate != GateFailed || !again.Reevaluated || again.RecordedBefore || len(store.inserted) != 2 {
		t.Errorf("reevaluation: %+v", again)
	}

	// A store error records nothing and keeps the result.
	broken := &fakeQualityStore{bodies: map[string]string{}, err: errors.New("down")}
	res := buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "fixed", 560)}, runSpans{}, at)
	recordQuality(context.Background(), broken, req, &res, at)
	if res.ID != "" || res.RecordError != "the evaluation was not recorded: down" || res.Gate != GatePassed {
		t.Errorf("store error: %+v", res)
	}
	// reevaluate without record is invalid.
	bad := QualityRequest{RulesYAML: requirementsYAML, Run: "r", TimeFrom: num(qBase), TimeTo: num(qBase + 1), Reevaluate: true}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "reevaluate needs record") {
		t.Errorf("reevaluate without record: %v", err)
	}
}
