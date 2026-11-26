package data

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Two kinds of work share ClickHouse: interactive requests, which a person
// waits for, and the background statistics refresh. They are kept apart so a
// heavy refresh can't make the UI wait:
//   - separate connection pools: background work holds at most its own pool,
//     so it can never take the connections requests need (and an interactive
//     burst can't take the refresher's);
//   - ClickHouse priorities: while a query of higher priority runs, queries of
//     lower priority are paused, so a request runs ahead of a refresh;
//   - one memory limit per query, a share of the server's memory, so no single
//     query can take the memory the others need (main.go, MemoryShare).
//
// A context marked with Background routes its queries to the background pool
// and gives them background priority; everything else is interactive.

var backgroundDB driver.Conn

// SetBackgroundConn sets the pool of background work (nil: use the main one).
func SetBackgroundConn(conn driver.Conn) { backgroundDB = conn }

type backgroundKey struct{}

// Background marks ctx as background work (the statistics refresh).
func Background(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundKey{}, true)
}

func isBackground(ctx context.Context) bool {
	background, _ := ctx.Value(backgroundKey{}).(bool)
	return background
}

// dbFor is the pool a query of ctx runs on.
func dbFor(ctx context.Context) driver.Conn {
	if isBackground(ctx) && backgroundDB != nil {
		return backgroundDB
	}
	return clickhouseDB
}

// ClickHouse's priority setting: 1 is the highest, a larger number is lower,
// and 0 disables priorities. Lower-priority queries pause while a
// higher-priority one runs.
const (
	interactivePriority = 1
	backgroundPriority  = 10
)

func priorityOf(ctx context.Context) int {
	if isBackground(ctx) {
		return backgroundPriority
	}
	return interactivePriority
}

// ServerMemoryLimit reads the memory limit ClickHouse enforces for all its
// queries together (max_server_memory_usage as the server computed it from
// its configuration and the RAM it sees, container limits included).
func ServerMemoryLimit(ctx context.Context, conn driver.Conn) (uint64, error) {
	var value string
	err := conn.QueryRow(ctx, `SELECT value FROM system.server_settings WHERE name = 'max_server_memory_usage'`).Scan(&value)
	if err != nil {
		return 0, fmt.Errorf("read the server's memory limit: %w", err)
	}
	var limit uint64
	if _, err := fmt.Sscan(value, &limit); err != nil || limit == 0 {
		return 0, fmt.Errorf("the server's memory limit is %q", value)
	}
	return limit, nil
}

// MemoryShare is the memory limit of one query when the server has
// serverLimit for everything and the API runs at most maxQueries interactive
// queries plus one background query at a time: an equal share each, so even
// all of them at their limit stay under the server's, capped at max (the
// configured default) and never below 32 MiB.
func MemoryShare(serverLimit uint64, maxQueries int, max uint64) uint64 {
	share := serverLimit / uint64(maxQueries+1)
	if share > max {
		share = max
	}
	if share < minQueryMemory {
		share = minQueryMemory
	}
	return share
}

const minQueryMemory = 32 << 20
