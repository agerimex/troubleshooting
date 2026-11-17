package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Quality gate requirements and history (stage 5; docs/quality-gate.md):
// the reference runs evaluated with requirements and recorded as three
// commits of one project's main branch: baseline, candidate and fix. The
// project is unique per fixture run; recording is idempotent, so checking
// again (after -replay) finds the same three records.

func (c *checker) qgProject() string { return "fx" + c.d.run + "-recording" }

func (c *checker) qgRulesYAML() string {
	return fmt.Sprintf(`version: 1
project: %s
entry: {service: %s, operation: start_session, span_kind: server}
checks:
  - {id: startup-p95, type: latency, max_ms: 800, target_ms: 500}
  - {id: db-calls, type: db_calls, max_per_execution: 3, target_per_execution: 2}
  - {id: no-directory, type: forbidden_call, to: %s}
requirements:
  - id: REQ-START
    title: A recording session starts within 800 ms
    owner: recording-team
    why: Participants leave a session that takes more than a second to start.
    checks: [startup-p95, db-calls]
  - id: REQ-DIRECTORY
    title: Starting a session never waits for the directory
    checks: [no-directory]
  - id: REQ-OUTAGE
    title: Recording continues during an analytics outage
`, c.qgProject(), c.d.service("qg-api"), c.d.service("qg-directory"))
}

type qgRecorded struct {
	ID           string `json:"id"`
	RecordedAt   string `json:"recorded_at"`
	RecordError  string `json:"record_error"`
	Gate         string `json:"gate"`
	Project      string `json:"project"`
	Requirements []struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Summary string `json:"summary"`
	} `json:"requirements"`
	Checks []struct {
		ID       string `json:"id"`
		Evidence []struct {
			Snapshot *struct {
				Spans []struct {
					Name string `json:"name"`
				} `json:"spans"`
			} `json:"snapshot"`
		} `json:"evidence"`
	} `json:"checks"`
}

func (c *checker) qgRecord(ctx context.Context, run, commit string) (qgRecorded, error) {
	body := map[string]any{"rules_yaml": c.qgRulesYAML(), "run": run, "record": true,
		"time_from": ns(c.d.m2Hour(6)), "time_to": ns(c.d.m2Hour(6).Add(30 * time.Minute)),
		"source": map[string]any{"commit": commit, "ref": "main", "ci_run": "fixture " + c.d.run}}
	var data struct {
		Evaluation qgRecorded `json:"Evaluation"`
	}
	if err := c.api.post(ctx, "/api/v1/quality/evaluate", body, &data); err != nil {
		return qgRecorded{}, err
	}
	ev := data.Evaluation
	if ev.RecordError != "" || len(ev.ID) != 24 || ev.RecordedAt == "" {
		return ev, fmt.Errorf("%s: not recorded: id %q, error %q", run, ev.ID, ev.RecordError)
	}
	return ev, nil
}

func requirementStatuses(ev qgRecorded) string {
	var parts []string
	for _, r := range ev.Requirements {
		parts = append(parts, r.ID+"="+r.Status)
	}
	return strings.Join(parts, " ")
}

type qgCatalogue struct {
	Catalogue struct {
		Latest *struct {
			ID string `json:"id"`
		} `json:"latest"`
		Requirements []struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Owner   string `json:"owner"`
			History []struct {
				Status string `json:"status"`
			} `json:"history"`
			Streak struct {
				Evaluations  int    `json:"evaluations"`
				StatusBefore string `json:"status_before"`
				Since        struct {
					Commit string `json:"commit"`
				} `json:"since"`
			} `json:"streak"`
		} `json:"requirements"`
	} `json:"Catalogue"`
}

// QG-HIST-001: requirement statuses follow their checks (one without a
// check is listed, not evaluated); recorded evaluations form the project's
// history; the catalogue shows each requirement's history and since when its
// status holds; a record keeps its rules and evidence snapshots; a retried
// request records once.
func (c *checker) qgHistory(ctx context.Context) error {
	steps := []struct{ run, commit, gate, reqs string }{
		{c.d.qgRun("baseline"), "c1" + c.d.run, "passed", "REQ-START=met REQ-DIRECTORY=met REQ-OUTAGE=not_evaluated"},
		{c.d.qgRun("candidate"), "c2" + c.d.run, "failed", "REQ-START=violated REQ-DIRECTORY=violated REQ-OUTAGE=not_evaluated"},
		{c.d.qgRun("fixed"), "c3" + c.d.run, "passed", "REQ-START=met REQ-DIRECTORY=met REQ-OUTAGE=not_evaluated"},
	}
	ids := make([]string, len(steps))
	var candidate qgRecorded
	for i, s := range steps {
		ev, err := c.qgRecord(ctx, s.run, s.commit)
		if err != nil {
			return err
		}
		if ev.Gate != s.gate || ev.Project != c.qgProject() || requirementStatuses(ev) != s.reqs {
			return fmt.Errorf("%s: gate %s, project %s, requirements %q, want %s and %q", s.commit, ev.Gate, ev.Project, requirementStatuses(ev), s.gate, s.reqs)
		}
		ids[i] = ev.ID
		if i == 1 {
			candidate = ev
		}
	}
	// The candidate's db-calls evidence keeps the whole execution: root,
	// check_permissions, 20 lookups, insert, client, directory.
	for _, ch := range candidate.Checks {
		if ch.ID == "db-calls" && (len(ch.Evidence) != 1 || ch.Evidence[0].Snapshot == nil || len(ch.Evidence[0].Snapshot.Spans) != 25) {
			return fmt.Errorf("db-calls snapshot: %+v", ch.Evidence)
		}
	}
	// Retried: the same record.
	again, err := c.qgRecord(ctx, steps[1].run, steps[1].commit)
	if err != nil {
		return err
	}
	if again.ID != ids[1] || again.RecordedAt != candidate.RecordedAt {
		return fmt.Errorf("retry recorded %s at %s, want %s at %s", again.ID, again.RecordedAt, ids[1], candidate.RecordedAt)
	}

	q := url.Values{"project": {c.qgProject()}}
	var list struct {
		Evaluations []struct {
			ID     string `json:"id"`
			Gate   string `json:"gate"`
			Source struct {
				Commit      string `json:"commit"`
				RulesSHA256 string `json:"rules_sha256"`
			} `json:"source"`
		} `json:"Evaluations"`
		Next string `json:"Next"`
	}
	if err := c.api.get(ctx, "/api/v1/quality/evaluations?"+q.Encode(), &list); err != nil {
		return err
	}
	var got []string
	for _, e := range list.Evaluations {
		got = append(got, e.ID+":"+e.Gate+":"+e.Source.Commit)
		if len(e.Source.RulesSHA256) != 64 {
			return fmt.Errorf("evaluation %s has no rules sha256", e.ID)
		}
	}
	want := []string{ids[2] + ":passed:" + steps[2].commit, ids[1] + ":failed:" + steps[1].commit, ids[0] + ":passed:" + steps[0].commit}
	if strings.Join(got, " ") != strings.Join(want, " ") || list.Next != "" {
		return fmt.Errorf("history %v (next %q), want %v", got, list.Next, want)
	}

	q.Set("ref", "main")
	var cat qgCatalogue
	if err := c.api.get(ctx, "/api/v1/quality/requirements?"+q.Encode(), &cat); err != nil {
		return err
	}
	k := cat.Catalogue
	if k.Latest == nil || k.Latest.ID != ids[2] || len(k.Requirements) != 3 {
		return fmt.Errorf("catalogue: %+v", k)
	}
	start := k.Requirements[0]
	var hist []string
	for _, h := range start.History {
		hist = append(hist, h.Status)
	}
	if start.ID != "REQ-START" || start.Status != "met" || start.Owner != "recording-team" || strings.Join(hist, ",") != "met,violated,met" ||
		start.Streak.Evaluations != 1 || start.Streak.StatusBefore != "violated" || start.Streak.Since.Commit != steps[2].commit {
		return fmt.Errorf("REQ-START: %+v", start)
	}
	if o := k.Requirements[2]; o.ID != "REQ-OUTAGE" || o.Status != "not_evaluated" || o.Streak.Evaluations != 3 {
		return fmt.Errorf("REQ-OUTAGE: %+v", o)
	}

	var rec struct {
		Record struct {
			Evaluation qgRecorded `json:"evaluation"`
			Rules      string     `json:"rules"`
		} `json:"Record"`
	}
	if err := c.api.get(ctx, "/api/v1/quality/evaluations/"+ids[1], &rec); err != nil {
		return err
	}
	if rec.Record.Evaluation.ID != ids[1] || rec.Record.Evaluation.Gate != "failed" || rec.Record.Rules != c.qgRulesYAML() {
		return fmt.Errorf("record %s: gate %s, rules kept %v", rec.Record.Evaluation.ID, rec.Record.Evaluation.Gate, rec.Record.Rules == c.qgRulesYAML())
	}
	var missing any
	if err := c.api.get(ctx, "/api/v1/quality/evaluations/000000000000000000000000", &missing); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		return fmt.Errorf("unknown evaluation: %v, want HTTP 404", err)
	}
	return nil
}
