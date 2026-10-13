package driver

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{"72h": 72 * time.Hour, "7d": 7 * 24 * time.Hour, "90m": 90 * time.Minute} {
		got, err := ParseDuration(in)
		if err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %s, %v; want %s", in, got, err, want)
		}
	}
	if _, err := ParseDuration("x d"); err == nil {
		t.Error("ParseDuration accepted a malformed day count")
	}
}

func TestRetentionCheckRejectsRawShorterThanRebuild(t *testing.T) {
	// The fixture policy: 48 h raw, 30 days statistics, 24 h lateness.
	ok := Retention{Traces: 48 * time.Hour, Logs: 48 * time.Hour, Stats: 30 * 24 * time.Hour}
	if err := ok.Check(24 * time.Hour); err != nil {
		t.Fatalf("fixture policy rejected: %v", err)
	}
	// 1 h bucket + 24 h lateness + 1 h margin = 26 h: raw retention must exceed it.
	short := ok
	short.Logs = 26 * time.Hour
	if err := short.Check(24 * time.Hour); err == nil {
		t.Fatal("raw log retention of 26h was accepted with 24h lateness")
	}
}

func TestTTLSeconds(t *testing.T) {
	cases := map[string]int64{
		"MergeTree PARTITION BY x ORDER BY UnixTime TTL toDateTime(fromUnixTimestamp64Nano(UnixTime)) + toIntervalDay(3) SETTINGS a = 1": 3 * 86400,
		"MergeTree ORDER BY BucketStart TTL BucketStart + toIntervalSecond(2592000)":                                                     2592000,
		"MergeTree ORDER BY UnixTime": -1,
	}
	for engine, want := range cases {
		if got := TTLSeconds(engine); got != want {
			t.Errorf("TTLSeconds(%q) = %d, want %d", engine, got, want)
		}
	}
}
