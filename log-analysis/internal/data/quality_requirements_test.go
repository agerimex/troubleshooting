package data

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

const requirementsYAML = `version: 1
project: recording
entry: {service: recording-api, operation: start_session, span_kind: server}
checks:
  - {id: startup-p95, type: latency, max_ms: 800, target_ms: 500}
  - {id: db-calls, type: db_calls, max_per_execution: 3}
  - {id: no-directory, type: forbidden_call, to: directory-api}
requirements:
  - id: REQ-START
    title: A session starts within 800 ms
    owner: recording-team
    why: Users abandon a start that takes longer than a second.
    checks: [startup-p95, db-calls]
  - id: REQ-DIRECTORY
    title: Starting a session never waits for the directory
    checks: [no-directory]
  - id: REQ-OUTAGE
    title: Recording survives an analytics outage
    owner: recording-team
`

func requirementsRequest(t *testing.T, text string) QualityRequest {
	t.Helper()
	req := QualityRequest{RulesYAML: text, Run: "candidate", TimeFrom: num(qBase), TimeTo: num(qBase + int64(time.Hour))}
	if err := req.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	return req
}

func requirementByID(t *testing.T, res QualityResult, id string) QualityRequirementResult {
	t.Helper()
	for _, r := range res.Requirements {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no requirement %s", id)
	return QualityRequirementResult{}
}

// A requirement's status comes from its own checks; one without a check is
// listed but never met.
func TestQualityRequirementStatuses(t *testing.T) {
	req := requirementsRequest(t, requirementsYAML)
	if req.Rules.Project != "recording" {
		t.Errorf("project %q", req.Rules.Project)
	}
	cand := buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "candidate", 910)}, runSpans{}, time.Unix(0, qBase))
	if cand.Project != "recording" || len(cand.Requirements) != 3 {
		t.Fatalf("project %q, requirements %+v", cand.Project, cand.Requirements)
	}
	start := requirementByID(t, cand, "REQ-START")
	if start.Status != RequirementViolated || start.Summary != "failed: startup-p95, db-calls" || start.Owner != "recording-team" ||
		!strings.HasPrefix(start.Why, "Users abandon") || strings.Join(start.Checks, ",") != "startup-p95,db-calls" {
		t.Errorf("REQ-START: %+v", start)
	}
	if r := requirementByID(t, cand, "REQ-OUTAGE"); r.Status != RequirementNotEvaluated || r.Summary != "no check enforces it yet" || r.Checks == nil {
		t.Errorf("REQ-OUTAGE: %+v", r)
	}

	fixed := buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "fixed", 560)}, runSpans{}, time.Unix(0, qBase))
	if r := requirementByID(t, fixed, "REQ-START"); r.Status != RequirementMet || r.Summary != "all 2 of its checks passed" {
		t.Errorf("fixed REQ-START: %+v", r)
	}
	if r := requirementByID(t, fixed, "REQ-DIRECTORY"); r.Status != RequirementMet || r.Summary != "its check passed" {
		t.Errorf("fixed REQ-DIRECTORY: %+v", r)
	}
	// The gate is decided by the checks, as before.
	if fixed.Gate != GatePassed || cand.Gate != GateFailed {
		t.Errorf("gates %s %s", fixed.Gate, cand.Gate)
	}

	none := buildQualityResult(req, runSpans{Problem: "outside_retention", Detail: "gone"}, runSpans{}, time.Unix(0, qBase))
	if r := requirementByID(t, none, "REQ-START"); r.Status != RequirementInsufficient || r.Summary != "not enough data: startup-p95, db-calls" {
		t.Errorf("no data REQ-START: %+v", r)
	}

	// A failure and missing data together: violated, and the missing data is
	// still named.
	mixed := requirementResults([]QualityRequirement{{ID: "R", Title: "t", Checks: []string{"a", "b"}}},
		[]QualityCheckResult{{ID: "a", Status: CheckInsufficient}, {ID: "b", Status: CheckFail}})
	if mixed[0].Status != RequirementViolated || mixed[0].Summary != "failed: b; not enough data: a" {
		t.Errorf("mixed: %+v", mixed[0])
	}

	// Without requirements the result still has an (empty) list.
	plain := buildQualityResult(validRequest(t, referenceRules(), "candidate", ""), runSpans{Spans: referenceRun("candidate", "fixed", 560)}, runSpans{}, time.Unix(0, qBase))
	b, _ := json.Marshal(plain)
	if !strings.Contains(string(b), `"requirements":[]`) || plain.Project != "recording-api" {
		t.Errorf("no requirements: project %q, %s", plain.Project, b[:120])
	}
}

func TestQualityRequirementValidation(t *testing.T) {
	for name, tc := range map[string]struct{ edit, want string }{
		"unknown check": {"checks: [no-directory]", `check "no-directorie" is not in the rules`},
		"twice":         {"checks: [no-directory]", "check no-directory is listed twice"},
		"no title":      {"title: Starting a session never waits for the directory", "requirement REQ-DIRECTORY: title is required"},
		"duplicate id":  {"id: REQ-DIRECTORY", "requirement REQ-START: duplicate id"},
		"bad id":        {"id: REQ-OUTAGE", "requirement 3: id must be"},
		"unknown key":   {"owner: recording-team\n    why:", "field ownr not found"},
		"bad project":   {"project: recording", "project must be"},
		"long title":    {"title: Recording survives an analytics outage", "title is longer than 200 characters"},
	} {
		text := requirementsYAML
		switch name {
		case "unknown check":
			text = strings.Replace(text, tc.edit, "checks: [no-directorie]", 1)
		case "twice":
			text = strings.Replace(text, tc.edit, "checks: [no-directory, no-directory]", 1)
		case "no title":
			text = strings.Replace(text, tc.edit, "title: ''", 1)
		case "duplicate id":
			text = strings.Replace(text, tc.edit, "id: REQ-START", 1)
		case "bad id":
			text = strings.Replace(text, tc.edit, "id: 'REQ OUTAGE'", 1)
		case "unknown key":
			text = strings.Replace(text, "owner: recording-team\n    why:", "ownr: recording-team\n    why:", 1)
		case "bad project":
			text = strings.Replace(text, tc.edit, "project: 'two words'", 1)
		case "long title":
			text = strings.Replace(text, tc.edit, "title: "+strings.Repeat("x", 201), 1)
		}
		q := QualityRequest{RulesYAML: text, Run: "r", TimeFrom: num(qBase), TimeTo: num(qBase + 1)}
		err := q.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
	// The project defaults to the first check's entry when there is no
	// top-level entry.
	rules := QualityRules{Version: 1, Checks: []QualityCheck{{ID: "a", Type: CheckForbidden, To: "x", Entry: &QualityEntry{Service: "svc", Operation: "op"}}}}
	if req := validRequest(t, rules, "r", ""); req.Rules.Project != "svc" {
		t.Errorf("default project %q", req.Rules.Project)
	}
}

func TestQualitySourceValidation(t *testing.T) {
	req := QualityRequest{RulesYAML: requirementsYAML, Run: "r", TimeFrom: num(qBase), TimeTo: num(qBase + 1), Record: true,
		Source: &QualitySource{Commit: " abc ", Ref: "main"}}
	if err := req.Validate(); err != nil || req.Source.Commit != "abc" || req.rulesText != requirementsYAML {
		t.Errorf("valid source: %v %+v", err, req.Source)
	}
	bad := QualityRequest{RulesYAML: requirementsYAML, Run: "r", TimeFrom: num(qBase), TimeTo: num(qBase + 1), Source: &QualitySource{Ref: "a\nb"}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "source.ref must be one line") {
		t.Errorf("multiline ref: %v", err)
	}
	// Recording keeps the exact rules text, so it needs rules_yaml.
	noText := QualityRequest{Rules: referenceRules(), Run: "r", TimeFrom: num(qBase), TimeTo: num(qBase + 1), Record: true}
	if err := noText.Validate(); err == nil || !strings.Contains(err.Error(), "record needs rules_yaml") {
		t.Errorf("record without rules_yaml: %v", err)
	}
}

// A recorded evaluation keeps each evidence trace's spans from the spans it
// read, in result.json's snapshot format.
func TestQualityRecordSnapshots(t *testing.T) {
	req := requirementsRequest(t, requirementsYAML)
	spans := referenceRun("candidate", "candidate", 910)
	for i := range spans {
		if spans[i].DB {
			spans[i].DBSystem = "postgresql"
		}
		if spans[i].Name == "GET /users" {
			spans[i].PeerService = "directory-api"
		}
	}
	plain := buildQualityResult(req, runSpans{Spans: spans}, runSpans{}, time.Unix(0, qBase))
	if checkByID(t, plain, "db-calls").Evidence[0].Snapshot != nil {
		t.Fatal("snapshot without record")
	}
	req.Record = true
	res := buildQualityResult(req, runSpans{Spans: spans}, runSpans{}, time.Unix(0, qBase))
	db := checkByID(t, res, "db-calls").Evidence[0]
	if db.Snapshot == nil || len(db.Snapshot.Spans) == 0 || db.Snapshot.Truncated {
		t.Fatalf("db-calls snapshot: %+v", db.Snapshot)
	}
	first := db.Snapshot.Spans[0]
	if first.SpanID != spanHex(1) || first.Name != "start_session" || first.Status != "unset" || first.ParentID != RootParentSpanID {
		t.Errorf("first span: %+v", first)
	}
	lookups := 0
	for _, s := range db.Snapshot.Spans {
		if s.Name == "permission_lookup" {
			lookups++
			if s.Peer != "postgresql" || s.Kind != "client" {
				t.Errorf("lookup: %+v", s)
			}
		}
	}
	if lookups != 20 {
		t.Errorf("%d lookups in the snapshot", lookups)
	}
	dir := checkByID(t, res, "no-directory").Evidence[0]
	found := false
	for _, s := range dir.Snapshot.Spans {
		found = found || (s.Name == "GET /users" && s.Peer == "directory-api")
	}
	if !found {
		t.Errorf("directory call not in the snapshot: %+v", dir.Snapshot.Spans)
	}
}

func TestQualityEvaluationID(t *testing.T) {
	req := requirementsRequest(t, requirementsYAML)
	req.Source = &QualitySource{Commit: "abc", CIRun: "https://ci/1/attempts/1"}
	a := qualityEvaluationID(req, "sha")
	if !ValidQualityEvaluationID(a) || a != qualityEvaluationID(req, "sha") {
		t.Fatalf("id %q is not stable", a)
	}
	req.Source.CIRun = "https://ci/1/attempts/2"
	if qualityEvaluationID(req, "sha") == a {
		t.Error("a new CI attempt must be a new record")
	}
	if qualityEvaluationID(req, "other") == qualityEvaluationID(req, "sha") {
		t.Error("other rules must be a new record")
	}
}

func historySummary(id, commit, sha string, statuses ...string) QualityEvaluationSummary {
	s := QualityEvaluationSummary{ID: id, RecordedAt: "t-" + id, Source: QualitySource{Commit: commit, Ref: "main", RulesSHA256: sha}}
	for i, st := range statuses {
		if st != "" {
			s.Requirements = append(s.Requirements, QualityStatusRef{ID: []string{"REQ-START", "REQ-OUTAGE"}[i], Status: st})
		}
	}
	return s
}

func TestQualityCatalogue(t *testing.T) {
	req := requirementsRequest(t, requirementsYAML)
	latest := buildQualityResult(req, runSpans{Spans: referenceRun("candidate", "candidate", 910)}, runSpans{}, time.Unix(0, qBase))
	summaries := []QualityEvaluationSummary{
		historySummary("e4", "c4", "s2", "violated", "not_evaluated"),
		historySummary("e3", "c3", "s2", "violated", "not_evaluated"),
		historySummary("e2", "c2", "s1", "met", ""),
		historySummary("e1", "c1", "s1", "met", ""),
	}
	cat := buildCatalogue("recording", "main", latest, summaries)
	if cat.Latest == nil || cat.Latest.ID != "e4" || len(cat.Requirements) != 3 || len(cat.Checks) != 3 {
		t.Fatalf("catalogue: %+v", cat)
	}
	start := cat.Requirements[0]
	if start.ID != "REQ-START" || start.Status != RequirementViolated || len(start.History) != 4 {
		t.Fatalf("REQ-START: %+v", start)
	}
	if s := start.Streak; s.Evaluations != 2 || s.Since.EvaluationID != "e3" || s.Since.Commit != "c3" || s.StatusBefore != "met" || s.AtLeast {
		t.Errorf("REQ-START streak: %+v", s)
	}
	// REQ-DIRECTORY wasn't in any summary: absent all along.
	dir := cat.Requirements[1]
	if dir.History[0].Status != "absent" || !dir.Streak.AtLeast || dir.Streak.Evaluations != 4 {
		t.Errorf("REQ-DIRECTORY: %+v", dir)
	}
	outage := cat.Requirements[2]
	if outage.History[2].Status != "absent" || outage.Streak.StatusBefore != "absent" || outage.Streak.Evaluations != 2 {
		t.Errorf("REQ-OUTAGE: %+v", outage)
	}
	if strings.Join(cat.Unlinked, ",") != "" {
		t.Errorf("unlinked %v", cat.Unlinked)
	}
	empty := buildCatalogue("recording", "", QualityResult{}, nil)
	if empty.Latest != nil || empty.Requirements == nil || empty.Checks == nil || empty.Unlinked == nil {
		t.Errorf("empty catalogue: %+v", empty)
	}
	// Checks without a requirement are listed.
	plain := buildQualityResult(validRequest(t, referenceRules(), "candidate", ""), runSpans{Spans: referenceRun("candidate", "fixed", 560)}, runSpans{}, time.Unix(0, qBase))
	if c := buildCatalogue("p", "", plain, summaries[:1]); strings.Join(c.Unlinked, ",") != "startup-p95,db-calls,no-directory" {
		t.Errorf("unlinked: %v", c.Unlinked)
	}
}

func TestQualityHistoryFilter(t *testing.T) {
	get := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	f, err := ParseQualityHistoryFilter(get(map[string]string{"project": "recording", "ref": "main", "before": "5", "limit": "10"}), true)
	if err != nil || f.Project != "recording" || f.Ref != "main" || f.Before != 5 || f.Limit != 10 {
		t.Errorf("filter %+v %v", f, err)
	}
	if f, _ := ParseQualityHistoryFilter(get(nil), false); f.Limit != DefaultQualityHistory {
		t.Errorf("default limit %d", f.Limit)
	}
	for name, m := range map[string]map[string]string{
		"no project":  {},
		"bad project": {"project": "a b"},
		"bad before":  {"project": "p", "before": "x"},
		"bad limit":   {"project": "p", "limit": "500"},
		"bad ref":     {"project": "p", "ref": "a\nb"},
	} {
		if _, err := ParseQualityHistoryFilter(get(m), true); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// The rules files of this repository are valid for the API, requirements
// included (the CLI only checks their keys).
func TestQualityRepositoryRulesFiles(t *testing.T) {
	for _, path := range []string{"../../../quality-check/quality.example.yaml", "../../../examples/quality-gate/quality.yaml", "../../../.github/quality.yaml"} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		req := QualityRequest{RulesYAML: string(b), Run: "r", TimeFrom: num(qBase), TimeTo: num(qBase + 1)}
		if err := req.Validate(); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}
