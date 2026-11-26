package data

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// QueryLimits bound the work one investigation query may do in ClickHouse.
// A row cap on the result alone doesn't bound the rows scanned, so every
// query also carries these settings. Exceeding a limit is an error ("throw"),
// never a silently partial result.
type QueryLimits struct {
	// Timeout is ClickHouse's max_execution_time; the API also cancels the
	// query when the HTTP request ends.
	Timeout time.Duration
	// MaxRowsToRead is max_rows_to_read (counted per query, subqueries included).
	MaxRowsToRead uint64
	// MaxMemoryBytes is max_memory_usage.
	MaxMemoryBytes uint64
	// MaxWindow is the widest time_from..time_to an investigation may search.
	MaxWindow time.Duration
}

// DefaultQueryLimits suit a small installation; the window covers the 3-day
// retention with a day to spare for clock differences.
var DefaultQueryLimits = QueryLimits{
	Timeout:        10 * time.Second,
	MaxRowsToRead:  50_000_000,
	MaxMemoryBytes: 1 << 30,
	MaxWindow:      96 * time.Hour,
}

var limits = DefaultQueryLimits

func (q QueryLimits) settings() clickhouse.Settings {
	return clickhouse.Settings{
		"max_execution_time":    int(q.Timeout.Seconds()),
		"timeout_overflow_mode": "throw",
		"max_rows_to_read":      q.MaxRowsToRead,
		"read_overflow_mode":    "throw",
		"max_memory_usage":      q.MaxMemoryBytes,
	}
}

// withLimits attaches the query limits and the priority of ctx's kind of
// work (workload.go) to ctx for the ClickHouse driver.
func withLimits(ctx context.Context) context.Context {
	return clickhouse.Context(ctx, clickhouse.WithSettings(querySettings(ctx)))
}

func querySettings(ctx context.Context) clickhouse.Settings {
	settings := limits.settings()
	settings["priority"] = priorityOf(ctx)
	return settings
}

// ClickHouse exception codes for the limits above.
const (
	chTimeoutExceeded     = 159
	chTooManyRows         = 158
	chMemoryLimitExceeded = 241
)

// ErrQueryLimit means a query hit a configured limit. The result is not
// returned at all, so it can't be mistaken for a complete one.
var ErrQueryLimit = errors.New("query limit exceeded")

// ErrUnavailable means ClickHouse could not be reached at all.
var ErrUnavailable = errors.New("the database is unavailable")

// ExplainQueryError turns a limit or deadline error into ErrQueryLimit with a
// message that names the limit; other errors are returned unchanged. ctx is
// the request's context: when its deadline passed, the driver reports only a
// network timeout. A failure to connect becomes ErrUnavailable.
func ExplainQueryError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	var dnsErr *net.DNSError
	var opErr *net.OpError
	if errors.As(err, &dnsErr) || (errors.As(err, &opErr) && opErr.Op == "dial") {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	// Every connection of the pool stayed busy (CLICKHOUSE_MAX_QUERIES queries
	// ran) for the whole dial timeout: the API is busy, a retry may succeed.
	if errors.Is(err, clickhouse.ErrAcquireConnTimeout) {
		return fmt.Errorf("%w: every database connection was busy; retry", ErrUnavailable)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w: the query took longer than %s; narrow the time window or filters", ErrQueryLimit, limits.Timeout)
	}
	var ex *clickhouse.Exception
	if errors.As(err, &ex) {
		switch ex.Code {
		case chTimeoutExceeded:
			return fmt.Errorf("%w: the query took longer than %s; narrow the time window or filters", ErrQueryLimit, limits.Timeout)
		case chTooManyRows:
			return fmt.Errorf("%w: the query would read more than %d rows; narrow the time window or filters", ErrQueryLimit, limits.MaxRowsToRead)
		case chMemoryLimitExceeded:
			if serverMemoryLimit(ex.Message) {
				return fmt.Errorf("%w: the database server is short of memory right now (its total limit, not this query's); retry", ErrUnavailable)
			}
			return fmt.Errorf("%w: the query needs more than %d MiB of memory; narrow the time window or filters", ErrQueryLimit, limits.MaxMemoryBytes>>20)
		}
	}
	return err
}

// serverMemoryLimit says whether a MEMORY_LIMIT_EXCEEDED came from the server's
// (or the user's) total limit rather than this query's own: the server then
// stops some query, often a small one, when all of them together reach the
// limit. Narrowing the query doesn't help; a retry usually does.
func serverMemoryLimit(message string) bool {
	m := strings.ToLower(message)
	return strings.Contains(m, "(total)") || strings.Contains(m, "(for user)") || strings.Contains(m, "overcommittracker")
}
