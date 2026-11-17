package data

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The reference case of docs/quality-gate.md: recording-api start_session,
// 200 executions per run. Baseline: one permission lookup and one insert;
// candidate: 20 lookups, an insert and a synchronous directory-api call;
// fixed: one batched lookup and one insert. p95 420 / 910 / 560 ms.

const qBase = int64(1_700_000_000_000_000_000)

func ip(v int) *int         { return &v }
func fp(v float64) *float64 { return &v }
func entryRS() *QualityEntry {
	return &QualityEntry{Service: "recording-api", Operation: "start_session", SpanKind: "server"}
}
func spanHex(n int) string { return fmt.Sprintf("%016x", n) }
func traceHex(run string, n int) string {
	h := 0
	for _, c := range run {
		h = h*31 + int(c)
	}
	return fmt.Sprintf("%016x%016x", h&0xffffffff, n+1)
}

func referenceRules() QualityRules {
	return QualityRules{Version: 1, Entry: entryRS(), Checks: []QualityCheck{
		{ID: "startup-p95", Type: CheckLatency, MaxMs: fp(800), TargetMs: fp(500)},
		{ID: "db-calls", Type: CheckDBCalls, MaxPerExecution: ip(3), TargetPer: ip(2)},
		{ID: "no-directory", Type: CheckForbidden, To: "directory-api"},
	}}
}

// p95ms gives execution i of 200 a duration in ms such that the nearest-rank
// p95 (the 190th smallest) is exactly p95.
func p95ms(i int, p95 float64) float64 {
	switch {
	case i < 189:
		return p95 - 60 + float64(i%50)
	case i == 189:
		return p95
	default:
		return p95 + 100
	}
}

// referenceRun builds one run: kind is "baseline", "candidate" or "fixed".
func referenceRun(run, kind string, p95 float64) []qSpan {
	var out []qSpan
	for i := 0; i < 200; i++ {
		trace := traceHex(run, i)
		start := qBase + int64(i)*int64(time.Second)
		dur := int64(p95ms(i, p95) * 1000)
		entry := qSpan{TraceID: trace, SpanID: spanHex(1), Parent: RootParentSpanID, Service: "recording-api", Name: "start_session",
			Kind: "server", Start: start, Duration: dur, Run: run}
		out = append(out, entry)
		id := 2
		add := func(parent string, s qSpan) string {
			s.TraceID, s.SpanID, s.Parent, s.Run = trace, spanHex(id), parent, run
			s.Start = start + int64(id)*int64(time.Millisecond)
			if s.Duration == 0 {
				s.Duration = 1000
			}
			id++
			out = append(out, s)
			return s.SpanID
		}
		perm := add(spanHex(1), qSpan{Service: "recording-api", Name: "check_permissions", Kind: "internal"})
		switch kind {
		case "baseline":
			add(perm, qSpan{Service: "recording-api", Name: "permission_lookup", Kind: "client", DB: true})
		case "candidate":
			for p := 0; p < 20; p++ {
				add(perm, qSpan{Service: "recording-api", Name: "permission_lookup", Kind: "client", DB: true})
			}
			client := add(spanHex(1), qSpan{Service: "recording-api", Name: "GET /users", Kind: "client"})
			add(client, qSpan{Service: "directory-api", Name: "GET /users/{id}", Kind: "server"})
		case "fixed":
			add(perm, qSpan{Service: "recording-api", Name: "permission_batch_lookup", Kind: "client", DB: true})
		}
		add(spanHex(1), qSpan{Service: "recording-api", Name: "insert_session", Kind: "client", DB: true})
	}
	return out
}

func validRequest(t *testing.T, rules QualityRules, run, baseline string) QualityRequest {
	t.Helper()
	req := QualityRequest{Rules: rules, Run: run, Baseline: baseline,
		TimeFrom: num(qBase), TimeTo: num(qBase + int64(time.Hour))}
	if err := req.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	return req
}

func checkByID(t *testing.T, res QualityResult, id string) QualityCheckResult {
	t.Helper()
	for _, c := range res.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no check %s", id)
	return QualityCheckResult{}
}

func TestQualityReferenceCandidateFails(t *testing.T) {
	req := validRequest(t, referenceRules(), "candidate", "baseline")
	res := buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "candidate", 910)},
		runSpans{Spans: referenceRun("baseline", "baseline", 420)}, time.Unix(0, qBase))
	if res.Gate != GateFailed || res.Contract != QualityContract {
		t.Fatalf("gate %s, contract %s", res.Gate, res.Contract)
	}
	lat := checkByID(t, res, "startup-p95")
	if lat.Status != CheckFail || lat.Reason != "limit_exceeded" || *lat.Value != 910 || lat.Executions != 200 || lat.Violations != 200 {
		t.Errorf("latency: %+v value %v", lat, *lat.Value)
	}
	if lat.Baseline == nil || *lat.Baseline.Value != 420 || lat.Baseline.Status != CheckPass {
		t.Errorf("latency baseline: %+v", lat.Baseline)
	}
	if lat.TargetStatus != TargetOpen || len(lat.Evidence) != 1 || lat.Evidence[0].SpanID != spanHex(1) {
		t.Errorf("latency target/evidence: %s %+v", lat.TargetStatus, lat.Evidence)
	}
	db := checkByID(t, res, "db-calls")
	if db.Status != CheckFail || *db.Value != 21 || db.Violations != 200 || *db.Baseline.Value != 2 {
		t.Errorf("db calls: %+v", db)
	}
	if strings.Join(db.Details, "|") != "target 2 per execution not met yet (advisory)|permission_lookup × 20|insert_session × 1" {
		t.Errorf("db details: %q", db.Details)
	}
	dir := checkByID(t, res, "no-directory")
	if dir.Status != CheckFail || dir.Reason != "forbidden_call" || dir.Violations != 200 || *dir.Baseline.Value != 0 {
		t.Errorf("directory: %+v", dir)
	}
	if len(dir.Evidence) != 1 || !strings.Contains(dir.Evidence[0].Note, "recording-api → directory-api (GET /users)") {
		t.Errorf("directory evidence: %+v", dir.Evidence)
	}
	if dir.Baseline.Status != CheckPass {
		t.Errorf("baseline directory status %s", dir.Baseline.Status)
	}
}

// The fix passes the limit but not the latency target, and stays slower than
// the baseline: a pass is not a claim that every number improved.
func TestQualityReferenceFixPasses(t *testing.T) {
	req := validRequest(t, referenceRules(), "fixed", "baseline")
	res := buildQualityResult(req, runSpans{Spans: referenceRun("fixed", "fixed", 560)},
		runSpans{Spans: referenceRun("baseline", "baseline", 420)}, time.Unix(0, qBase))
	if res.Gate != GatePassed {
		t.Fatalf("gate %s: %+v", res.Gate, res.Checks)
	}
	lat := checkByID(t, res, "startup-p95")
	if *lat.Value != 560 || lat.TargetStatus != TargetOpen || *lat.Baseline.Value != 420 {
		t.Errorf("latency: %+v", lat)
	}
	db := checkByID(t, res, "db-calls")
	if *db.Value != 2 || db.TargetStatus != TargetMet || db.Status != CheckPass {
		t.Errorf("db: %+v", db)
	}
	dir := checkByID(t, res, "no-directory")
	if dir.Status != CheckPass || dir.Reason != "not_observed" {
		t.Errorf("directory: %+v", dir)
	}
	b, err := json.Marshal(res)
	if err != nil || !strings.Contains(string(b), `"gate":"passed"`) {
		t.Errorf("json: %v %s", err, b)
	}
}

// Equality meets the limit; one unit above fails.
func TestQualityLatencyBoundary(t *testing.T) {
	for _, tc := range []struct {
		p95  float64
		want string
	}{{800, CheckPass}, {800.001, CheckFail}} {
		rules := QualityRules{Version: 1, Entry: entryRS(), Checks: []QualityCheck{{ID: "p95", Type: CheckLatency, MaxMs: fp(800)}}}
		req := validRequest(t, rules, "r", "")
		res := buildQualityResult(req, runSpans{Spans: referenceRun("r", "fixed", tc.p95)}, runSpans{}, time.Now())
		if got := res.Checks[0].Status; got != tc.want {
			t.Errorf("p95 %v: %s, want %s (value %v)", tc.p95, got, tc.want, *res.Checks[0].Value)
		}
	}
}

func TestNearestRank(t *testing.T) {
	v := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for p, want := range map[float64]float64{95: 10, 90: 9, 50: 5, 10: 1, 1: 1, 100: 10} {
		if got := nearestRank(v, p); got != want {
			t.Errorf("p%v = %v, want %v", p, got, want)
		}
	}
}

// Missing evidence never passes: no executions, too few, or a load problem.
func TestQualityInsufficientEvidence(t *testing.T) {
	rules := referenceRules()
	req := validRequest(t, rules, "other-run", "")
	res := buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "candidate", 910)}, runSpans{}, time.Now())
	if res.Gate != GateUnresolved {
		t.Fatalf("spans of another run were counted: %s", res.Gate)
	}
	for _, c := range res.Checks {
		if c.Status != CheckInsufficient || c.Reason != "no_executions" {
			t.Errorf("%s: %s %s", c.ID, c.Status, c.Reason)
		}
	}

	few := referenceRun("r", "fixed", 560)
	var nineteen []qSpan
	for _, s := range few {
		if s.Start < qBase+19*int64(time.Second) {
			nineteen = append(nineteen, s)
		}
	}
	req = validRequest(t, rules, "r", "")
	res = buildQualityResult(req, runSpans{Spans: nineteen}, runSpans{}, time.Now())
	lat := checkByID(t, res, "startup-p95")
	if lat.Status != CheckInsufficient || lat.Reason != "too_few_executions" || !strings.Contains(lat.Summary, "19 executions, at least 20 needed") {
		t.Errorf("19 executions: %+v", lat)
	}
	if res.Gate != GateUnresolved {
		t.Errorf("gate %s", res.Gate)
	}

	res = buildQualityResult(req, runSpans{Problem: "query_limit", Detail: "query timed out"}, runSpans{}, time.Now())
	for _, c := range res.Checks {
		if c.Status != CheckInsufficient || c.Reason != "query_limit" || c.Summary != "query timed out" {
			t.Errorf("%s: %+v", c.ID, c)
		}
	}
}

// Entries outside the window are not executions; their children still are
// read up to the lookahead, so an execution ending after the window counts.
func TestQualityWindow(t *testing.T) {
	spans := referenceRun("r", "fixed", 560)
	rules := QualityRules{Version: 1, Entry: entryRS(), Checks: []QualityCheck{{ID: "db", Type: CheckDBCalls, MaxPerExecution: ip(3)}}}
	req := QualityRequest{Rules: rules, Run: "r", TimeFrom: num(qBase + 10*int64(time.Second)), TimeTo: num(qBase + 20*int64(time.Second))}
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	res := buildQualityResult(req, runSpans{Spans: spans}, runSpans{}, time.Now())
	if res.Checks[0].Executions != 10 {
		t.Errorf("executions in [10s, 20s): %d", res.Checks[0].Executions)
	}
}

func TestQualityGatePrecedence(t *testing.T) {
	c := func(status string, advisory bool) QualityCheckResult {
		return QualityCheckResult{Status: status, Advisory: advisory}
	}
	for _, tc := range []struct {
		checks []QualityCheckResult
		want   string
	}{
		{[]QualityCheckResult{c(CheckPass, false)}, GatePassed},
		{[]QualityCheckResult{c(CheckInsufficient, false), c(CheckFail, false)}, GateFailed},
		{[]QualityCheckResult{c(CheckPass, false), c(CheckInsufficient, false)}, GateUnresolved},
		{[]QualityCheckResult{c(CheckPass, false), c(CheckFail, true)}, GatePassed},
		{[]QualityCheckResult{c(CheckPass, false), c(CheckInsufficient, true)}, GatePassed},
	} {
		if got := gateOf(tc.checks); got != tc.want {
			t.Errorf("%+v: %s, want %s", tc.checks, got, tc.want)
		}
	}
}

// A CLIENT naming an uninstrumented peer is a call to it; "from" restricts
// the caller; a call inside the forbidden service is not a call to it.
func TestQualityForbiddenCall(t *testing.T) {
	tr := strings.Repeat("a", 32)
	spans := []qSpan{
		{TraceID: tr, SpanID: spanHex(1), Parent: RootParentSpanID, Service: "api", Name: "start", Kind: "server", Start: qBase, Duration: 1000, Run: "r"},
		{TraceID: tr, SpanID: spanHex(2), Parent: spanHex(1), Service: "api", Name: "call billing", Kind: "client", Start: qBase + 1, Peer: "billing", Run: "r"},
		{TraceID: tr, SpanID: spanHex(3), Parent: spanHex(1), Service: "worker", Name: "job", Kind: "server", Start: qBase + 2},
		{TraceID: tr, SpanID: spanHex(4), Parent: spanHex(3), Service: "worker", Name: "inner", Kind: "internal", Start: qBase + 3},
	}
	entry := &QualityEntry{Service: "api", Operation: "start"}
	run := func(c QualityCheck) QualityCheckResult {
		req := validRequest(t, QualityRules{Version: 1, Entry: entry, Checks: []QualityCheck{c}}, "r", "")
		return buildQualityResult(req, runSpans{Spans: spans}, runSpans{}, time.Now()).Checks[0]
	}
	if r := run(QualityCheck{ID: "a", Type: CheckForbidden, To: "billing"}); r.Status != CheckFail || r.Evidence[0].SpanID != spanHex(2) {
		t.Errorf("uninstrumented peer: %+v", r)
	}
	if r := run(QualityCheck{ID: "b", Type: CheckForbidden, To: "worker"}); r.Status != CheckFail || r.Evidence[0].SpanID != spanHex(1) {
		t.Errorf("server span of worker: %+v", r)
	}
	if r := run(QualityCheck{ID: "c", Type: CheckForbidden, From: "billing", To: "worker"}); r.Status != CheckPass {
		t.Errorf("caller filter: %+v", r)
	}
	if r := run(QualityCheck{ID: "d", Type: CheckForbidden, To: "api"}); r.Status != CheckPass {
		t.Errorf("the entry's own service is not a call: %+v", r)
	}
}

// An execution with a span whose parent was not recorded (starting within
// the entry's duration) is incomplete: a count or an absence can't pass, a
// failure stands, latency is unaffected. The entry's own missing parent and
// orphans outside the entry's duration don't make it incomplete.
func TestQualityIncompleteExecutions(t *testing.T) {
	tr := strings.Repeat("b", 32)
	entry := qSpan{TraceID: tr, SpanID: spanHex(1), Parent: RootParentSpanID, Service: "api", Name: "start", Kind: "server", Start: qBase, Duration: 1000, Run: "r"}
	db := func(id int, parent string, start int64) qSpan {
		return qSpan{TraceID: tr, SpanID: spanHex(id), Parent: parent, Service: "api", Name: "lookup", Kind: "client", DB: true, Start: start, Duration: 10}
	}
	orphanCall := qSpan{TraceID: tr, SpanID: spanHex(9), Parent: spanHex(8), Service: "directory", Name: "GET /users", Kind: "server", Start: qBase + 5, Duration: 10}
	rules := QualityRules{Version: 1, Entry: &QualityEntry{Service: "api", Operation: "start"}, Checks: []QualityCheck{
		{ID: "db", Type: CheckDBCalls, MaxPerExecution: ip(1), TargetPer: ip(1)},
		{ID: "dir", Type: CheckForbidden, To: "directory"},
		{ID: "p95", Type: CheckLatency, MaxMs: fp(10), MinExecutions: 1},
	}}
	eval := func(spans ...qSpan) []QualityCheckResult {
		return buildQualityResult(validRequest(t, rules, "r", ""), runSpans{Spans: spans}, runSpans{}, time.Now()).Checks
	}

	r := eval(entry, db(2, spanHex(1), qBase+2), orphanCall)
	if r[0].Status != CheckInsufficient || r[0].Reason != "incomplete_executions" || r[0].TargetStatus != "" || *r[0].Value != 1 ||
		!strings.Contains(r[0].Summary, "1 of 1 executions are incomplete") || len(r[0].Evidence) != 2 || r[0].Evidence[0].SpanID != spanHex(9) ||
		!strings.Contains(r[0].Evidence[0].Note, "names parent "+spanHex(8)) {
		t.Errorf("db calls of an incomplete execution: %+v", r[0])
	}
	if r[1].Status != CheckInsufficient || r[1].Reason != "incomplete_executions" {
		t.Errorf("absence in an incomplete execution: %+v", r[1])
	}
	if r[2].Status != CheckPass {
		t.Errorf("latency is the entry's own duration: %+v", r[2])
	}

	// A counted violation stands: the recorded calls are real.
	r = eval(entry, db(2, spanHex(1), qBase+2), db(3, spanHex(1), qBase+3), orphanCall)
	if r[0].Status != CheckFail || !strings.Contains(strings.Join(r[0].Details, "|"), "1 of 1 executions are incomplete") {
		t.Errorf("failure with an incomplete execution: %+v", r[0])
	}
	recorded := orphanCall
	recorded.Parent = spanHex(2)
	if r = eval(entry, db(2, spanHex(1), qBase+2), recorded); r[1].Status != CheckFail {
		t.Errorf("recorded forbidden call: %+v", r[1])
	}

	// Not incomplete: the entry's own parent is missing (an uninstrumented
	// gateway), or the orphan starts outside the entry's duration.
	gateway := entry
	gateway.Parent = spanHex(7)
	late := orphanCall
	late.Start = qBase + 1000*1000 + 1
	early := orphanCall
	early.Start = qBase - 1
	for name, spans := range map[string][]qSpan{"gateway": {gateway, db(2, spanHex(1), qBase+2)}, "late": {entry, late}, "early": {entry, early}} {
		if r = eval(spans...); r[0].Status != CheckPass || r[1].Status != CheckPass || len(r[0].Evidence) > 0 && r[0].Evidence[0].SpanID == spanHex(9) {
			t.Errorf("%s: %+v / %+v", name, r[0], r[1])
		}
	}
	// An invalid entry duration: any orphan from its start on counts.
	open := entry
	open.Duration = -1
	if r = eval(open, late); r[0].Status != CheckInsufficient {
		t.Errorf("invalid duration: %+v", r[0])
	}

	// Not incomplete: a sibling of the entry under the same unrecorded parent
	// (the uninstrumented gateway also calls another service meanwhile) is
	// beside the entry, not below it.
	sibling := orphanCall
	sibling.Parent = gateway.Parent
	if r = eval(gateway, db(2, spanHex(1), qBase+2), sibling); r[0].Status != CheckPass || r[1].Status != CheckPass {
		t.Errorf("sibling under the entry's missing parent: %+v / %+v", r[0], r[1])
	}

	// An execution running past the spans read (window end + lookahead) is
	// incomplete: its last calls were not read.
	long := entry
	long.Start = qBase + int64(time.Hour) - int64(time.Second)
	long.Duration = int64(10*time.Minute) / 1000 // microseconds
	r = eval(long, db(2, spanHex(1), long.Start+1))
	if r[0].Status != CheckInsufficient || r[0].Reason != "incomplete_executions" || r[1].Status != CheckInsufficient ||
		!strings.Contains(r[0].Evidence[0].Note, "runs past the spans read") || !strings.Contains(strings.Join(r[0].Details, "|"), "end the window later") {
		t.Errorf("execution past the spans read: %+v / %+v", r[0], r[1])
	}
	short := long
	short.Duration = int64(time.Minute) / 1000
	if r = eval(short, db(2, spanHex(1), short.Start+1)); r[0].Status != CheckPass {
		t.Errorf("execution ending within the lookahead: %+v", r[0])
	}
}

// A field of another check type is an error, never silently ignored.
func TestQualityForeignFields(t *testing.T) {
	c := QualityCheck{ID: "db", Type: CheckDBCalls, MaxPerExecution: ip(3), MaxMs: fp(200), To: "x"}
	err := validateCheck(&c, &QualityEntry{Service: "s", Operation: "o"})
	if err == nil || err.Error() != "max_ms, to don't apply to a db_calls check" {
		t.Errorf("foreign fields: %v", err)
	}
}

func TestQualityValidate(t *testing.T) {
	ok := func() QualityRequest {
		return QualityRequest{Rules: referenceRules(), Run: "r", TimeFrom: num(qBase), TimeTo: num(qBase + 1)}
	}
	req := ok()
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	c := req.Rules.Checks
	if c[0].Percentile != 95 || c[0].MinExecutions != 20 || c[1].MinExecutions != 1 || c[2].Entry.Service != "recording-api" {
		t.Errorf("defaults: %+v", c)
	}
	req = ok()
	req.Baseline = "b"
	if err := req.Validate(); err != nil || req.BaselineFrom != req.TimeFrom || req.BaselineTo != req.TimeTo {
		t.Errorf("baseline window default: %v %+v", err, req)
	}
	for name, mutate := range map[string]func(*QualityRequest){
		"no run":                  func(q *QualityRequest) { q.Run = " " },
		"same baseline":           func(q *QualityRequest) { q.Baseline = "r" },
		"window":                  func(q *QualityRequest) { q.TimeTo = q.TimeFrom },
		"long window":             func(q *QualityRequest) { q.TimeTo = num(qBase + int64(25*time.Hour)) },
		"baseline no run":         func(q *QualityRequest) { q.BaselineFrom = q.TimeFrom },
		"version":                 func(q *QualityRequest) { q.Rules.Version = 2 },
		"no checks":               func(q *QualityRequest) { q.Rules.Checks = nil },
		"duplicate id":            func(q *QualityRequest) { q.Rules.Checks[1].ID = q.Rules.Checks[0].ID },
		"bad id":                  func(q *QualityRequest) { q.Rules.Checks[0].ID = "a b" },
		"no entry":                func(q *QualityRequest) { q.Rules.Entry = nil },
		"type":                    func(q *QualityRequest) { q.Rules.Checks[0].Type = "sql" },
		"target above max":        func(q *QualityRequest) { q.Rules.Checks[0].TargetMs = fp(900) },
		"no max":                  func(q *QualityRequest) { q.Rules.Checks[0].MaxMs = nil },
		"percentile":              func(q *QualityRequest) { q.Rules.Checks[0].Percentile = 101 },
		"db target":               func(q *QualityRequest) { q.Rules.Checks[1].TargetPer = ip(4) },
		"db no max":               func(q *QualityRequest) { q.Rules.Checks[1].MaxPerExecution = nil },
		"forbidden no to":         func(q *QualityRequest) { q.Rules.Checks[2].To = "" },
		"forbidden from=to":       func(q *QualityRequest) { q.Rules.Checks[2].From = "directory-api" },
		"negative min":            func(q *QualityRequest) { q.Rules.Checks[0].MinExecutions = -1 },
		"max_ms on db_calls":      func(q *QualityRequest) { q.Rules.Checks[1].MaxMs = fp(100) },
		"to on latency":           func(q *QualityRequest) { q.Rules.Checks[0].To = "x" },
		"percentile on forbidden": func(q *QualityRequest) { q.Rules.Checks[2].Percentile = 50 },
	} {
		q := ok()
		mutate(&q)
		if err := q.Validate(); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestQualityQuery(t *testing.T) {
	q := qualityQuery([]QualityEntry{*entryRS(), *entryRS(), {Service: "s", Operation: "o"}}, 10, 20)
	for _, want := range []string{
		`"ResourceAttributes"['troubleshooting.run']`,
		`= {qualityRun:String}`,
		`("ServiceName", "SpanName") IN [('recording-api','start_session'),('s','o')]`,
		`"UnixTime" >= 10 AND "UnixTime" < ` + num(20+int64(qualityLookahead)),
		`"UnixTime" >= 10 AND "UnixTime" < 20`,
		`LIMIT 1 BY "TraceId", "SpanId"`,
		`LIMIT ` + num(MaxQualitySpans+1),
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query is missing %q:\n%s", want, q)
		}
	}
	bound := map[string]bool{"qualityRun": true}
	for _, m := range namedPlaceholder.FindAllStringSubmatch(q, -1) {
		if !bound[m[1]] {
			t.Errorf("placeholder %q is not bound", m[1])
		}
	}
}

const referenceYAML = `version: 1
entry: {service: recording-api, operation: start_session, span_kind: server}
checks:
  - {id: startup-p95, type: latency, max_ms: 800, target_ms: 500}
  - {id: db-calls, type: db_calls, max_per_execution: 3, target_per_execution: 2}
  - {id: no-directory, type: forbidden_call, to: directory-api}
`

// The UI sends the rules file as text; it is read as strictly as the CLI does.
func TestQualityRulesYAML(t *testing.T) {
	req := QualityRequest{RulesYAML: referenceYAML, Run: "r", TimeFrom: num(qBase), TimeTo: num(qBase + 1)}
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	if req.RulesYAML != "" || len(req.Rules.Checks) != 3 || *req.Rules.Checks[0].MaxMs != 800 || *req.Rules.Checks[1].TargetPer != 2 ||
		req.Rules.Checks[2].To != "directory-api" || req.Rules.Checks[2].Entry.SpanKind != "server" {
		t.Errorf("parsed: %+v", req.Rules)
	}
	for name, text := range map[string]string{
		"unknown key":   strings.Replace(referenceYAML, "target_ms", "targt_ms", 1),
		"duplicate key": strings.Replace(referenceYAML, "max_ms: 800,", "max_ms: 800, max_ms: 900,", 1),
		"two documents": referenceYAML + "---\nversion: 1\n",
		"not yaml":      "version: [1",
		"too large":     referenceYAML + "#" + strings.Repeat("x", MaxQualityRulesBytes),
	} {
		q := QualityRequest{RulesYAML: text, Run: "r", TimeFrom: num(qBase), TimeTo: num(qBase + 1)}
		if err := q.Validate(); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	both := QualityRequest{RulesYAML: referenceYAML, Rules: referenceRules(), Run: "r", TimeFrom: num(qBase), TimeTo: num(qBase + 1)}
	if err := both.Validate(); err == nil || !strings.Contains(err.Error(), "not both") {
		t.Errorf("rules and rules_yaml: %v", err)
	}
}
