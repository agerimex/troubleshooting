// Command regression loads the regression fixture into a Troubleshooting test
// stack and checks the investigation workflow against it. See README.md.
//
//	go run . load  [addresses]   fresh run: new base time and ids
//	go run . check [addresses] [-replay] [-outage] [-recreate] [-limited-api URL]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type options struct {
	api, otlp, sender, limitedAPI string
	statePath, reportPath         string
	replay, outage, recreate      bool
	retention                     bool
	container, composeFile        string
	composeProject                string
	readyTimeout                  time.Duration
}

func main() {
	if len(os.Args) < 2 || (os.Args[1] != "load" && os.Args[1] != "check") {
		fmt.Fprintln(os.Stderr, "usage: regression load|check [flags]  (-h for flags)")
		os.Exit(2)
	}
	var o options
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	fs.StringVar(&o.api, "api", "http://127.0.0.1:28094", "log-analysis base URL")
	fs.StringVar(&o.otlp, "otlp", "127.0.0.1:24317", "receiver OTLP/gRPC address")
	fs.StringVar(&o.sender, "sender", "127.0.0.1:20055", "receiver log-sender (SendSpans) address")
	fs.StringVar(&o.limitedAPI, "limited-api", "", "log-analysis with tiny query limits; empty skips the limit check")
	fs.StringVar(&o.statePath, "state", filepath.Join(".state", "fixture.json"), "run state written by load")
	fs.StringVar(&o.reportPath, "report", filepath.Join("results", "report.md"), "check report (Markdown)")
	fs.BoolVar(&o.replay, "replay", false, "check: resend the whole payload twice, then check again (RF02)")
	fs.BoolVar(&o.outage, "outage", false, "check: stop the test ClickHouse, expect explicit errors, start it, retry (RF13)")
	fs.BoolVar(&o.recreate, "recreate", false, "check: recreate the test ClickHouse container with its volume, check again (RF15)")
	fs.BoolVar(&o.retention, "retention", false, "check: milestone 2 retention phase (moves the test clock, restarts log-analysis, recreates the receiver)")
	fs.StringVar(&o.container, "container", "troubleshooting-test-clickhouse", "test ClickHouse container (-outage)")
	fs.StringVar(&o.composeFile, "compose-file", filepath.Join("..", "..", "docker-compose.test.yml"), "test stack compose file (-recreate)")
	fs.StringVar(&o.composeProject, "compose-project", "troubleshooting-test", "test stack compose project (-recreate)")
	fs.DurationVar(&o.readyTimeout, "ready-timeout", 3*time.Minute, "how long to wait for the stack and the data")
	_ = fs.Parse(os.Args[2:])

	ctx := context.Background()
	var err error
	if os.Args[1] == "load" {
		err = load(ctx, o)
	} else {
		err = check(ctx, o)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
}

// load sends a fresh run: new run id and base time, so it never mixes with
// the ids of an earlier run.
func load(ctx context.Context, o options) error {
	st, err := newState()
	if err != nil {
		return err
	}
	d := st.dataset()
	fmt.Printf("run %s, base time %s\n", st.Run, st.Loaded)

	c := &checker{api: newAPI(o.api), d: d, state: st, statePath: o.statePath, notes: map[string]string{}}
	if err := c.waitReady(ctx, o); err != nil {
		return err
	}

	otlp, err := dialOTLP(o.otlp)
	if err != nil {
		return err
	}
	defer otlp.Close()
	if err := otlp.sendAll(ctx, d.deliveries()); err != nil {
		return err
	}
	if err := sendParity(ctx, d, o.sender, o.otlp); err != nil {
		return err
	}
	st.ParitySent = true
	if err := sendLogs(ctx, d, o.sender, d.m1Logs()); err != nil {
		return fmt.Errorf("milestone 1 logs: %w", err)
	}
	if err := otlp.sendAll(ctx, d.m2Spans()); err != nil {
		return fmt.Errorf("milestone 2 spans: %w", err)
	}
	if err := sendLogsChunked(ctx, d, o.sender, d.m2Logs()); err != nil {
		return fmt.Errorf("milestone 2 logs: %w", err)
	}
	for _, decl := range d.m2Coverage() {
		if err := c.api.declare(ctx, decl); err != nil {
			return fmt.Errorf("coverage declaration: %w", err)
		}
	}
	if err := st.save(o.statePath); err != nil {
		return err
	}

	want := c.expectedTotal()
	if err := waitFor(ctx, "the fixture data", o.readyTimeout, func() error {
		f := c.wide()
		f.ServiceName, f.Scope = "fx"+d.run, "all"
		n, err := c.api.count(ctx, f)
		if err == nil && n != want {
			err = fmt.Errorf("%d of %d spans", n, want)
		}
		return err
	}); err != nil {
		return err
	}
	if err := c.m2Ready(ctx, o); err != nil {
		return err
	}
	fmt.Printf("loaded %d logical spans and %d log records (milestone 1), the milestone 2 reference releases and cases; state in %s\n",
		want, len(d.m1Logs()), o.statePath)
	return nil
}

// waitFor polls ready every second until it succeeds or timeout passes.
func waitFor(ctx context.Context, what string, timeout time.Duration, ready func() error) error {
	deadline := time.Now().Add(timeout)
	for {
		err := ready()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s not ready after %s: %w", what, timeout, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

type result struct {
	phase, id, title string
	err              error
	skipped          string
}

type runner struct {
	results []result
}

func (r *runner) run(ctx context.Context, phase string, checks []namedCheck) {
	for _, ch := range checks {
		err := ch.run(ctx)
		r.results = append(r.results, result{phase: phase, id: ch.id, title: ch.title, err: err})
		status := "PASS"
		if err != nil {
			status = "FAIL: " + err.Error()
		}
		fmt.Printf("[%s] %-6s %-55s %s\n", phase, ch.id, ch.title, status)
	}
}

func (r *runner) skip(id, title, why string) {
	r.results = append(r.results, result{phase: "-", id: id, title: title, skipped: why})
	fmt.Printf("[-] %-6s %-55s SKIPPED: %s\n", id, title, why)
}

func (r *runner) failed() int {
	n := 0
	for _, res := range r.results {
		if res.err != nil {
			n++
		}
	}
	return n
}

func check(ctx context.Context, o options) error {
	st, err := loadState(o.statePath)
	if err != nil {
		return err
	}
	d := st.dataset()
	c := &checker{api: newAPI(o.api), d: d, state: st, statePath: o.statePath, notes: map[string]string{}}
	otlp, err := dialOTLP(o.otlp)
	if err != nil {
		return err
	}
	defer otlp.Close()
	r := &runner{}
	fmt.Printf("run %s, base time %s\n", st.Run, st.Loaded)
	if err := c.waitReady(ctx, o); err != nil {
		return err
	}

	// RF05 first: the parent is delivered only after the child was seen alone.
	if !st.LateSent {
		r.run(ctx, "initial", []namedCheck{{"RF05", "Child searchable, parent not yet in the window", func(ctx context.Context) error {
			if err := c.rf05(ctx, false); err != nil {
				return err
			}
			if _, err := otlp.send(ctx, d.late()); err != nil {
				return err
			}
			st.LateSent = true
			return st.save(o.statePath)
		}}})
	}
	// M1-REL-003 likewise: the parent and the referenced span arrive after a report without them.
	if !st.M1LateSent {
		r.run(ctx, "initial", []namedCheck{{"M1-REL-003", "Report before the late parent and referenced span", func(ctx context.Context) error {
			if err := c.m1Late(ctx, false); err != nil {
				return err
			}
			if _, err := otlp.send(ctx, d.m1Late()); err != nil {
				return err
			}
			st.M1LateSent = true
			return st.save(o.statePath)
		}}})
	}
	r.run(ctx, "initial", c.scenarioChecks())
	r.run(ctx, "initial", c.m1Checks())
	r.run(ctx, "initial", c.m2Checks(o))
	r.run(ctx, "initial", []namedCheck{
		{"PARITY", "log-sender and OTLP paths store the same span", c.parity},
		{"IDS", "Invalid ids are rejected, valid span kept", func(ctx context.Context) error {
			err := c.invalidIDs(ctx, otlp)
			if err == nil && !st.InvalidSent {
				st.InvalidSent = true
				err = st.save(o.statePath)
			}
			return err
		}},
	})
	if o.limitedAPI != "" {
		r.run(ctx, "initial", []namedCheck{{"LIMIT", "A query over its row limit fails explicitly (HTTP 504)", func(ctx context.Context) error {
			return c.limited(ctx, newAPI(o.limitedAPI))
		}}})
	} else {
		r.skip("LIMIT", "A query over its row limit fails explicitly", "no -limited-api")
	}

	if o.replay {
		r.run(ctx, "replay", []namedCheck{{"RF02", "Resend every span and log request twice", func(ctx context.Context) error {
			for i := 0; i < 2; i++ {
				if err := otlp.sendAll(ctx, d.deliveries()); err != nil {
					return err
				}
				if _, err := otlp.send(ctx, d.late()); err != nil {
					return err
				}
				st.LateSent = true
				if err := sendParity(ctx, d, o.sender, o.otlp); err != nil {
					return err
				}
				if _, err := otlp.send(ctx, d.invalidIDs()); err != nil {
					return err
				}
				st.InvalidSent = true
				if _, err := otlp.send(ctx, d.m1Late()); err != nil {
					return err
				}
				st.M1LateSent = true
				if err := sendLogs(ctx, d, o.sender, d.m1Logs()); err != nil {
					return err
				}
				if err := c.m2Replay(ctx, o, otlp); err != nil {
					return err
				}
			}
			return st.save(o.statePath)
		}}})
		r.run(ctx, "after replay", c.scenarioChecks())
		r.run(ctx, "after replay", c.m1Checks())
		r.run(ctx, "after replay", c.m2Checks(o))
		r.run(ctx, "after replay", []namedCheck{{"PARITY", "log-sender and OTLP paths store the same span", c.parity}})
	} else {
		r.skip("RF02", "Replay", "no -replay")
	}

	if o.outage {
		r.run(ctx, "outage", []namedCheck{{"RF13", "Outage: explicit errors, no false log confirmation (M1-REF-003); retry recovers", func(ctx context.Context) error {
			return c.outage(ctx, o)
		}}})
	} else {
		r.skip("RF13", "Database outage", "no -outage")
	}

	if o.recreate {
		r.run(ctx, "recreate", []namedCheck{{"RF15", "Recreate ClickHouse, keep its volume (M1-REF-002)", func(ctx context.Context) error {
			return c.recreate(ctx, o)
		}}})
		r.run(ctx, "after recreate", c.scenarioChecks())
		r.run(ctx, "after recreate", c.m1Checks())
		r.run(ctx, "after recreate", c.m2Checks(o))
	} else {
		r.skip("RF15", "Container recreation", "no -recreate")
	}

	if o.retention {
		defer func() { _ = c.clock(context.Background(), 0) }()
		r.run(ctx, "retention", c.m2RetentionChecks(o))
	} else {
		r.skip("M2-RET", "Statistics retention phase", "no -retention")
	}

	r.skip("RF11", "Span list succeeds while count fails", "UI check: log-ui/e2e (npm run test:e2e)")
	r.skip("RF14", "Slow old response after a newer one", "UI check: log-ui/e2e (npm run test:e2e)")

	if err := writeReport(o.reportPath, st, c, r); err != nil {
		return err
	}
	fmt.Printf("report: %s\n", o.reportPath)
	if n := r.failed(); n > 0 {
		return fmt.Errorf("%d checks failed", n)
	}
	return nil
}

func docker(ctx context.Context, args ...string) error {
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return nil
}

// outage stops the test ClickHouse: a search must fail with an explicit
// error and no rows. After a restart the same search succeeds again.
func (c *checker) outage(ctx context.Context, o options) error {
	if err := docker(ctx, "stop", o.container); err != nil {
		return err
	}
	started := false
	defer func() {
		if !started {
			_ = docker(context.Background(), "start", o.container)
		}
	}()
	f := c.service(c.window(), "rf04", "all")
	f.Status = "error"
	rows, _, err := c.api.spans(ctx, f)
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.Status < 500 || apiErr.Message == "" {
		return fmt.Errorf("with ClickHouse stopped: %d rows, error %v; want an explicit 5xx error", len(rows), err)
	}
	if _, err := c.api.count(ctx, f); err == nil {
		return fmt.Errorf("with ClickHouse stopped the count succeeded")
	}
	// M1-CMP-001 on the real stack: the report still answers, with each section
	// marked unavailable rather than empty.
	report, err := c.api.report(ctx, reportRequest{TraceID: c.d.traceID(scM1Rec, 1), TimeFrom: f.TimeFrom, TimeTo: f.TimeTo})
	if err != nil {
		return fmt.Errorf("with ClickHouse stopped the report failed as a whole (%v); want unavailable sections", err)
	}
	if report.Complete || report.Spans == nil || report.Spans.Status == "ok" || report.Logs == nil || report.Logs.Status == "ok" ||
		len(report.Logs.Records) != 0 || report.Logs.Total != nil {
		return fmt.Errorf("with ClickHouse stopped the report looks usable: complete %v", report.Complete)
	}
	if err := c.m1OutageSend(ctx, o); err != nil {
		return err
	}
	c.notes["outage error"] = apiErr.Error()

	if err := docker(ctx, "start", o.container); err != nil {
		return err
	}
	started = true
	if err := c.waitReady(ctx, o); err != nil {
		return err
	}
	if err := c.rf04(ctx); err != nil {
		return err
	}
	return c.m1OutageRetry(ctx, o)
}

// recreate replaces the ClickHouse container (keeping the named volume) and
// checks that it really is a new container.
func (c *checker) recreate(ctx context.Context, o options) error {
	before, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.Id}}", o.container).Output()
	if err != nil {
		return fmt.Errorf("inspect %s: %w", o.container, err)
	}
	if err := docker(ctx, "compose", "-p", o.composeProject, "-f", o.composeFile,
		"up", "-d", "--force-recreate", "--wait", "clickhouse-server"); err != nil {
		return err
	}
	after, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.Id}}", o.container).Output()
	if err != nil {
		return err
	}
	if string(before) == string(after) {
		return fmt.Errorf("the container was not recreated")
	}
	return c.waitReady(ctx, o)
}

// waitReady polls until the API answers the query shapes the checks use: a
// page of spans with child counts, a trace and a count. A newly started
// ClickHouse container can take tens of seconds to plan its first queries
// under Docker Desktop, which the API reports as a timeout.
func (c *checker) waitReady(ctx context.Context, o options) error {
	return waitFor(ctx, "the API", o.readyTimeout, func() error {
		f := c.wide()
		f.Scope = "all"
		if _, _, err := c.api.spans(ctx, f); err != nil {
			return err
		}
		f.TraceID = c.d.traceID(scRF01, 1)
		if _, _, err := c.api.spans(ctx, f); err != nil {
			return err
		}
		_, err := c.api.count(ctx, f)
		return err
	})
}

func writeReport(path string, st *state, c *checker, r *runner) error {
	var b strings.Builder
	commit, _ := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	dirty, _ := exec.Command("git", "status", "--porcelain").Output()
	fmt.Fprintf(&b, "# Regression fixture report\n\n")
	fmt.Fprintf(&b, "- Checked: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Commit: %s", strings.TrimSpace(string(commit)))
	if len(dirty) > 0 {
		fmt.Fprintf(&b, " (with uncommitted changes)")
	}
	fmt.Fprintf(&b, "\n- Fixture version %d, run `%s`, base time %s\n", st.Version, st.Run, st.Loaded)
	skipped := 0
	for _, res := range r.results {
		if res.skipped != "" {
			skipped++
		}
	}
	fmt.Fprintf(&b, "- Result: %d checks run, %d failed, %d not run here\n\n", len(r.results)-skipped, r.failed(), skipped)
	fmt.Fprintf(&b, "| Phase | Check | Scenario | Result |\n| --- | --- | --- | --- |\n")
	for _, res := range r.results {
		outcome := "PASS"
		switch {
		case res.skipped != "":
			outcome = "not run here: " + res.skipped
		case res.err != nil:
			outcome = "**FAIL**: " + strings.ReplaceAll(res.err.Error(), "\n", " ")
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", res.phase, res.id, res.title, strings.ReplaceAll(outcome, "|", "\\|"))
	}
	if len(c.notes) > 0 {
		fmt.Fprintf(&b, "\n## Notes\n\n")
		for _, k := range sortedKeys(c.notes) {
			fmt.Fprintf(&b, "- %s: %s\n", k, c.notes[k])
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
