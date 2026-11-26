package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// overload checks that heavy work can't make the investigation API unusable
// (log-analysis data/workload.go):
//
//   - OVL-001: while ClickHouse runs heavy low-priority queries (the shape of a
//     big statistics refresh, priority 10 like the API's background work),
//     light interactive requests still answer, about as fast as without them;
//   - OVL-002: a burst of more requests than the API's connection pool answers
//     each one with its result or 503 "busy; retry", never another failure.
//
// It reads the loaded fixture and changes nothing; run it after load and
// before check (check's retention phase expires the raw data). The result is
// also written to results/overload.md.
func overload(ctx context.Context, o options) error {
	st, err := loadState(o.statePath)
	if err != nil {
		return err
	}
	c := &checker{api: newAPI(o.api), d: st.dataset(), state: st, statePath: o.statePath, notes: map[string]string{}}
	if err := c.waitReady(ctx, o); err != nil {
		return err
	}
	light := c.wide()
	light.ServiceName, light.Scope, light.RowsPerPage = "fx"+st.Run, "all", 25
	request := func(ctx context.Context) error {
		_, _, err := c.api.spans(ctx, light)
		return err
	}

	baseline, err := measure(ctx, request, 20, 1)
	if err != nil {
		return fmt.Errorf("baseline: %w", err)
	}

	hogCtx, stopHogs := context.WithCancel(ctx)
	var hogs sync.WaitGroup
	for i := 0; i < overloadHogs; i++ {
		hogs.Add(1)
		go func() {
			defer hogs.Done()
			for hogCtx.Err() == nil {
				_ = exec.CommandContext(hogCtx, "docker", "exec", o.container, "clickhouse-client",
					"--priority", "10", "--max_execution_time", "60",
					"--query", "SELECT /* overload-hog */ sum(cityHash64(number)) FROM numbers_mt(100000000000) FORMAT Null").Run()
			}
		}()
	}
	defer func() {
		stopHogs()
		killHogs(o.container)
		hogs.Wait()
	}()
	if err := waitFor(ctx, "the heavy queries", 30*time.Second, func() error {
		n, err := runningHogs(ctx, o.container)
		if err == nil && n < overloadHogs {
			err = fmt.Errorf("%d of %d running", n, overloadHogs)
		}
		return err
	}); err != nil {
		return err
	}

	loaded, err := measure(ctx, request, 40, 4)
	if err != nil {
		return fmt.Errorf("OVL-001: a light request failed under load: %w", err)
	}
	burst := burstStatuses(ctx, func(ctx context.Context) error {
		_, err := c.api.count(ctx, light)
		return err
	}, 3*overloadAPIConns)

	report := fmt.Sprintf("## Overload\n\n%d heavy low-priority queries ran in ClickHouse.\n\n"+
		"| | requests | p50 | p95 | max |\n|---|---|---|---|---|\n"+
		"| without load | %d | %s | %s | %s |\n| under load | %d | %s | %s | %s |\n\n"+
		"Burst of %d count requests: %s.\n",
		overloadHogs, baseline.n, baseline.p50, baseline.p95, baseline.max, loaded.n, loaded.p50, loaded.p95, loaded.max,
		3*overloadAPIConns, burst.summary())
	fmt.Print(report)
	if err := os.WriteFile(filepath.Join(filepath.Dir(o.reportPath), "overload.md"), []byte(report), 0o644); err != nil {
		return err
	}

	limit := 3*baseline.p95 + 250*time.Millisecond
	if loaded.p95 > limit {
		return fmt.Errorf("OVL-001: under load light requests took p95 %s, over %s (3 × %s without load + 250 ms): interactive queries are not running ahead of background work",
			loaded.p95, limit, baseline.p95)
	}
	if len(burst.other) > 0 {
		return fmt.Errorf("OVL-002: a burst of %d requests got failures other than \"busy; retry\": %s", 3*overloadAPIConns, strings.Join(burst.other, "; "))
	}
	fmt.Println("OVL-001, OVL-002 passed")
	return nil
}

// overloadHogs heavy queries keep every ClickHouse core busy.
const overloadHogs = 3

// overloadAPIConns is the API's interactive pool (CLICKHOUSE_MAX_QUERIES,
// default 4).
const overloadAPIConns = 4

type latencies struct {
	n             int
	p50, p95, max time.Duration
}

// measure runs request n times with workers at once and returns the spread.
func measure(ctx context.Context, request func(context.Context) error, n, workers int) (latencies, error) {
	var mu sync.Mutex
	var took []time.Duration
	var firstErr error
	jobs := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		jobs <- struct{}{}
	}
	close(jobs)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				start := time.Now()
				err := request(ctx)
				mu.Lock()
				took = append(took, time.Since(start))
				if err != nil && firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return latencies{}, firstErr
	}
	sort.Slice(took, func(i, j int) bool { return took[i] < took[j] })
	at := func(q float64) time.Duration { return took[int(q*float64(len(took)-1))].Round(time.Millisecond) }
	return latencies{n: len(took), p50: at(0.5), p95: at(0.95), max: took[len(took)-1].Round(time.Millisecond)}, nil
}

type burstResult struct {
	ok, busy int
	other    []string
}

func (b burstResult) summary() string {
	return fmt.Sprintf("%d answered, %d \"busy; retry\", %d other failures", b.ok, b.busy, len(b.other))
}

// burstStatuses sends n requests at once.
func burstStatuses(ctx context.Context, request func(context.Context) error, n int) burstResult {
	var mu sync.Mutex
	var res burstResult
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := request(ctx)
			mu.Lock()
			defer mu.Unlock()
			var ae *apiError
			switch {
			case err == nil:
				res.ok++
			case errors.As(err, &ae) && ae.Status == 503 && strings.Contains(ae.Message, "busy"):
				res.busy++
			default:
				res.other = append(res.other, err.Error())
			}
		}()
	}
	wg.Wait()
	return res
}

func runningHogs(ctx context.Context, container string) (int, error) {
	out, err := exec.CommandContext(ctx, "docker", "exec", container, "clickhouse-client", "--query",
		"SELECT count() FROM system.processes WHERE query LIKE '%overload-hog%' AND query NOT LIKE '%system.processes%'").Output()
	if err != nil {
		return 0, fmt.Errorf("count the heavy queries: %w", err)
	}
	var n int
	_, err = fmt.Sscan(strings.TrimSpace(string(out)), &n)
	return n, err
}

// killHogs stops the heavy queries in ClickHouse (closing the client alone
// may leave them running until their timeout).
func killHogs(container string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "docker", "exec", container, "clickhouse-client", "--query",
		"KILL QUERY WHERE query LIKE '%overload-hog%' AND query NOT LIKE '%KILL QUERY%' SYNC FORMAT Null").Run()
}
