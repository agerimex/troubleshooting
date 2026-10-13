package driver

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Retention is how long each store keeps data. Raw traces and logs expire by
// their event time; statistics by their bucket start. Removal is done by
// ClickHouse in the background (whole parts, ttl_only_drop_parts), so data
// can stay readable for a while after it expired logically.
type Retention struct {
	Traces time.Duration
	Logs   time.Duration
	Stats  time.Duration
}

// DefaultRetention keeps the 3-day raw retention of earlier versions and 30
// days of statistics.
var DefaultRetention = Retention{Traces: 72 * time.Hour, Logs: 72 * time.Hour, Stats: 30 * 24 * time.Hour}

// Statistics are built per one-hour bucket. A bucket can be rebuilt from raw
// records until its allowed lateness has passed, plus a margin for the
// rebuild itself, so raw retention must be longer than all three together.
const (
	StatsBucket        = time.Hour
	StatsRebuildMargin = time.Hour
)

// DefaultAllowedLateness is how long after a bucket ends a record may still
// arrive and be counted in it (STATS_ALLOWED_LATENESS).
const DefaultAllowedLateness = 24 * time.Hour

// ParseDuration accepts Go durations (72h, 90m) and whole days (7d).
func ParseDuration(value string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(value, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(value)
}

// RetentionFromEnv reads TRACE_RETENTION, LOG_RETENTION, STATS_RETENTION and
// STATS_ALLOWED_LATENESS, and rejects a raw retention too short to rebuild a
// bucket before its records expire. Values are whole seconds or more.
func RetentionFromEnv() (Retention, time.Duration, error) {
	r := DefaultRetention
	lateness := DefaultAllowedLateness
	for _, d := range []struct {
		env string
		dst *time.Duration
	}{{"TRACE_RETENTION", &r.Traces}, {"LOG_RETENTION", &r.Logs}, {"STATS_RETENTION", &r.Stats}, {"STATS_ALLOWED_LATENESS", &lateness}} {
		value := os.Getenv(d.env)
		if value == "" {
			continue
		}
		parsed, err := ParseDuration(value)
		if err != nil || parsed < time.Second {
			return r, lateness, fmt.Errorf("%s=%q: want a duration such as 72h or 30d", d.env, value)
		}
		*d.dst = parsed.Truncate(time.Second)
	}
	return r, lateness, r.Check(lateness)
}

// Check rejects a raw retention that doesn't exceed bucket + allowed lateness
// + rebuild margin: a bucket could lose raw records before it is final.
func (r Retention) Check(lateness time.Duration) error {
	need := StatsBucket + lateness + StatsRebuildMargin
	for _, raw := range []struct {
		name string
		d    time.Duration
	}{{"TRACE_RETENTION", r.Traces}, {"LOG_RETENTION", r.Logs}} {
		if raw.d <= need {
			return fmt.Errorf("%s=%s must be longer than the statistics bucket (%s) + allowed lateness (%s) + rebuild margin (%s) = %s",
				raw.name, raw.d, StatsBucket, lateness, StatsRebuildMargin, need)
		}
	}
	if r.Stats < StatsBucket {
		return fmt.Errorf("STATS_RETENTION=%s must be at least one bucket (%s)", r.Stats, StatsBucket)
	}
	return nil
}
