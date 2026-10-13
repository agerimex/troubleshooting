package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// Milestone 2 retention checks (statistics_retention.feature). They move the
// test stack's application clock (TEST_CLOCK_CONTROL) instead of waiting
// days: +25 hours finalizes the fixture's buckets (their 24 h lateness has
// passed while their raw records are still within the 48 h retention, which
// finalization needs), +4 days puts the raw records outside the 48 h raw retention, +31
// days the statistics outside their 30 days. Arrival times stay real.
// The phase runs last and resets the clock when it ends.

const (
	day            = 24 * time.Hour
	finalizeOffset = 25 * time.Hour
)

func (c *checker) clock(ctx context.Context, offset time.Duration) error {
	return c.api.setClock(ctx, int64(offset/time.Second))
}

func (c *checker) sendSpans(ctx context.Context, o options, reqs ...*collectortracepb.ExportTraceServiceRequest) error {
	otlp, err := dialOTLP(o.otlp)
	if err != nil {
		return err
	}
	defer otlp.Close()
	return otlp.sendAll(ctx, reqs)
}

func outcomeOf(outcomes []bucketOutcome, start time.Time) (bucketOutcome, error) {
	for _, b := range outcomes {
		if b.BucketStart == ns(start) {
			return b, nil
		}
	}
	return bucketOutcome{}, fmt.Errorf("no outcome for bucket %s", start.Format(time.RFC3339))
}

// callsOf returns the call count and plan of a single-row population.
func (c *checker) callsOf(ctx context.Context, p population) (int, apiStats, error) {
	s, err := c.api.stats(ctx, p, false, false)
	if err != nil {
		return 0, s, err
	}
	row, err := s.only()
	return row.Calls, s, err
}

// M2-RET-005: a late call within the lateness is included once.
func (c *checker) m2Provisional(ctx context.Context, o options) error {
	h5 := c.d.m2Hour(5)
	p := c.h5("m2-prov", "tick")
	if _, err := c.api.rebuild(ctx, ns(h5), ns(h5.Add(time.Hour))); err != nil {
		return err
	}
	n, s, err := c.callsOf(ctx, p)
	if err != nil {
		return err
	}
	want := 100
	if c.state.M2ProvSent {
		want = 101
	}
	if n != want || s.Plan[0].Source != "stored" || s.Plan[0].State != "provisional" {
		return fmt.Errorf("before the late call: %d calls from %s %s, want %d from provisional statistics", n, s.Plan[0].State, s.Plan[0].Source, want)
	}
	first := s.Plan[0].Revision
	for i := 0; i < 2; i++ { // the call, then its replay
		if err := c.sendSpans(ctx, o, c.d.m2ProvExtra()); err != nil {
			return err
		}
		c.state.M2ProvSent = true
		if err := c.state.save(c.statePath); err != nil {
			return err
		}
		outcomes, err := c.api.rebuild(ctx, ns(h5), ns(h5.Add(time.Hour)))
		if err != nil {
			return err
		}
		b, err := outcomeOf(outcomes, h5)
		if err != nil {
			return err
		}
		n, s, err = c.callsOf(ctx, p)
		if err != nil {
			return err
		}
		if n != 101 || s.Plan[0].State != "provisional" {
			return fmt.Errorf("after delivery %d: %d calls (%s), want 101 in a provisional revision", i+1, n, s.Plan[0].State)
		}
		if i == 0 && want == 100 && (b.Outcome != "revised" || s.Plan[0].Revision == first) {
			return fmt.Errorf("the late call made no new revision (outcome %s)", b.Outcome)
		}
		if i == 1 && b.Outcome != "unchanged" {
			return fmt.Errorf("the replay changed the bucket (outcome %s)", b.Outcome)
		}
	}
	return nil
}

// M2-RET-008: a failed refresh keeps the previous revision and is visible.
func (c *checker) m2FailedRefresh(ctx context.Context, o options) error {
	h5 := c.d.m2Hour(5)
	p := c.h5("m2-fail", "tick")
	if _, err := c.api.rebuild(ctx, ns(h5), ns(h5.Add(time.Hour))); err != nil {
		return err
	}
	if !c.state.M2FailSent {
		before, s, err := c.callsOf(ctx, p)
		if err != nil {
			return err
		}
		if before != 100 {
			return fmt.Errorf("%d calls before, want 100", before)
		}
		revision := s.Plan[0].Revision
		if err := c.sendSpans(ctx, o, c.d.m2FailExtra()); err != nil {
			return err
		}
		c.state.M2FailSent = true
		if err := c.state.save(c.statePath); err != nil {
			return err
		}
		if err := c.api.failNext(ctx, 1); err != nil {
			return err
		}
		outcomes, err := c.api.rebuild(ctx, ns(h5), ns(h5.Add(time.Hour)))
		if err != nil {
			return err
		}
		if b, err := outcomeOf(outcomes, h5); err != nil || b.Outcome != "failed" {
			return fmt.Errorf("the forced failure was not reported: %+v %v", b, err)
		}
		n, s, err := c.callsOf(ctx, p)
		if err != nil {
			return err
		}
		if n != 100 || s.Plan[0].Revision != revision {
			return fmt.Errorf("after the failed refresh: %d calls from revision %d, want 100 from %d", n, s.Plan[0].Revision, revision)
		}
		failures, err := c.api.refresherFailures(ctx)
		if err != nil {
			return err
		}
		if failures[ns(h5)] == "" {
			return fmt.Errorf("the failed refresh is not visible in the refresher status")
		}
		if _, err := c.api.rebuild(ctx, ns(h5), ns(h5.Add(time.Hour))); err != nil {
			return err
		}
	}
	n, _, err := c.callsOf(ctx, p)
	if err != nil {
		return err
	}
	return expectInt("calls after the retry", n, 101)
}

// m2Finalize moves the clock 25 hours on and rebuilds: every fixture bucket
// becomes final (T+5h's lateness ends at real time +22 h; T-3h's raw records
// leave the rebuildable range at +35 h).
func (c *checker) m2Finalize(ctx context.Context) error {
	if err := c.clock(ctx, finalizeOffset); err != nil {
		return err
	}
	outcomes, err := c.api.rebuild(ctx, ns(c.d.m2Hour(-3)), ns(c.d.m2Hour(6)))
	if err != nil {
		return err
	}
	for _, b := range outcomes {
		if b.State != "final" {
			return fmt.Errorf("bucket %s: %s (%s), want final", b.BucketStart, b.Outcome, b.Error)
		}
	}
	return nil
}

// M2-RET-007: stored hours and raw records never count one interval twice.
func (c *checker) m2Mixed(ctx context.Context) error {
	n, s, err := c.callsOf(ctx, c.small("m2-mix", "tick", c.d.m2Hour(4), c.d.m2Hour(6)))
	if err != nil {
		return err
	}
	if err := expectInt("calls over both hours", n, 150); err != nil {
		return err
	}
	if len(s.Plan) != 2 || s.Plan[0].Source != "stored" || s.Plan[1].Source != "stored" {
		return fmt.Errorf("plan %+v, want each hour from one stored revision", s.Plan)
	}
	// Half of the second hour comes from raw records next to the stored first hour.
	n, s, err = c.callsOf(ctx, c.small("m2-mix", "tick", c.d.m2Hour(4), c.d.m2Hour(5).Add(30*time.Minute)))
	if err != nil {
		return err
	}
	if len(s.Plan) != 2 || s.Plan[0].Source != "stored" || s.Plan[1].Source != "raw" {
		return fmt.Errorf("plan %+v, want stored then raw", s.Plan)
	}
	return expectInt("calls with a raw half hour", n, 150)
}

// M2-RET-006: after a restart, a replayed call and an unseen call for a final
// bucket change nothing and are reported as excluded.
func (c *checker) m2ClosedBucket(ctx context.Context, o options) error {
	if err := docker(ctx, "compose", "-p", o.composeProject, "-f", o.composeFile, "restart", "log-analysis"); err != nil {
		return err
	}
	if err := c.waitReady(ctx, o); err != nil {
		return err
	}
	// The clock offset is not persisted; the finalization metadata is.
	if err := c.clock(ctx, finalizeOffset); err != nil {
		return err
	}
	if err := c.sendSpans(ctx, o, c.d.m2Late(), c.d.referenceSpans(0)[0]); err != nil {
		return err
	}
	c.state.M2LateSent = true
	if err := c.state.save(c.statePath); err != nil {
		return err
	}
	outcomes, err := c.api.rebuild(ctx, ns(c.d.m2T()), ns(c.d.m2Hour(1)))
	if err != nil {
		return err
	}
	if b, err := outcomeOf(outcomes, c.d.m2T()); err != nil || b.Outcome != "final" {
		return fmt.Errorf("the closed bucket was rebuilt: %+v %v", b, err)
	}
	s, err := c.api.stats(ctx, c.refPop(0), false, false)
	if err != nil {
		return err
	}
	row, err := s.only()
	if err != nil {
		return err
	}
	if err := firstErr(expectInt("calls", row.Calls, 1000), expectInt("errors", row.Errors, 10), expectInt("excluded late calls", s.LateExcluded, 1),
		expectPercentiles("1.0.0", row, 100, 100, 100)); err != nil {
		return err
	}
	if _, ok := s.notice("late"); !ok {
		return fmt.Errorf("no exclusion notice")
	}
	return nil
}

// M2-RET-001: four days later the raw evidence is gone, the statistics stay.
func (c *checker) m2Expired(ctx context.Context) error {
	if err := c.clock(ctx, 4*day); err != nil {
		return err
	}
	s, err := c.api.stats(ctx, c.refPop(0), false, false)
	if err != nil {
		return err
	}
	row, err := s.only()
	if err != nil {
		return err
	}
	if err := firstErr(expectInt("calls", row.Calls, 1000), expectInt("errors", row.Errors, 10), expectPercentiles("1.0.0", row, 100, 100, 100)); err != nil {
		return err
	}
	if s.Definitions["metrics"] != "metrics-v1" {
		return fmt.Errorf("calculation version not retained")
	}
	g, err := c.api.errorGroups(ctx, c.refPop(0))
	if err != nil {
		return err
	}
	exc := g.find("exception", func(x apiErrorGroup) bool { return x.Frame == "storage.Upload" })
	if len(exc) != 1 || exc[0].Occurrences != 10 {
		return fmt.Errorf("retained exception group %+v, want 10 occurrences", exc)
	}
	ex, err := c.api.examples(ctx, examplesRequest{Population: c.refPop(0), Kind: "error", Group: refOf(exc[0]), Limit: 3})
	if err != nil {
		return err
	}
	if ex.Status != "outside_retention" || len(ex.Examples) != 0 || !strings.Contains(ex.Condition, "outside retention") {
		return fmt.Errorf("expired example: status %s, %d examples; want outside retention and none", ex.Status, len(ex.Examples))
	}
	return nil
}

// M2-RET-009: only whole buckets remain; exact sub-hour bounds are refused
// with an explicit whole-bucket alternative.
func (c *checker) m2WholeBuckets(ctx context.Context) error {
	p := c.refPop(0)
	p.TimeFrom, p.TimeTo = ns(c.d.m2T().Add(10*time.Minute)), ns(c.d.m2T().Add(70*time.Minute))
	s, err := c.api.stats(ctx, p, false, false)
	if err != nil {
		return err
	}
	if s.Status != "exact_bounds_unavailable" || s.SuggestedFrom != ns(c.d.m2T()) || s.SuggestedTo != ns(c.d.m2Hour(2)) || len(s.Rows) != 0 {
		return fmt.Errorf("status %s, suggestion [%s, %s), %d rows; want exact bounds unavailable and [T, T+2h)", s.Status, s.SuggestedFrom, s.SuggestedTo, len(s.Rows))
	}
	p.TimeFrom, p.TimeTo = s.SuggestedFrom, s.SuggestedTo
	s, err = c.api.stats(ctx, p, false, false)
	if err != nil {
		return err
	}
	row, err := s.only()
	if err != nil {
		return err
	}
	return firstErr(expectInt("calls in the whole buckets", row.Calls, 1000), near("window seconds", &s.DurationSeconds, 7200, 0))
}

// M2-RET-003: beyond statistical retention the result is unavailable.
func (c *checker) m2BeyondRetention(ctx context.Context) error {
	if err := c.clock(ctx, 31*day); err != nil {
		return err
	}
	s, err := c.api.stats(ctx, c.refPop(0), false, false)
	if err != nil {
		return err
	}
	if s.Status != "outside_retention" || len(s.Rows) != 0 {
		return fmt.Errorf("status %s with %d rows, want outside_retention without a count", s.Status, len(s.Rows))
	}
	a, err := c.api.compare(ctx, compareRequest{Baseline: c.refPop(0), Candidate: c.refPop(1)})
	if err != nil {
		return err
	}
	for _, m := range a.Rows[0].Metrics {
		if m.Classification == "improvement_signal" || m.Classification == "regression_signal" {
			return fmt.Errorf("%s classified %s from expired data", m.Metric, m.Classification)
		}
	}
	return nil
}

// M2-RET-004: backfill twice; unrecoverable ranges stay unavailable.
func (c *checker) m2Backfill(ctx context.Context) error {
	if err := c.clock(ctx, 0); err != nil {
		return err
	}
	from, to := time.Now().Add(-4*day).Truncate(time.Hour), time.Now()
	first, err := c.api.rebuild(ctx, ns(from), ns(to))
	if err != nil {
		return err
	}
	second, err := c.api.rebuild(ctx, ns(from), ns(to))
	if err != nil {
		return err
	}
	unrecoverable := 0
	for i, b := range second {
		if b.Outcome == "revised" && b.BucketStart != ns(time.Now().Truncate(time.Hour).Add(-time.Hour)) {
			return fmt.Errorf("the repeated backfill revised bucket %s", b.BucketStart)
		}
		if b.Outcome == "unrecoverable" {
			unrecoverable++
			if first[i].Outcome != "unrecoverable" {
				return fmt.Errorf("bucket %s changed from %s to unrecoverable", b.BucketStart, first[i].Outcome)
			}
		}
		if b.BucketStart == ns(c.d.m2T()) && b.Outcome != "final" {
			return fmt.Errorf("the reference bucket is %s, want final", b.Outcome)
		}
	}
	if unrecoverable == 0 {
		return fmt.Errorf("no bucket was reported unrecoverable although raw retention is 48 h")
	}
	return nil
}

// M2-RET-002: retention changes apply to the existing installation.
func (c *checker) m2RetentionChange(ctx context.Context, o options) error {
	apply := func(traces, logs, stats string) error {
		cmd := exec.CommandContext(ctx, "docker", "compose", "-p", o.composeProject, "-f", o.composeFile, "up", "-d", "--force-recreate", "--wait", "log-receiver")
		cmd.Env = append(os.Environ(), "FIXTURE_TRACE_RETENTION="+traces, "FIXTURE_LOG_RETENTION="+logs, "FIXTURE_STATS_RETENTION="+stats)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("recreate the receiver: %w: %s", err, out)
		}
		return waitFor(ctx, "the new retention", o.readyTimeout, func() error {
			p, err := c.api.policy(ctx)
			if err == nil && (p.TracesSeconds != seconds(traces) || p.LogsSeconds != seconds(logs) || p.StatsSeconds != seconds(stats)) {
				err = fmt.Errorf("effective retention %+v", p)
			}
			return err
		})
	}
	defer func() { _ = apply("48h", "48h", "720h") }()
	for i := 0; i < 2; i++ {
		if err := apply("168h", "72h", "2160h"); err != nil {
			return err
		}
		outcomes, err := c.api.rebuild(ctx, ns(c.d.m2T()), ns(c.d.m2Hour(6)))
		if err != nil {
			return err
		}
		for _, b := range outcomes {
			if b.Outcome != "final" {
				return fmt.Errorf("after applying the retention %d time(s), bucket %s is %s", i+1, b.BucketStart, b.Outcome)
			}
		}
	}
	n, _, err := c.callsOf(ctx, c.refPop(0))
	if err != nil {
		return err
	}
	if err := expectInt("calls after the retention change", n, 1000); err != nil {
		return err
	}
	return apply("48h", "48h", "720h")
}

func seconds(d string) int64 {
	parsed, err := time.ParseDuration(d)
	if err != nil {
		return -1
	}
	return int64(parsed.Seconds())
}

// m2RetentionChecks run in order, after every other phase.
func (c *checker) m2RetentionChecks(o options) []namedCheck {
	return []namedCheck{
		{"M2-RET-005", "Late call within lateness counted once", func(ctx context.Context) error { return c.m2Provisional(ctx, o) }},
		{"M2-RET-008", "Failed refresh keeps the previous revision", func(ctx context.Context) error { return c.m2FailedRefresh(ctx, o) }},
		{"M2-FINAL", "Clock +25 h: every fixture bucket final", c.m2Finalize},
		{"M2-RET-007", "Stored and raw hours never counted twice", c.m2Mixed},
		{"M2-RET-006", "Restart; late and replayed calls excluded", func(ctx context.Context) error { return c.m2ClosedBucket(ctx, o) }},
		{"M2-RET-001", "Clock +4 days: statistics stay, raw gone", c.m2Expired},
		{"M2-RET-009", "Sub-hour bounds refused, whole buckets offered", c.m2WholeBuckets},
		{"M2-RET-003", "Clock +31 days: unavailable, not zero", c.m2BeyondRetention},
		{"M2-RET-004", "Backfill twice; unrecoverable stays so", c.m2Backfill},
		{"M2-RET-002", "Retention change on the existing stack", func(ctx context.Context) error { return c.m2RetentionChange(ctx, o) }},
	}
}
