package data

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// StatsConfig is the statistics policy of this installation.
type StatsConfig struct {
	// AllowedLateness is how long after a bucket ends a record may arrive
	// and still count in it (STATS_ALLOWED_LATENESS).
	AllowedLateness time.Duration
	// RefreshInterval is how often the refresher rebuilds changed buckets.
	RefreshInterval time.Duration
	// DefaultSampling applies to spans that declare no sampling policy
	// (SAMPLING_POLICY_DEFAULT); empty means unknown.
	DefaultSampling string
}

// Statistics buckets are one UTC hour. A bucket is rebuilt only while every
// raw record it can contain is retained, with this margin.
const (
	StatsBucket        = time.Hour
	StatsRebuildMargin = time.Hour
)

var DefaultStatsConfig = StatsConfig{AllowedLateness: 24 * time.Hour, RefreshInterval: time.Minute}

var statsConfig = DefaultStatsConfig

// SetStatsConfig sets the statistics policy.
func SetStatsConfig(c StatsConfig) { statsConfig = c }

// Clock is the application's notion of now for retention, expiry and
// finalization. Arrival times (ReceivedTime) are always real. Tests can move
// it forward (TEST_CLOCK_CONTROL) to finalize buckets or expire evidence
// without waiting days.
type Clock struct {
	mu     sync.Mutex
	offset time.Duration
}

var clock = &Clock{}

// Now is the real time plus the test offset.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Now().Add(c.offset)
}

// Offset returns the test offset (zero in normal operation).
func (c *Clock) Offset() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.offset
}

// SetOffset moves the clock; only for test installations.
func (c *Clock) SetOffset(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset = d
}

// AppClock is the clock statistics use.
func AppClock() *Clock { return clock }

// EffectivePolicy is the retention ClickHouse really applies (read from the
// table definitions, so it can't drift from the configuration), and the
// statistics settings.
type EffectivePolicy struct {
	TracesSeconds    int64  `json:"tracesSeconds"`
	LogsSeconds      int64  `json:"logsSeconds"`
	StatsSeconds     int64  `json:"statsSeconds"`
	BucketSeconds    int64  `json:"bucketSeconds"`
	LatenessSeconds  int64  `json:"latenessSeconds"`
	MarginSeconds    int64  `json:"marginSeconds"`
	Compatible       bool   `json:"compatible"`
	Problem          string `json:"problem,omitempty"`
	DefaultSampling  string `json:"defaultSampling"`
	ClockOffsetSec   int64  `json:"clockOffsetSeconds"`
	PhysicalDeletion string `json:"physicalDeletion"`
}

var (
	policyMu sync.Mutex
	policy   = EffectivePolicy{}
)

// CurrentPolicy returns the last policy read from ClickHouse.
func CurrentPolicy() EffectivePolicy {
	policyMu.Lock()
	defer policyMu.Unlock()
	p := policy
	p.ClockOffsetSec = int64(clock.Offset().Seconds())
	return p
}

// LoadPolicy reads the TTLs of the raw and statistics tables, checks that
// raw retention allows rebuilding a bucket until it is final, and makes them
// the retention the trace report and statistics use.
func LoadPolicy(ctx context.Context) (EffectivePolicy, error) {
	ttl := map[string]int64{}
	rows, err := clickhouseDB.Query(ctx, `SELECT name, engine_full FROM system.tables
		WHERE database = currentDatabase() AND name IN ('otel_traces', 'log_records', 'stat_calls')`)
	if err != nil {
		return EffectivePolicy{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, engine string
		if err := rows.Scan(&name, &engine); err != nil {
			return EffectivePolicy{}, err
		}
		ttl[name] = ttlSeconds(engine)
	}
	if err := rows.Err(); err != nil {
		return EffectivePolicy{}, err
	}
	p := EffectivePolicy{
		TracesSeconds:    ttl["otel_traces"],
		LogsSeconds:      ttl["log_records"],
		StatsSeconds:     ttl["stat_calls"],
		BucketSeconds:    int64(StatsBucket.Seconds()),
		LatenessSeconds:  int64(statsConfig.AllowedLateness.Seconds()),
		MarginSeconds:    int64(StatsRebuildMargin.Seconds()),
		DefaultSampling:  statsConfig.DefaultSampling,
		PhysicalDeletion: "Expired records are removed by ClickHouse in the background, whole parts at a time; they can remain readable for a while after they expired logically, and deleted records can't be restored by a longer retention.",
	}
	p.Compatible, p.Problem = true, ""
	need := p.BucketSeconds + p.LatenessSeconds + p.MarginSeconds
	switch {
	case p.TracesSeconds <= 0 || p.LogsSeconds <= 0 || p.StatsSeconds <= 0:
		p.Compatible, p.Problem = false, "the statistics tables or raw retention are missing; start the receiver of this version first"
	case p.TracesSeconds <= need || p.LogsSeconds <= need:
		p.Compatible, p.Problem = false, fmt.Sprintf("raw retention must be longer than bucket + allowed lateness + rebuild margin (%s)", time.Duration(need)*time.Second)
	}
	policyMu.Lock()
	policy = p
	policyMu.Unlock()
	if p.TracesSeconds > 0 && p.LogsSeconds > 0 {
		SetRetention(RetentionPolicy{Traces: time.Duration(p.TracesSeconds) * time.Second, Logs: time.Duration(p.LogsSeconds) * time.Second})
	}
	return p, nil
}

// ttlSeconds parses the interval of the first TTL of an engine_full
// definition (toIntervalSecond/Hour/Day); -1 if there is none.
func ttlSeconds(engine string) int64 {
	i := strings.Index(engine, "TTL ")
	if i < 0 {
		return -1
	}
	rest := engine[i:]
	for unit, seconds := range map[string]int64{"toIntervalSecond(": 1, "toIntervalHour(": 3600, "toIntervalDay(": 86400} {
		j := strings.Index(rest, unit)
		if j < 0 {
			continue
		}
		end := strings.Index(rest[j:], ")")
		if end < 0 {
			continue
		}
		if n, err := strconv.ParseInt(rest[j+len(unit):j+end], 10, 64); err == nil {
			return n * seconds
		}
	}
	return -1
}

// rawHorizon is the oldest event time whose raw records of a source are
// still retained, by the application clock (a record can stay readable
// longer, until ClickHouse removes it).
func rawHorizon(now time.Time, seconds int64) int64 {
	if seconds <= 0 {
		return now.UnixNano()
	}
	return now.Add(-time.Duration(seconds) * time.Second).UnixNano()
}

// statsHorizon is the oldest bucket start whose statistics are retained.
func statsHorizon(now time.Time, p EffectivePolicy) int64 {
	return rawHorizon(now, p.StatsSeconds)
}

// coverage declarations

// Coverage statuses.
const (
	CoverageComplete = "complete" // every minute is covered by a heartbeat or a declaration
	CoverageGap      = "gap"      // a declared gap or minutes without heartbeat
	CoverageUnknown  = "unknown"  // nothing is known about collection in this range
	CoverageOpen     = "open"     // the range is not over yet
)

// Declaration records coverage the heartbeats can't know.
type Declaration struct {
	Source   string `json:"source"` // all, traces or logs
	TimeFrom string `json:"time_from"`
	TimeTo   string `json:"time_to"`
	Status   string `json:"status"` // complete or gap
	Reason   string `json:"reason"`
}

// Validate checks a declaration.
func (d *Declaration) Validate() error {
	if err := validateWindow(d.TimeFrom, d.TimeTo); err != nil && !strings.Contains(err.Error(), "longer than") {
		return err
	}
	if d.Source == "" {
		d.Source = "all"
	}
	if d.Source != "all" && d.Source != "traces" && d.Source != "logs" {
		return invalid("source must be all, traces or logs")
	}
	if d.Status != CoverageComplete && d.Status != CoverageGap {
		return invalid("status must be complete or gap")
	}
	if strings.TrimSpace(d.Reason) == "" {
		return invalid("reason is required: a declaration must say why")
	}
	return nil
}

// Declare stores a coverage declaration.
func Declare(ctx context.Context, d Declaration) error {
	from, _ := strconv.ParseInt(d.TimeFrom, 10, 64)
	to, _ := strconv.ParseInt(d.TimeTo, 10, 64)
	return clickhouseDB.Exec(ctx, `INSERT INTO coverage_declarations ("DeclaredAt", "Source", "From", "To", "Status", "Reason")
		VALUES ({at:Int64}, {source:String}, {from:Int64}, {to:Int64}, {status:String}, {reason:String})`,
		clickhouse.Named("at", strconv.FormatInt(time.Now().UnixNano(), 10)),
		clickhouse.Named("source", d.Source),
		clickhouse.Named("from", strconv.FormatInt(from, 10)),
		clickhouse.Named("to", strconv.FormatInt(to, 10)),
		clickhouse.Named("status", d.Status),
		clickhouse.Named("reason", d.Reason))
}

// coverageEvidence is what is known about collection around a range.
type coverageEvidence struct {
	minutes      map[int64]bool // minute starts (Unix seconds) with a heartbeat
	declarations []declared
}

type declared struct {
	from, to int64
	source   string
	status   string
	reason   string
}

// loadCoverage reads heartbeats and declarations overlapping [from, to).
func loadCoverage(ctx context.Context, from, to int64) (coverageEvidence, error) {
	ev := coverageEvidence{minutes: map[int64]bool{}}
	rows, err := clickhouseDB.Query(withLimits(ctx), `SELECT DISTINCT toUnixTimestamp(toStartOfMinute("Time")) FROM collection_heartbeats
		WHERE "Time" >= toDateTime(`+num(from/1e9-60)+`, 'UTC') AND "Time" < toDateTime(`+num(to/1e9+60)+`, 'UTC')`)
	if err != nil {
		return ev, err
	}
	for rows.Next() {
		var m uint32
		if err := rows.Scan(&m); err != nil {
			rows.Close()
			return ev, err
		}
		ev.minutes[int64(m)] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ev, err
	}
	rows, err = clickhouseDB.Query(withLimits(ctx), `SELECT "From", "To", "Source", "Status", "Reason" FROM coverage_declarations
		WHERE "From" < `+num(to)+` AND "To" > `+num(from)+` ORDER BY "DeclaredAt"`)
	if err != nil {
		return ev, err
	}
	defer rows.Close()
	for rows.Next() {
		var d declared
		if err := rows.Scan(&d.from, &d.to, &d.source, &d.status, &d.reason); err != nil {
			return ev, err
		}
		ev.declarations = append(ev.declarations, d)
	}
	return ev, rows.Err()
}

// CoverageAssessment says how completely a range was collected.
type CoverageAssessment struct {
	Status  string   `json:"status"`
	Covered int      `json:"coveredMinutes"`
	Minutes int      `json:"minutes"`
	Reasons []string `json:"reasons"`
}

// assess judges [from, to) for a source (traces or logs) at now: a declared
// gap makes it a gap; a minute is covered by a heartbeat or a complete
// declaration; uncovered minutes after collection started are a gap; with
// nothing known at all it is unknown. Minutes after now are open.
func (ev coverageEvidence) assess(from, to int64, source string, now time.Time) CoverageAssessment {
	a := CoverageAssessment{Reasons: []string{}}
	end := to
	open := false
	if now.UnixNano() < end {
		end = now.UnixNano()
		open = true
	}
	applies := func(d declared) bool { return d.source == "all" || d.source == source }
	gap := false
	for _, d := range ev.declarations {
		if applies(d) && d.status == CoverageGap && d.from < to && d.to > from {
			gap = true
			a.Reasons = append(a.Reasons, "declared collection gap: "+d.reason)
		}
	}
	for m := (from / 1e9 / 60) * 60; m*1e9 < end; m += 60 {
		a.Minutes++
		if ev.minutes[m] {
			a.Covered++
			continue
		}
		for _, d := range ev.declarations {
			if applies(d) && d.status == CoverageComplete && d.from <= m*1e9 && d.to >= (m+60)*1e9 {
				a.Covered++
				break
			}
		}
	}
	declaredComplete := false
	for _, d := range ev.declarations {
		if applies(d) && d.status == CoverageComplete && d.from < to && d.to > from {
			declaredComplete = true
			a.Reasons = append(a.Reasons, "declared complete: "+d.reason)
		}
	}
	switch {
	case gap:
		a.Status = CoverageGap
	case a.Minutes > 0 && a.Covered == a.Minutes && open:
		a.Status = CoverageOpen
		a.Reasons = append(a.Reasons, "the window has not ended yet")
	case a.Minutes > 0 && a.Covered == a.Minutes:
		a.Status = CoverageComplete
	case a.Covered == 0 && !declaredComplete:
		a.Status = CoverageUnknown
		a.Reasons = append(a.Reasons, "no receiver heartbeat or coverage declaration for this range")
	default:
		a.Status = CoverageGap
		a.Reasons = append(a.Reasons, fmt.Sprintf("%d of %d minutes have no receiver heartbeat", a.Minutes-a.Covered, a.Minutes))
	}
	return a
}

// combineCoverage is the weakest status of several ranges.
func combineCoverage(parts []CoverageAssessment) CoverageAssessment {
	rank := map[string]int{CoverageComplete: 0, CoverageOpen: 1, CoverageUnknown: 2, CoverageGap: 3}
	out := CoverageAssessment{Status: CoverageComplete, Reasons: []string{}}
	seen := map[string]bool{}
	for _, p := range parts {
		out.Minutes += p.Minutes
		out.Covered += p.Covered
		if rank[p.Status] > rank[out.Status] {
			out.Status = p.Status
		}
		for _, r := range p.Reasons {
			if !seen[r] {
				seen[r] = true
				out.Reasons = append(out.Reasons, r)
			}
		}
	}
	return out
}

// policyFor returns the effective policy, reading it first if it couldn't be
// read yet (the API can start before the receiver created the tables).
func policyFor(ctx context.Context) EffectivePolicy {
	p := CurrentPolicy()
	if p.TracesSeconds > 0 && p.LogsSeconds > 0 && p.StatsSeconds > 0 {
		return p
	}
	if loaded, err := LoadPolicy(ctx); err == nil {
		loaded.ClockOffsetSec = p.ClockOffsetSec
		return loaded
	}
	return p
}
