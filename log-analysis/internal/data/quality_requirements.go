package data

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Requirements (quality gate, stage 5; docs/quality-gate.md): a named intent
// with an owner and a reason, enforced by checks of the same rules file. A
// requirement's status comes from its checks only:
//
//   - violated: one of its checks failed;
//   - insufficient: none failed, but one had not enough data;
//   - met: every one of its checks passed;
//   - not_evaluated: no check enforces it yet (listed, never met).
//
// Advisory checks count for the requirement as well: advisory only means the
// check doesn't decide the gate.
//
// Guardrails are checks that support a requirement without assessing it (a
// query budget protects a latency requirement, but four database calls don't
// show that a start was slow). They are listed with the requirement and still
// decide the gate like every check, but never its status.

// Requirement statuses.
const (
	RequirementMet          = "met"
	RequirementViolated     = "violated"
	RequirementInsufficient = "insufficient"
	RequirementNotEvaluated = "not_evaluated"
)

// Limits of the requirement fields.
const (
	MaxQualityRequirements = 50
	maxRequirementTitle    = 200
	maxRequirementOwner    = 100
	maxRequirementWhy      = 1000
)

// QualityRequirement is one requirement of the rules file.
type QualityRequirement struct {
	ID     string   `json:"id" yaml:"id"`
	Title  string   `json:"title" yaml:"title"`
	Owner  string   `json:"owner,omitempty" yaml:"owner,omitempty"`
	Why    string   `json:"why,omitempty" yaml:"why,omitempty"`
	Checks []string `json:"checks,omitempty" yaml:"checks,omitempty"`
	// Guardrails are checks that support the requirement without deciding
	// its status.
	Guardrails []string `json:"guardrails,omitempty" yaml:"guardrails,omitempty"`
}

// QualityRequirementResult is a requirement with its status in one evaluation.
type QualityRequirementResult struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Owner   string   `json:"owner,omitempty"`
	Why     string   `json:"why,omitempty"`
	Checks  []string `json:"checks"`
	Status  string   `json:"status"`
	Summary string   `json:"summary"`
	// Guardrails and their summary (empty without guardrails).
	Guardrails       []string `json:"guardrails,omitempty"`
	GuardrailSummary string   `json:"guardrail_summary,omitempty"`
	// Criteria is the sha256 of what decides the status (its checks'
	// definitions without ids and targets, and run.executions): a history
	// streak never spans different criteria.
	Criteria string `json:"criteria_sha256,omitempty"`
}

func textField(name, v string, limit int, required bool) (string, error) {
	v = strings.TrimSpace(v)
	if required && v == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	if utf8.RuneCountInString(v) > limit {
		return "", fmt.Errorf("%s is longer than %d characters", name, limit)
	}
	if strings.ContainsRune(v, 0) {
		return "", fmt.Errorf("%s contains a NUL character", name)
	}
	return v, nil
}

// validateRequirements checks the requirements of validated checks: unique
// ids, a title, and checks that exist (each at most once per requirement).
func validateRequirements(r *QualityRules) error {
	if len(r.Requirements) > MaxQualityRequirements {
		return invalid("rules have more than %d requirements", MaxQualityRequirements)
	}
	checks := map[string]bool{}
	for _, c := range r.Checks {
		checks[c.ID] = true
	}
	seen := map[string]bool{}
	for i := range r.Requirements {
		q := &r.Requirements[i]
		q.ID = strings.TrimSpace(q.ID)
		if !checkIDPattern.MatchString(q.ID) {
			return invalid("requirement %d: id must be 1-64 letters, digits, '_', '.' or '-'", i+1)
		}
		if seen[q.ID] {
			return invalid("requirement %s: duplicate id", q.ID)
		}
		seen[q.ID] = true
		var err error
		if q.Title, err = textField("title", q.Title, maxRequirementTitle, true); err != nil {
			return invalid("requirement %s: %s", q.ID, err.Error())
		}
		if q.Owner, err = textField("owner", q.Owner, maxRequirementOwner, false); err != nil {
			return invalid("requirement %s: %s", q.ID, err.Error())
		}
		if q.Why, err = textField("why", q.Why, maxRequirementWhy, false); err != nil {
			return invalid("requirement %s: %s", q.ID, err.Error())
		}
		own := map[string]bool{}
		for j, id := range q.Checks {
			id = strings.TrimSpace(id)
			q.Checks[j] = id
			if !checks[id] {
				return invalid("requirement %s: check %q is not in the rules", q.ID, id)
			}
			if own[id] {
				return invalid("requirement %s: check %s is listed twice", q.ID, id)
			}
			own[id] = true
		}
		guard := map[string]bool{}
		for j, id := range q.Guardrails {
			id = strings.TrimSpace(id)
			q.Guardrails[j] = id
			if !checks[id] {
				return invalid("requirement %s: guardrail %q is not in the rules", q.ID, id)
			}
			if own[id] {
				return invalid("requirement %s: check %s is both a check and a guardrail", q.ID, id)
			}
			if guard[id] {
				return invalid("requirement %s: guardrail %s is listed twice", q.ID, id)
			}
			guard[id] = true
		}
	}
	return nil
}

var projectPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_./-]{0,127}$`)

// validateProject checks the project name, or fills it in from the entry
// (the top-level one, else the first check's).
func validateProject(r *QualityRules) error {
	r.Project = strings.TrimSpace(r.Project)
	if r.Project == "" {
		if r.Entry != nil {
			r.Project = r.Entry.Service
		} else if len(r.Checks) > 0 && r.Checks[0].Entry != nil {
			r.Project = r.Checks[0].Entry.Service
		}
	}
	if !projectPattern.MatchString(r.Project) {
		return invalid("project must be 1-128 letters, digits, '_', '.', '/' or '-' (default: the entry's service)")
	}
	return nil
}

// requirementResults gives every requirement its status from the check
// results of the same evaluation.
func requirementResults(reqs []QualityRequirement, checks []QualityCheckResult) []QualityRequirementResult {
	byID := map[string]QualityCheckResult{}
	for _, c := range checks {
		byID[c.ID] = c
	}
	out := make([]QualityRequirementResult, 0, len(reqs))
	for _, q := range reqs {
		r := QualityRequirementResult{ID: q.ID, Title: q.Title, Owner: q.Owner, Why: q.Why, Checks: append([]string{}, q.Checks...)}
		r.Guardrails, r.GuardrailSummary = guardrailSummary(q.Guardrails, byID)
		var failed, noData []string
		for _, id := range q.Checks {
			switch byID[id].Status {
			case CheckFail:
				failed = append(failed, id)
			case CheckPass:
			default:
				noData = append(noData, id)
			}
		}
		switch {
		case len(q.Checks) == 0:
			r.Status, r.Summary = RequirementNotEvaluated, "no check enforces it yet"
		case len(failed) > 0:
			r.Status, r.Summary = RequirementViolated, "failed: "+strings.Join(failed, ", ")
			if len(noData) > 0 {
				r.Summary += "; not enough data: " + strings.Join(noData, ", ")
			}
		case len(noData) > 0:
			r.Status, r.Summary = RequirementInsufficient, "not enough data: "+strings.Join(noData, ", ")
		default:
			r.Status = RequirementMet
			r.Summary = "its check passed"
			if len(q.Checks) > 1 {
				r.Summary = fmt.Sprintf("all %d of its checks passed", len(q.Checks))
			}
		}
		out = append(out, r)
	}
	return out
}

// guardrailSummary: "guardrail failed: db-calls (it decides the gate, not
// this requirement)", or which passed.
func guardrailSummary(ids []string, byID map[string]QualityCheckResult) ([]string, string) {
	if len(ids) == 0 {
		return nil, ""
	}
	var failed, noData, passed []string
	for _, id := range ids {
		switch byID[id].Status {
		case CheckFail:
			failed = append(failed, id)
		case CheckPass:
			passed = append(passed, id)
		default:
			noData = append(noData, id)
		}
	}
	var parts []string
	if len(failed) > 0 {
		parts = append(parts, "guardrail failed: "+strings.Join(failed, ", "))
	}
	if len(noData) > 0 {
		parts = append(parts, "guardrail without enough data: "+strings.Join(noData, ", "))
	}
	if len(passed) > 0 {
		parts = append(parts, "guardrail passed: "+strings.Join(passed, ", "))
	}
	s := strings.Join(parts, "; ")
	if len(failed)+len(noData) > 0 {
		s += " (guardrails decide the gate, not this requirement)"
	}
	return append([]string{}, ids...), s
}

// setRequirementCriteria fills in each requirement's criteria hash from the
// validated rules: the definitions of its checks without their ids, targets
// and advisory flag (none of them changes a status), in a fixed order, and
// run.executions and run.child_counts. Guardrails don't decide the status and aren't part of it.
func setRequirementCriteria(res []QualityRequirementResult, rules QualityRules) {
	byID := map[string]QualityCheck{}
	for _, c := range rules.Checks {
		byID[c.ID] = c
	}
	executions, counts := 0, ""
	if rules.Run != nil {
		executions, counts = rules.Run.Executions, rules.Run.ChildCounts
	}
	for i := range res {
		if len(res[i].Checks) == 0 {
			continue
		}
		defs := make([]string, 0, len(res[i].Checks))
		for _, id := range res[i].Checks {
			c := byID[id]
			c.ID, c.TargetMs, c.TargetPer, c.Advisory = "", nil, nil, false
			b, _ := json.Marshal(c)
			defs = append(defs, string(b))
		}
		sort.Strings(defs)
		b, _ := json.Marshal(struct {
			Checks     []string `json:"checks"`
			Executions int      `json:"executions"`
			// Left out when not set, so criteria recorded before it stay equal.
			ChildCounts string `json:"child_counts,omitempty"`
		}{defs, executions, counts})
		sum := sha256.Sum256(b)
		res[i].Criteria = hex.EncodeToString(sum[:])
	}
}

// QualitySource says what was tested and where the rules came from (the CLI
// sends it from CI). It is recorded as given.
type QualitySource struct {
	Commit      string `json:"commit,omitempty"`
	PRHead      string `json:"pr_head,omitempty"`
	Ref         string `json:"ref,omitempty"`
	CIRun       string `json:"ci_run,omitempty"`
	Rules       string `json:"rules,omitempty"`
	RulesRef    string `json:"rules_ref,omitempty"`
	RulesSHA256 string `json:"rules_sha256,omitempty"`
}

func (s *QualitySource) validate() error {
	if s == nil {
		return nil
	}
	for _, f := range []struct {
		name string
		v    *string
	}{{"commit", &s.Commit}, {"pr_head", &s.PRHead}, {"ref", &s.Ref}, {"ci_run", &s.CIRun}, {"rules", &s.Rules}, {"rules_ref", &s.RulesRef}, {"rules_sha256", &s.RulesSHA256}} {
		v, err := textField("source."+f.name, *f.v, 512, false)
		if err != nil {
			return invalid("%s", err.Error())
		}
		if strings.ContainsAny(v, "\n\r") {
			return invalid("source.%s must be one line", f.name)
		}
		*f.v = v
	}
	return nil
}

// Evidence snapshots of a recorded evaluation: the spans of each evidence
// trace from the spans the evaluation already read (no further query), in the
// format of troubleshooting-check's result.json.

const maxSnapshotSpans = 200

// QualitySnapshot is the saved spans of one evidence trace.
type QualitySnapshot struct {
	Spans     []QualitySnapSpan `json:"spans,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
}

// QualitySnapSpan is one span of a snapshot.
type QualitySnapSpan struct {
	SpanID   string `json:"span_id"`
	ParentID string `json:"parent_id"`
	Service  string `json:"service"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Start    string `json:"start"`       // Unix nanoseconds
	Duration int64  `json:"duration_us"` // -1: invalid
	Status   string `json:"status"`
	Peer     string `json:"peer,omitempty"` // peer.service or db.system of a CLIENT span
}

var snapStatus = map[int8]string{0: "unset", 1: "error", 2: "ok"}

func addSnapshots(res *QualityResult, run runSpans) {
	byTrace := map[string][]*qSpan{}
	for i := range run.Spans {
		s := &run.Spans[i]
		byTrace[s.TraceID] = append(byTrace[s.TraceID], s)
	}
	made := map[string]*QualitySnapshot{}
	for i := range res.Checks {
		for j := range res.Checks[i].Evidence {
			e := &res.Checks[i].Evidence[j]
			key := res.Checks[i].Type + ":" + e.TraceID + ":" + e.SpanID
			if snap, ok := made[key]; ok {
				e.Snapshot = snap
				continue
			}
			spans := byTrace[e.TraceID]
			if len(spans) == 0 {
				continue
			}
			sort.SliceStable(spans, func(a, b int) bool {
				if spans[a].Start != spans[b].Start {
					return spans[a].Start < spans[b].Start
				}
				return spans[a].SpanID < spans[b].SpanID
			})
			ids := make([]string, len(spans))
			parents := make([]string, len(spans))
			for k, s := range spans {
				ids[k], parents[k] = s.SpanID, s.Parent
			}
			// A db_calls counterexample is its database calls: they are kept first.
			focus := []string{e.SpanID}
			if res.Checks[i].Type == CheckDBCalls {
				for _, s := range spans {
					if s.DB && s.Kind == "client" {
						focus = append(focus, s.SpanID)
					}
				}
			}
			keep, truncated := SnapshotSelection(ids, parents, focus, maxSnapshotSpans)
			snap := &QualitySnapshot{Truncated: truncated}
			for _, k := range keep {
				s := spans[k]
				sp := QualitySnapSpan{SpanID: s.SpanID, ParentID: s.Parent, Service: s.Service, Name: s.Name, Kind: s.Kind,
					Start: num(s.Start), Duration: s.Duration, Status: snapStatus[s.Status]}
				if s.Kind == "client" {
					sp.Peer = s.PeerService
					if sp.Peer == "" {
						sp.Peer = s.DBSystem
					}
				}
				if sp.Duration < 0 {
					sp.Duration = -1
				}
				snap.Spans = append(snap.Spans, sp)
			}
			made[key] = snap
			e.Snapshot = snap
		}
	}
}

// SnapshotSelection picks at most limit spans of one trace for a snapshot,
// so the counterexample survives in a large trace: first each focus span
// (the evidence span, then the spans that make the counterexample, e.g. the
// database calls of a db_calls check) with its ancestors, then the spans
// below the first focus, then the rest; each group in the given order (start
// order). It returns the chosen indexes in that order and whether spans were
// left out. troubleshooting-check's snapshots use the same rule.
func SnapshotSelection(ids, parents, focus []string, limit int) ([]int, bool) {
	index := map[string]int{}
	children := map[string][]int{}
	for i, id := range ids {
		if _, ok := index[id]; !ok {
			index[id] = i
		}
		children[parents[i]] = append(children[parents[i]], i)
	}
	chosen := make([]bool, len(ids))
	n := 0
	take := func(i int) {
		if !chosen[i] && n < limit {
			chosen[i] = true
			n++
		}
	}
	for _, id := range focus {
		f, ok := index[id]
		if !ok {
			continue
		}
		// The path from the focus up to the root, the focus first.
		for i, seen := f, map[int]bool{}; !seen[i]; {
			seen[i] = true
			take(i)
			p, ok := index[parents[i]]
			if !ok {
				break
			}
			i = p
		}
	}
	if len(focus) > 0 {
		if f, ok := index[focus[0]]; ok {
			// Below the evidence span: its whole subtree, in start order.
			below := make([]bool, len(ids))
			stack := []int{f}
			for len(stack) > 0 {
				i := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				for _, c := range children[ids[i]] {
					if !below[c] && c != f {
						below[c] = true
						stack = append(stack, c)
					}
				}
			}
			for i := range ids {
				if below[i] {
					take(i)
				}
			}
		}
	}
	for i := range ids {
		take(i)
	}
	out := make([]int, 0, n)
	for i := range ids {
		if chosen[i] {
			out = append(out, i)
		}
	}
	return out, len(ids) > limit
}
