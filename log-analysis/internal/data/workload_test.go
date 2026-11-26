package data

import (
	"context"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func TestInteractiveQueriesRunAheadOfBackgroundOnes(t *testing.T) {
	ctx := context.Background()
	if p := querySettings(ctx)["priority"]; p != interactivePriority {
		t.Errorf("an interactive query has priority %v, want %d", p, interactivePriority)
	}
	bg := Background(ctx)
	for name, s := range map[string]clickhouse.Settings{"querySettings": querySettings(bg), "refreshSettings": refreshSettings(bg)} {
		if p := s["priority"]; p != backgroundPriority {
			t.Errorf("%s in background work: priority %v, want %d", name, p, backgroundPriority)
		}
	}
	// ClickHouse: 1 is the highest priority, a larger number is lower.
	if !(interactivePriority >= 1 && backgroundPriority > interactivePriority) {
		t.Errorf("priorities %d (interactive) and %d (background) don't put requests first", interactivePriority, backgroundPriority)
	}
	// A context derived from a background one stays background.
	derived, cancel := context.WithCancel(bg)
	defer cancel()
	if !isBackground(derived) {
		t.Error("a context derived from background work lost its kind")
	}
}

type namedConn struct {
	driver.Conn
	name string
}

func TestBackgroundWorkUsesItsOwnPool(t *testing.T) {
	prevMain, prevBg := clickhouseDB, backgroundDB
	defer func() { clickhouseDB, backgroundDB = prevMain, prevBg }()
	clickhouseDB = namedConn{name: "interactive"}
	backgroundDB = nil
	if got := dbFor(Background(context.Background())).(namedConn).name; got != "interactive" {
		t.Errorf("without a background pool, background work uses %q", got)
	}
	SetBackgroundConn(namedConn{name: "background"})
	if got := dbFor(context.Background()).(namedConn).name; got != "interactive" {
		t.Errorf("an interactive query uses the %q pool", got)
	}
	if got := dbFor(Background(context.Background())).(namedConn).name; got != "background" {
		t.Errorf("background work uses the %q pool", got)
	}
}

func TestMemoryShare(t *testing.T) {
	const mib = 1 << 20
	for _, tc := range []struct {
		server     uint64
		maxQueries int
		max, want  uint64
	}{
		{430_000_000, 4, 1 << 30, 86_000_000},   // the 1 GB demo server: a fifth each
		{64 << 30, 4, 1 << 30, 1 << 30},         // a big server: the configured default caps it
		{100 * mib, 8, 1 << 30, minQueryMemory}, // never below the floor
	} {
		if got := MemoryShare(tc.server, tc.maxQueries, tc.max); got != tc.want {
			t.Errorf("MemoryShare(%d, %d, %d) = %d, want %d", tc.server, tc.maxQueries, tc.max, got, tc.want)
		}
	}
}
