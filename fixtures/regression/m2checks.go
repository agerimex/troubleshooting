package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	sender "github.com/agerimex/troubleshooting/log-sender"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// Milestone 2 checks. Expected values are literals from
// REFERENCE_FIXTURE.md and the feature files, never computed with the API's
// own queries. Percentiles use the fixture tolerance of 1 ms.

const msTol = 1.0

func (c *checker) refPop(ri int) population {
	rel := releases[ri]
	from := c.d.m2T().Add(rel.offset)
	return population{Service: c.d.service("recording-worker"), Operation: sp("upload_recording"), SpanKind: sp("consumer"),
		Environment: sp(m2Env), Version: sp(rel.version), Installation: sp(m2Installation), Segment: sp(m2Segment),
		TimeFrom: ns(from), TimeTo: ns(from.Add(time.Hour))}
}

// small is a population of one small case in [from, to).
func (c *checker) small(service, op string, from, to time.Time) population {
	p := population{Service: c.d.service(service), TimeFrom: ns(from), TimeTo: ns(to)}
	if op != "" {
		p.Operation, p.SpanKind = sp(op), sp("server")
	}
	return p
}

func (c *checker) h5(service, op string) population {
	return c.small(service, op, c.d.m2Hour(5), c.d.m2Hour(6))
}

func expectInt(what string, got, want int) error {
	if got != want {
		return fmt.Errorf("%s = %d, want %d", what, got, want)
	}
	return nil
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func expectPercentiles(what string, s apiCallStats, p50, p95, p99 float64) error {
	for name, want := range map[string]float64{"p50": p50, "p95": p95, "p99": p99} {
		if err := near(what+" "+name, s.Percentiles[name].ValueMs, want, msTol); err != nil {
			return err
		}
	}
	return nil
}

func expectClass(m apiMetric, want string) error {
	if m.Classification != want {
		return fmt.Errorf("%s classified %s, want %s (reasons %v)", m.Metric, m.Classification, want, m.Reasons)
	}
	return nil
}

// m2Ready waits until the reference releases are queryable.
func (c *checker) m2Ready(ctx context.Context, o options) error {
	return waitFor(ctx, "the milestone 2 data", o.readyTimeout, func() error {
		for ri := range releases {
			s, err := c.api.stats(ctx, c.refPop(ri), false, false)
			if err != nil {
				return err
			}
			row, err := s.only()
			if err == nil && row.Calls != 1000 {
				err = fmt.Errorf("release %s: %d calls", releases[ri].version, row.Calls)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// M2-REF-001, M2-CMP-001: the regression 1.0.0 -> 1.1.0 and its evidence.
func (c *checker) m2Regression(ctx context.Context) error {
	a, err := c.api.compare(ctx, compareRequest{Baseline: c.refPop(0), Candidate: c.refPop(1)})
	if err != nil {
		return err
	}
	if len(a.Rows) != 1 || a.Rows[0].Baseline == nil || a.Rows[0].Candidate == nil {
		return fmt.Errorf("want one row with both populations, got %d rows", len(a.Rows))
	}
	b, cand := *a.Rows[0].Baseline, *a.Rows[0].Candidate
	if err := firstErr(expectInt("baseline calls", b.Calls, 1000), expectInt("candidate calls", cand.Calls, 1000),
		expectPercentiles("baseline", b, 100, 100, 100), expectPercentiles("candidate", cand, 160, 160, 160),
		near("baseline rate", &b.RatePerMinute, 16.6667, 0.001), near("candidate rate", &cand.RatePerMinute, 16.6667, 0.001)); err != nil {
		return err
	}
	p95, err := a.metric(0, "p95")
	if err != nil {
		return err
	}
	ef, err := a.metric(0, "error_fraction")
	if err != nil {
		return err
	}
	if err := firstErr(near("p95 difference", p95.Absolute, 60, msTol), near("p95 relative", p95.Relative, 60, 1), expectClass(p95, "regression_signal"),
		near("baseline error fraction", ef.Baseline, 1, 1e-9), near("candidate error fraction", ef.Candidate, 5, 1e-9),
		near("error fraction difference (pp)", ef.Absolute, 4, 1e-9), near("error fraction relative", ef.Relative, 400, 1e-6),
		expectClass(ef, "regression_signal")); err != nil {
		return err
	}
	if !strings.Contains(a.Interpretation, "not proof") {
		return fmt.Errorf("the analysis doesn't say that a signal is not proof of a cause: %q", a.Interpretation)
	}
	// Error groups: each source kind separately, 10 -> 50, never added up.
	want := map[string]func(apiErrorGroup) bool{
		"exception":   func(g apiErrorGroup) bool { return g.ExceptionType == "StorageError" && g.Frame == "storage.Upload" },
		"span_status": func(g apiErrorGroup) bool { return g.Operation == "upload_recording" },
		"log":         func(g apiErrorGroup) bool { return g.Signature == "Storage returned 403 AccessDenied" },
	}
	for kind, match := range want {
		found := false
		for _, g := range a.ErrorGroups {
			if g.SourceKind != kind || g.Service != c.d.service("recording-worker") || !match(g.apiErrorGroup) {
				continue
			}
			found = true
			if g.Baseline.Occurrences == nil || *g.Baseline.Occurrences != 10 || g.Candidate.Occurrences == nil || *g.Candidate.Occurrences != 50 {
				return fmt.Errorf("%s group: baseline %v, candidate %v occurrences, want 10 and 50", kind, g.Baseline.Occurrences, g.Candidate.Occurrences)
			}
			if g.Label != "recurring" {
				return fmt.Errorf("%s group labeled %s, want recurring", kind, g.Label)
			}
		}
		if !found {
			return fmt.Errorf("no %s group in the comparison", kind)
		}
	}
	c.state.M2Regression = a.ID
	c.state.M2RegressionGenerated = a.GeneratedAt
	return c.state.save(c.statePath)
}

// M2-REF-001 (investigation): a failed candidate upload opens its trace, with
// the API parent, the storage dependency and the 403 log, which is recorded
// nowhere in the spans.
func (c *checker) m2RegressionEvidence(ctx context.Context) error {
	groups, err := c.api.errorGroups(ctx, c.refPop(1))
	if err != nil {
		return err
	}
	exc := groups.find("exception", func(g apiErrorGroup) bool { return g.Frame == "storage.Upload" })
	if len(exc) != 1 {
		return fmt.Errorf("%d StorageError groups in 1.1.0, want 1", len(exc))
	}
	if exc[0].Occurrences != 50 || exc[0].AffectedTraces != 50 {
		return fmt.Errorf("StorageError group: %d occurrences, %d traces; want 50 and 50", exc[0].Occurrences, exc[0].AffectedTraces)
	}
	ex, err := c.api.examples(ctx, examplesRequest{Population: c.refPop(1), Kind: "error", Group: refOf(exc[0]), Limit: 3})
	if err != nil {
		return err
	}
	if ex.Status != "ok" || ex.Shown != 3 || ex.Total != 50 || !ex.Truncated {
		return fmt.Errorf("examples: status %s, %d shown of %d, truncated %v; want 3 of 50, truncated", ex.Status, ex.Shown, ex.Total, ex.Truncated)
	}
	e := ex.Examples[0]
	p := c.refPop(1)
	r, err := c.api.report(ctx, reportRequest{TraceID: e.TraceID, TimeFrom: p.TimeFrom, TimeTo: p.TimeTo})
	if err != nil {
		return err
	}
	if err := reportOK(r); err != nil {
		return err
	}
	if err := expectNames("report spans", r.Spans.Records, "start_recording", "upload_recording", "storage.PutObject"); err != nil {
		return err
	}
	var denied *apiLog
	for i, l := range r.Logs.Records {
		if l.Message == "Storage returned 403 AccessDenied" {
			denied = &r.Logs.Records[i]
		}
	}
	if denied == nil || denied.SpanID != idChild1 {
		return fmt.Errorf("the report has no 403 log associated with the upload span")
	}
	for _, s := range r.Spans.Records {
		text := s.StatusMessage
		for _, t := range s.Tags {
			text += " " + t["value"]
		}
		for _, ev := range s.Events {
			for _, v := range ev.Attributes {
				text += " " + v
			}
		}
		if strings.Contains(text, "403") || strings.Contains(text, "AccessDenied") {
			return fmt.Errorf("span %s records the 403 detail; the fixture keeps it in the log only", s.Name)
		}
	}
	return nil
}

// M2-REF-002, M2-CMP-002: the fix 1.1.1 against 1.1.0 and the original baseline.
func (c *checker) m2Fix(ctx context.Context) error {
	a, err := c.api.compare(ctx, compareRequest{Baseline: c.refPop(1), Candidate: c.refPop(2)})
	if err != nil {
		return err
	}
	if len(a.Rows) != 1 || a.Rows[0].Candidate == nil {
		return fmt.Errorf("want one row")
	}
	if err := expectPercentiles("1.1.1", *a.Rows[0].Candidate, 105, 105, 105); err != nil {
		return err
	}
	p95, _ := a.metric(0, "p95")
	ef, _ := a.metric(0, "error_fraction")
	if err := firstErr(near("p95 difference", p95.Absolute, -55, msTol), near("p95 relative", p95.Relative, -34.375, 0.7),
		expectClass(p95, "improvement_signal"), near("error fraction", ef.Candidate, 1, 1e-9), near("error fraction difference", ef.Absolute, -4, 1e-9),
		near("error fraction relative", ef.Relative, -80, 1e-6), expectClass(ef, "improvement_signal")); err != nil {
		return err
	}
	a, err = c.api.compare(ctx, compareRequest{Baseline: c.refPop(0), Candidate: c.refPop(2)})
	if err != nil {
		return err
	}
	p95, _ = a.metric(0, "p95")
	ef, _ = a.metric(0, "error_fraction")
	if err := firstErr(near("p95 difference", p95.Absolute, 5, msTol), near("p95 relative", p95.Relative, 5, 1), expectClass(p95, "within_threshold"),
		near("error fraction difference", ef.Absolute, 0, 1e-9), expectClass(ef, "within_threshold")); err != nil {
		return err
	}
	if c.state.M2Regression == "" {
		return fmt.Errorf("the regression comparison was not saved")
	}
	orig, err := c.api.analysis(ctx, c.state.M2Regression)
	if err != nil {
		return fmt.Errorf("the original regression report is no longer available: %w", err)
	}
	if orig.ID != c.state.M2Regression || orig.GeneratedAt != c.state.M2RegressionGenerated {
		return fmt.Errorf("the original regression report changed")
	}
	return nil
}

// M2-CMP-006: an environment mismatch makes conclusions unavailable but keeps
// the statistics.
func (c *checker) m2Mismatch(ctx context.Context) error {
	cand := c.refPop(1)
	cand.Environment = sp("test")
	a, err := c.api.compare(ctx, compareRequest{Baseline: c.refPop(0), Candidate: cand})
	if err != nil {
		return err
	}
	if len(a.Mismatches) == 0 || a.Mismatches[0].Dimension != "environment" {
		return fmt.Errorf("mismatches %v, want environment", a.Mismatches)
	}
	for _, name := range []string{"p95", "error_fraction"} {
		m, err := a.metric(0, name)
		if err != nil {
			return err
		}
		if err := expectClass(m, "incomparable"); err != nil {
			return err
		}
	}
	if len(a.Baseline.Rows) != 1 || a.Baseline.Rows[0].Calls != 1000 {
		return fmt.Errorf("the baseline statistics are not inspectable")
	}
	return nil
}

// M2-CAL-001: calls of the selected SERVER operation only.
func (c *checker) m2Kinds(ctx context.Context) error {
	s, err := c.api.stats(ctx, c.h5("m2-kinds", "start_recording"), false, false)
	if err != nil {
		return err
	}
	row, err := s.only()
	if err != nil {
		return err
	}
	if err := firstErr(expectInt("calls", row.Calls, 5), expectInt("errors", row.Errors, 2), expectInt("ok", row.Ok, 2),
		expectInt("unset", row.Unset, 1), near("error fraction", row.ErrorFraction, 0.4, 1e-9)); err != nil {
		return err
	}
	all, err := c.api.stats(ctx, c.h5("m2-kinds", ""), false, false)
	if err != nil {
		return err
	}
	got := map[string]int{}
	for _, r := range all.Rows {
		got[r.Operation+"/"+r.SpanKind] = r.Calls
	}
	if len(got) != 2 || got["start_recording/server"] != 5 || got["db.query/client"] != 12 {
		return fmt.Errorf("rows by operation and kind %v, want start_recording/server 5 and db.query/client 12, never summed", got)
	}
	return nil
}

// M2-CAL-002: versions and environments, unknown version explicit.
func (c *checker) m2Versions(ctx context.Context) error {
	p := c.h5("m2-env", "handle")
	p.Environment = sp("production")
	s, err := c.api.stats(ctx, p, true, false)
	if err != nil {
		return err
	}
	got := map[string]int{}
	for _, r := range s.Rows {
		if r.Version == nil {
			return fmt.Errorf("a row without version group")
		}
		got[*r.Version] = r.Calls
	}
	if len(got) != 3 || got["1.0.0"] != 2 || got["1.1.0"] != 3 || got[""] != 1 {
		return fmt.Errorf("production by version %v, want 1.0.0: 2, 1.1.0: 3, not recorded: 1", got)
	}
	return nil
}

// M2-CAL-003: [T, T+120s) holds two of three calls; rate per full window.
func (c *checker) m2Window(ctx context.Context) error {
	s0 := c.d.m2WindowStart()
	s, err := c.api.stats(ctx, c.small("m2-window", "tick", s0, s0.Add(120*time.Second)), false, false)
	if err != nil {
		return err
	}
	row, err := s.only()
	if err != nil {
		return err
	}
	return firstErr(expectInt("calls", row.Calls, 2), near("rate per minute", &row.RatePerMinute, 1, 1e-9),
		near("window seconds", &s.DurationSeconds, 120, 1e-9))
}

// M2-CAL-004: one span id in two traces is two calls, also after replays.
func (c *checker) m2Collide(ctx context.Context) error {
	s, err := c.api.stats(ctx, c.h5("m2-collide", "collide"), false, false)
	if err != nil {
		return err
	}
	row, err := s.only()
	if err != nil {
		return err
	}
	return firstErr(expectInt("calls", row.Calls, 2), expectInt("valid durations", row.ValidDurations, 2))
}

// M2-CAL-006: a sampled source in a collection gap.
func (c *checker) m2Sampled(ctx context.Context) error {
	s, err := c.api.stats(ctx, c.small("m2-sampled", "tick", c.d.m2Hour(-1), c.d.m2Hour(0)), false, true)
	if err != nil {
		return err
	}
	row, err := s.only()
	if err != nil {
		return err
	}
	if err := expectInt("calls", row.Calls, 200); err != nil {
		return err
	}
	if s.Coverage.Status != "gap" {
		return fmt.Errorf("coverage %s, want gap", s.Coverage.Status)
	}
	if n, ok := s.notice("sampling"); !ok || !strings.Contains(n.Message, "head:0.2") {
		return fmt.Errorf("the sampling policy is not shown: %v", s.Notices)
	}
	if len(s.Series) != 1 || s.Series[0].Available {
		return fmt.Errorf("the gap bucket is shown as a confirmed value")
	}
	return nil
}

// M2-CAL-007: two payloads of one identity count once, with a conflict notice.
func (c *checker) m2Conflict(ctx context.Context) error {
	s, err := c.api.stats(ctx, c.h5("m2-conflict", "conflict"), false, false)
	if err != nil {
		return err
	}
	row, err := s.only()
	if err != nil {
		return err
	}
	if err := firstErr(expectInt("calls", row.Calls, 1), expectInt("conflicts", row.Conflicts, 1)); err != nil {
		return err
	}
	if _, ok := s.notice("conflict"); !ok {
		return fmt.Errorf("no conflict notice")
	}
	return nil
}

// M2-CAL-008: 100, 0, then a coverage gap rather than a zero.
func (c *checker) m2Series(ctx context.Context) error {
	s, err := c.api.stats(ctx, c.small("m2-series", "tick", c.d.m2Hour(-3), c.d.m2Hour(0)), false, true)
	if err != nil {
		return err
	}
	if len(s.Series) != 3 {
		return fmt.Errorf("%d buckets, want 3", len(s.Series))
	}
	b := s.Series
	if !b[0].Available || b[0].Calls == nil || *b[0].Calls != 100 || !b[1].Available || b[1].Calls == nil || *b[1].Calls != 0 {
		return fmt.Errorf("first buckets %v / %v, want 100 and a confirmed 0", b[0].Calls, b[1].Calls)
	}
	if b[2].Available || b[2].Calls != nil || b[2].Coverage != "gap" {
		return fmt.Errorf("third bucket: available %v, calls %v, coverage %s; want a gap", b[2].Available, b[2].Calls, b[2].Coverage)
	}
	if s.Coverage.Status != "gap" {
		return fmt.Errorf("the summary has coverage %s, want an incomplete-coverage qualification", s.Coverage.Status)
	}
	return nil
}

// M2-LAT-002: the slow tail is kept.
func (c *checker) m2Dist(ctx context.Context) error {
	s, err := c.api.stats(ctx, c.h5("m2-dist", "render"), false, false)
	if err != nil {
		return err
	}
	row, err := s.only()
	if err != nil {
		return err
	}
	for _, name := range []string{"p50", "p95", "p99"} {
		if row.Percentiles[name].Samples != 1000 || row.Percentiles[name].Status != "available" {
			return fmt.Errorf("%s: %d samples, %s", name, row.Percentiles[name].Samples, row.Percentiles[name].Status)
		}
	}
	return expectPercentiles("render", row, 100, 300, 1000)
}

// M2-LAT-003: unequal buckets combine their distributions (stored states).
func (c *checker) m2Buckets(ctx context.Context) error {
	if _, err := c.api.rebuild(ctx, ns(c.d.m2Hour(4)), ns(c.d.m2Hour(6))); err != nil {
		return err
	}
	s, err := c.api.stats(ctx, c.small("m2-buckets", "render", c.d.m2Hour(4), c.d.m2Hour(6)), false, false)
	if err != nil {
		return err
	}
	for _, p := range s.Plan {
		if p.Source != "stored" {
			return fmt.Errorf("bucket %s comes from %s, want stored statistics", p.From, p.Source)
		}
	}
	row, err := s.only()
	if err != nil {
		return err
	}
	return firstErr(expectInt("valid durations", row.ValidDurations, 1000), near("p95", row.Percentiles["p95"].ValueMs, 100, msTol))
}

// M2-LAT-005: invalid durations are counted as calls but not as samples.
func (c *checker) m2Invalid(ctx context.Context) error {
	s, err := c.api.stats(ctx, c.h5("m2-invalid", "render"), false, false)
	if err != nil {
		return err
	}
	row, err := s.only()
	if err != nil {
		return err
	}
	return firstErr(expectInt("calls", row.Calls, 1003), expectInt("valid durations", row.ValidDurations, 1001),
		expectInt("excluded durations", row.InvalidDurations, 2), expectPercentiles("render", row, 100, 100, 100))
}

// M2-ERR-001: equal exceptions group by type and frame, not by request.
func (c *checker) m2Exceptions(ctx context.Context) error {
	g, err := c.api.errorGroups(ctx, c.h5("m2-exc", ""))
	if err != nil {
		return err
	}
	got := map[string]int{}
	for _, x := range g.find("exception", func(apiErrorGroup) bool { return true }) {
		if x.Method != "exception" {
			return fmt.Errorf("exception group method %s", x.Method)
		}
		got[x.ExceptionType+" at "+x.Frame] = x.Occurrences
	}
	if len(got) != 2 || got["StorageError at storage.Upload"] != 2 || got["StorageError at auth.Authorize"] != 1 {
		return fmt.Errorf("exception groups %v, want storage.Upload: 2, auth.Authorize: 1", got)
	}
	if g.Definitions["errors"] != "errors-v1" {
		return fmt.Errorf("grouping rule version not shown")
	}
	return nil
}

// M2-ERR-002: message groups keep status codes apart and original text.
func (c *checker) m2Messages(ctx context.Context) error {
	g, err := c.api.errorGroups(ctx, c.h5("m2-msg", ""))
	if err != nil {
		return err
	}
	logs := g.find("log", func(apiErrorGroup) bool { return true })
	got := map[string]int{}
	var g403 apiErrorGroup
	for _, x := range logs {
		got[x.Signature] = x.Occurrences
		if x.Method != "message" {
			return fmt.Errorf("group %q method %s, want message", x.Signature, x.Method)
		}
		if strings.Contains(x.Signature, "403") {
			g403 = x
		}
	}
	if len(got) != 2 || g403.Occurrences != 2 {
		return fmt.Errorf("message groups %v, want 403: 2 and 500: 1", got)
	}
	ex, err := c.api.examples(ctx, examplesRequest{Population: c.h5("m2-msg", ""), Kind: "error", Group: refOf(g403), Limit: 10})
	if err != nil {
		return err
	}
	var messages []string
	for _, e := range ex.Examples {
		messages = append(messages, e.Message)
	}
	return expectList("403 examples", messages, []string{"Request request-A failed with status 403", "Request request-B failed with status 403"})
}

// M2-ERR-003: log occurrences don't turn an OK call into a failed one.
func (c *checker) m2LogErrorsOkCall(ctx context.Context) error {
	g, err := c.api.errorGroups(ctx, c.h5("m2-okerr", ""))
	if err != nil {
		return err
	}
	logs := g.find("log", func(apiErrorGroup) bool { return true })
	if len(logs) != 1 || logs[0].Occurrences != 3 || logs[0].AffectedTraces != 1 {
		return fmt.Errorf("log groups %+v, want one group with 3 occurrences in 1 trace", logs)
	}
	if n := len(g.find("span_status", func(apiErrorGroup) bool { return true })); n != 0 {
		return fmt.Errorf("%d span-status groups, want 0", n)
	}
	s, err := c.api.stats(ctx, c.h5("m2-okerr", "charge"), false, false)
	if err != nil {
		return err
	}
	row, err := s.only()
	if err != nil {
		return err
	}
	return firstErr(expectInt("calls", row.Calls, 1), expectInt("error calls", row.Errors, 0))
}

// M2-ERR-004: an unlinked error log stays under an unknown version.
func (c *checker) m2Unlinked(ctx context.Context) error {
	g, err := c.api.errorGroups(ctx, c.h5("m2-unlinked", ""))
	if err != nil {
		return err
	}
	logs := g.find("log", func(apiErrorGroup) bool { return true })
	if len(logs) != 1 {
		return fmt.Errorf("%d log groups, want 1", len(logs))
	}
	x := logs[0]
	if x.Occurrences != 1 || x.Unlinked != 1 || x.AffectedTraces != 0 || len(x.Versions) != 1 || x.Versions[0].Version != "" {
		return fmt.Errorf("unlinked group %+v, want 1 unlinked occurrence under an unknown version", x)
	}
	p := c.h5("m2-unlinked", "")
	p.Version = sp("1.1.0")
	g, err = c.api.errorGroups(ctx, p)
	if err != nil {
		return err
	}
	if n := len(g.find("log", func(apiErrorGroup) bool { return true })); n != 0 {
		return fmt.Errorf("the unlinked log was attributed to version 1.1.0")
	}
	return nil
}

// M2-ERR-005: recurring, newly observed and not observed within the scopes.
func (c *checker) m2GroupLabels(ctx context.Context) error {
	b, cand := c.h5("m2-groups", ""), c.h5("m2-groups", "")
	b.Version, cand.Version = sp("b"), sp("c")
	a, err := c.api.compare(ctx, compareRequest{Baseline: b, Candidate: cand})
	if err != nil {
		return err
	}
	got := map[string]string{}
	for _, g := range a.ErrorGroups {
		got[g.Signature] = g.Label
		if g.Label == "recurring" && (g.Baseline.FirstSeen == "" || g.Candidate.LastSeen == "") {
			return fmt.Errorf("first/last occurrence missing for %q", g.Signature)
		}
	}
	want := map[string]string{"Cache lookup timed out": "recurring", "Webhook signature invalid": "newly_observed_in_candidate", "Thumbnail generation failed": "not_observed_in_candidate"}
	for sig, label := range want {
		if got[sig] != label {
			return fmt.Errorf("labels %v, want %v", got, want)
		}
	}
	return nil
}

// M2-ERR-006: distinct identities stay distinct; source kinds stay apart.
func (c *checker) m2Dups(ctx context.Context) error {
	g, err := c.api.errorGroups(ctx, c.h5("m2-dups", ""))
	if err != nil {
		return err
	}
	count := func(kind string) int {
		n := 0
		for _, x := range g.find(kind, func(apiErrorGroup) bool { return true }) {
			n += x.Occurrences
		}
		return n
	}
	return firstErr(expectInt("log occurrences", count("log"), 2), expectInt("ERROR-status span occurrences", count("span_status"), 1),
		expectInt("exception occurrences", count("exception"), 1))
}

// M2-CMP-004: different window lengths compare by rate.
func (c *checker) m2Rates(ctx context.Context) error {
	b := c.small("m2-rate", "poll", c.d.m2Hour(1), c.d.m2Hour(2))
	cand := c.small("m2-rate", "poll", c.d.m2Hour(3), c.d.m2Hour(5))
	a, err := c.api.compare(ctx, compareRequest{Baseline: b, Candidate: cand})
	if err != nil {
		return err
	}
	calls, _ := a.metric(0, "calls")
	rate, _ := a.metric(0, "rate")
	if err := firstErr(near("call difference", calls.Absolute, 2000, 0), near("baseline rate", rate.Baseline, 33.333, 0.01),
		near("candidate rate", rate.Candidate, 33.333, 0.01), expectClass(calls, "descriptive")); err != nil {
		return err
	}
	for _, n := range a.Notices {
		if n.Code == "window_length" {
			return nil
		}
	}
	return fmt.Errorf("the different window durations are not pointed out")
}

// M2-CMP-008: an operation without candidate calls stays visible.
func (c *checker) m2MissingOperation(ctx context.Context) error {
	b, cand := c.h5("m2-ops", ""), c.h5("m2-ops", "")
	b.Version, cand.Version = sp("b"), sp("c")
	a, err := c.api.compare(ctx, compareRequest{Baseline: b, Candidate: cand})
	if err != nil {
		return err
	}
	for i, r := range a.Rows {
		if r.Operation != "publish_recording" {
			continue
		}
		if r.Baseline == nil || r.Baseline.Calls != 1000 || r.Candidate != nil {
			return fmt.Errorf("publish_recording row: baseline %v, candidate %v", r.Baseline, r.Candidate)
		}
		calls, _ := a.metric(i, "calls")
		p95, _ := a.metric(i, "p95")
		ef, _ := a.metric(i, "error_fraction")
		if calls.Candidate == nil || *calls.Candidate != 0 || p95.Candidate != nil || ef.Candidate != nil {
			return fmt.Errorf("candidate calls %v, p95 %v, error fraction %v; want 0 and unavailable", calls.Candidate, p95.Candidate, ef.Candidate)
		}
		if len(r.Notes) == 0 || !strings.Contains(r.Notes[0], "does not mean it was fixed") {
			return fmt.Errorf("notes %v", r.Notes)
		}
		return nil
	}
	return fmt.Errorf("publish_recording is missing from the comparison")
}

// M2-INV-001: a slow candidate call opens its whole trace.
func (c *checker) m2SlowExample(ctx context.Context) error {
	p := c.refPop(1)
	ex, err := c.api.examples(ctx, examplesRequest{Population: p, Kind: "slow", ThresholdMs: 160, Limit: 5})
	if err != nil {
		return err
	}
	if ex.Status != "ok" || ex.Shown != 5 || ex.Total != 1000 {
		return fmt.Errorf("slow examples: %s, %d of %d", ex.Status, ex.Shown, ex.Total)
	}
	e := ex.Examples[0]
	if e.SpanID != idChild1 || e.DurationMs == nil || *e.DurationMs < 160 || !strings.HasPrefix(e.TraceID, c.d.run) {
		return fmt.Errorf("example %+v doesn't match the population", e)
	}
	r, err := c.api.report(ctx, reportRequest{TraceID: e.TraceID, TimeFrom: p.TimeFrom, TimeTo: p.TimeTo})
	if err != nil {
		return err
	}
	if err := expectNames("report spans", r.Spans.Records, "start_recording", "upload_recording", "storage.PutObject"); err != nil {
		return err
	}
	services := map[string]bool{}
	for _, l := range r.Logs.Records {
		if l.Severity == "INFO" {
			services[l.Service] = true
		}
	}
	if !services[c.d.service("recording-api")] || !services[c.d.service("recording-worker")] {
		return fmt.Errorf("the report hides supporting INFO logs of other services: %v", services)
	}
	return nil
}

// M2-INV-002: a log error group opens a trace whose spans stay OK.
func (c *checker) m2LogExample(ctx context.Context) error {
	p := c.h5("m2-okerr", "")
	g, err := c.api.errorGroups(ctx, p)
	if err != nil {
		return err
	}
	logs := g.find("log", func(apiErrorGroup) bool { return true })
	if len(logs) != 1 {
		return fmt.Errorf("%d log groups", len(logs))
	}
	ex, err := c.api.examples(ctx, examplesRequest{Population: p, Kind: "error", Group: refOf(logs[0]), Limit: 5})
	if err != nil {
		return err
	}
	if ex.Total != 3 || ex.Examples[0].TraceID != c.d.traceID(scM2OkErr, 1) || ex.Examples[0].SpanID != idRoot || ex.Examples[0].RecordID == "" {
		return fmt.Errorf("examples %+v", ex.Examples)
	}
	r, err := c.api.report(ctx, reportRequest{TraceID: ex.Examples[0].TraceID, TimeFrom: p.TimeFrom, TimeTo: p.TimeTo})
	if err != nil {
		return err
	}
	if len(r.Spans.Records) != 1 || r.Spans.Records[0].Status != "ok" {
		return fmt.Errorf("the report's span is not the recorded OK span")
	}
	if len(r.Logs.Records) != 4 {
		return fmt.Errorf("%d logs in the report, want the 3 ERROR logs and the INFO context", len(r.Logs.Records))
	}
	return nil
}

// M2-INV-003: bounded examples don't change the statistics.
func (c *checker) m2BoundedExamples(ctx context.Context) error {
	p := c.h5("m2-slow", "render")
	ex, err := c.api.examples(ctx, examplesRequest{Population: p, Kind: "slow", ThresholdMs: 500, Limit: 5})
	if err != nil {
		return err
	}
	if ex.Shown != 5 || ex.Total != 40 || !ex.Truncated {
		return fmt.Errorf("%d shown of %d, truncated %v; want 5 of 40, truncated", ex.Shown, ex.Total, ex.Truncated)
	}
	for _, e := range ex.Examples {
		if e.DurationMs == nil || *e.DurationMs < 500 {
			return fmt.Errorf("example below the threshold: %v", e.DurationMs)
		}
	}
	s, err := c.api.stats(ctx, p, false, false)
	if err != nil {
		return err
	}
	row, err := s.only()
	if err != nil {
		return err
	}
	return expectInt("calls", row.Calls, 1000)
}

// M2-INV-004: a new report includes a later log; the saved comparison doesn't change.
func (c *checker) m2LaterEvidence(ctx context.Context, o options) error {
	if c.state.M2Regression == "" {
		return fmt.Errorf("the regression comparison was not saved")
	}
	if !c.state.M2LateLogSent {
		if err := sendLogs(ctx, c.d, o.sender, []sender.LogRecord{c.d.m2LateLog()}); err != nil {
			return err
		}
		c.state.M2LateLogSent = true
		if err := c.state.save(c.statePath); err != nil {
			return err
		}
	}
	p := c.refPop(1)
	r, err := c.api.report(ctx, reportRequest{TraceID: c.d.traceID(scM2Ref+1, 1), TimeFrom: p.TimeFrom, TimeTo: p.TimeTo})
	if err != nil {
		return err
	}
	found := false
	for _, l := range r.Logs.Records {
		found = found || l.RecordID == c.d.recordID("inv4-late")
	}
	if !found {
		return fmt.Errorf("the new report doesn't include the later log")
	}
	orig, err := c.api.analysis(ctx, c.state.M2Regression)
	if err != nil {
		return err
	}
	if orig.GeneratedAt != c.state.M2RegressionGenerated || len(orig.Rows) != 1 || orig.Rows[0].Candidate.Calls != 1000 {
		return fmt.Errorf("the saved comparison changed")
	}
	return nil
}

// M2-REP-002: a reviewed comparison keeps its values; regenerating includes
// later calls under a new revision.
func (c *checker) m2Revision(ctx context.Context, o options) error {
	b := c.small("m2-rep", "tick", c.d.m2Hour(1), c.d.m2Hour(2))
	cand := c.small("m2-rep", "tick", c.d.m2Hour(3), c.d.m2Hour(4))
	b.Version, cand.Version = sp("b"), sp("c")
	if c.state.M2Reviewed == "" {
		a, err := c.api.compare(ctx, compareRequest{Baseline: b, Candidate: cand})
		if err != nil {
			return err
		}
		if len(a.Rows) != 1 || a.Rows[0].Candidate == nil || a.Rows[0].Candidate.Calls != 1000 {
			return fmt.Errorf("reviewed comparison: want 1000 candidate calls")
		}
		c.state.M2Reviewed = a.ID
		if err := c.state.save(c.statePath); err != nil {
			return err
		}
	}
	if !c.state.M2RepSent {
		otlp, err := dialOTLP(o.otlp)
		if err != nil {
			return err
		}
		defer otlp.Close()
		if err := otlp.sendAll(ctx, []*collectortracepb.ExportTraceServiceRequest{c.d.m2RepExtra()}); err != nil {
			return err
		}
		c.state.M2RepSent = true
		if err := c.state.save(c.statePath); err != nil {
			return err
		}
	}
	if _, err := c.api.rebuild(ctx, ns(c.d.m2Hour(3)), ns(c.d.m2Hour(4))); err != nil {
		return err
	}
	reviewed, err := c.api.analysis(ctx, c.state.M2Reviewed)
	if err != nil {
		return err
	}
	if reviewed.Rows[0].Candidate.Calls != 1000 || reviewed.Rules.Version != "comparison-v1" {
		return fmt.Errorf("the reviewed comparison changed: %d calls, rules %s", reviewed.Rows[0].Candidate.Calls, reviewed.Rules.Version)
	}
	a, err := c.api.compare(ctx, compareRequest{Baseline: b, Candidate: cand, Regenerates: c.state.M2Reviewed})
	if err != nil {
		return err
	}
	if a.ID == c.state.M2Reviewed || a.Regenerates != c.state.M2Reviewed || a.Rows[0].Candidate.Calls != 1010 {
		return fmt.Errorf("regenerated: id %s (regenerates %s), %d candidate calls; want a new revision with 1010", a.ID, a.Regenerates, a.Rows[0].Candidate.Calls)
	}
	return nil
}

// m2Checks run on every pass.
func (c *checker) m2Checks(o options) []namedCheck {
	return append([]namedCheck{
		{"M2-REF-001", "Known regression 1.0.0 -> 1.1.0 (with M2-CMP-001)", c.m2Regression},
		{"M2-REF-001E", "Regression example opens trace and 403 log", c.m2RegressionEvidence},
		{"M2-REF-002", "Fix 1.1.1 against 1.1.0 and 1.0.0 (with M2-CMP-002)", c.m2Fix},
		{"M2-CAL-001", "SERVER calls without dependency spans", c.m2Kinds},
		{"M2-CAL-002", "Versions and environments, unknown version", c.m2Versions},
		{"M2-CAL-003", "Half-open window and rate per minute", c.m2Window},
		{"M2-CAL-004", "Colliding span ids, replays", c.m2Collide},
		{"M2-CAL-006", "Sampling and collection gap shown", c.m2Sampled},
		{"M2-CAL-007", "Conflicting payloads count once, with notice", c.m2Conflict},
		{"M2-CAL-008", "Time series: 100, 0, gap", c.m2Series},
		{"M2-LAT-002", "Slow tail: p50 100, p95 300, p99 1000", c.m2Dist},
		{"M2-LAT-003", "Unequal buckets merge distributions", c.m2Buckets},
		{"M2-LAT-005", "Invalid durations excluded, calls kept", c.m2Invalid},
		{"M2-ERR-001", "Exceptions grouped by type and frame", c.m2Exceptions},
		{"M2-ERR-002", "Message groups keep status codes apart", c.m2Messages},
		{"M2-ERR-003", "Error logs don't fail an OK call", c.m2LogErrorsOkCall},
		{"M2-ERR-004", "Unlinked error under unknown version", c.m2Unlinked},
		{"M2-ERR-005", "Recurring, new, not observed labels", c.m2GroupLabels},
		{"M2-ERR-006", "Distinct events kept; source kinds apart", c.m2Dups},
		{"M2-CMP-004", "Different windows compared by rate", c.m2Rates},
		{"M2-CMP-006", "Environment mismatch: no conclusion", c.m2Mismatch},
		{"M2-CMP-008", "Operation without candidate calls stays", c.m2MissingOperation},
		{"M2-INV-001", "Slow example opens its whole trace", c.m2SlowExample},
		{"M2-INV-002", "Log error example, spans stay OK", c.m2LogExample},
		{"M2-INV-003", "Bounded examples, statistics unchanged", c.m2BoundedExamples},
		{"M2-INV-004", "Later log in new report, comparison unchanged", func(ctx context.Context) error { return c.m2LaterEvidence(ctx, o) }},
		{"M2-REP-002", "Reviewed comparison stable; regenerate 1010", func(ctx context.Context) error { return c.m2Revision(ctx, o) }},
	}, c.pwChecks()...)
}

// m2Replay resends every milestone 2 record sent so far, byte for byte.
func (c *checker) m2Replay(ctx context.Context, o options, otlp *otlpSender) error {
	reqs := c.d.m2Spans()
	if c.state.M2RepSent {
		reqs = append(reqs, c.d.m2RepExtra())
	}
	if c.state.M2ProvSent {
		reqs = append(reqs, c.d.m2ProvExtra())
	}
	if c.state.M2FailSent {
		reqs = append(reqs, c.d.m2FailExtra())
	}
	if c.state.M2LateSent {
		reqs = append(reqs, c.d.m2Late())
	}
	if err := otlp.sendAll(ctx, reqs); err != nil {
		return err
	}
	logs := c.d.m2Logs()
	if c.state.M2LateLogSent {
		logs = append(logs, c.d.m2LateLog())
	}
	return sendLogsChunked(ctx, c.d, o.sender, logs)
}
