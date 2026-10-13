package data

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// The refresher keeps statistics of completed buckets. For each bucket it
// recomputes the whole bucket from canonical raw records (bounded
// recomputation, not incremental views, so duplicates and replays can't add
// up) and stores the result under a new revision:
//
//  1. insert the stat_calls and stat_errors rows of the new revision;
//  2. insert its stat_revisions row, the commit marker.
//
// Readers only use committed revisions, so a refresh that fails before step 2
// leaves the previous revision in place and its rows are never read. A bucket
// whose allowed lateness has passed is committed as final and never replaced:
// later arrivals and replays don't change it (they are reported as excluded).
// A bucket is only (re)built while every raw record it can contain is
// retained; older history is never rebuilt from a surviving subset.

// Bucket outcomes.
const (
	OutcomeRevised       = "revised"       // a new revision was committed
	OutcomeFinalized     = "finalized"     // the unchanged revision was committed as final
	OutcomeUnchanged     = "unchanged"     // the raw content didn't change
	OutcomeFinal         = "final"         // already final; never replaced
	OutcomeExtended      = "extended"      // a final revision got the statistics of a newer definition (execution shapes)
	OutcomeUnrecoverable = "unrecoverable" // raw records are no longer complete
	OutcomeOpen          = "open"          // the bucket has not ended
	OutcomeFailed        = "failed"        // the previous revision stays
)

// BucketOutcome is the result of refreshing one bucket.
type BucketOutcome struct {
	BucketStart string `json:"bucketStart"`
	Outcome     string `json:"outcome"`
	Revision    int64  `json:"revision,omitempty"`
	State       string `json:"state,omitempty"`
	Calls       uint64 `json:"calls"`
	Error       string `json:"error,omitempty"`
}

// Refresher rebuilds statistics buckets.
type Refresher struct {
	mu        sync.Mutex
	running   sync.Mutex
	lastRun   time.Time
	lastError string
	failures  map[int64]string // bucket start -> last failure, cleared on success
	// fingerprints are the raw fingerprints of buckets found unchanged or
	// rebuilt, so an unchanged bucket isn't hashed again every cycle.
	fingerprints map[int64]rawFingerprintValue
	// failNext makes the next n commits fail after the rows were written
	// (TEST_CLOCK_CONTROL installations only), to show a failed refresh.
	failNext int
}

var refresher = &Refresher{failures: map[int64]string{}, fingerprints: map[int64]rawFingerprintValue{}}

// StatsRefresher returns the refresher.
func StatsRefresher() *Refresher { return refresher }

// FailNext makes the next n bucket commits fail (tests).
func (r *Refresher) FailNext(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failNext = n
}

// Status describes the refresher for the API.
type RefresherStatus struct {
	LastRun   string            `json:"lastRun"`
	LastError string            `json:"lastError,omitempty"`
	Failures  map[string]string `json:"failures"` // bucket start (ns) -> error of its last failed refresh
	Policy    EffectivePolicy   `json:"policy"`
}

// Status returns the refresher state.
func (r *Refresher) Status() RefresherStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := RefresherStatus{LastError: r.lastError, Failures: map[string]string{}, Policy: CurrentPolicy()}
	if !r.lastRun.IsZero() {
		s.LastRun = nanos(r.lastRun)
	}
	for b, e := range r.failures {
		s.Failures[strconv.FormatInt(b, 10)] = e
	}
	return s
}

// failure returns the last failed refresh of a bucket, if any.
func (r *Refresher) failure(bucket int64) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failures[bucket]
}

// Run refreshes the rebuildable range every interval until ctx ends.
func (r *Refresher) Run(ctx context.Context, interval time.Duration) {
	for {
		pol, err := LoadPolicy(ctx)
		if err != nil {
			r.setError("read retention: " + err.Error())
		} else if !pol.Compatible {
			r.setError("statistics refresh is off: " + pol.Problem)
		} else {
			now := clock.Now()
			from, _ := rebuildableFrom(now, pol)
			if _, err := r.RefreshRange(ctx, from, time.Now().UnixNano(), "refresh"); err != nil {
				r.setError(err.Error())
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func (r *Refresher) setError(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastError = msg
	log.Println("statistics:", msg)
}

// rebuildableFrom is the start of the oldest bucket all of whose raw records
// (including spans from eventLookback before it) are still retained, with
// the rebuild margin.
func rebuildableFrom(now time.Time, p EffectivePolicy) (int64, bool) {
	if p.TracesSeconds <= 0 || p.LogsSeconds <= 0 {
		return 0, false
	}
	raw := min(p.TracesSeconds, p.LogsSeconds)
	oldest := now.Add(-time.Duration(raw)*time.Second + StatsRebuildMargin + eventLookback).UnixNano()
	return ceilHour(oldest), true
}

// RefreshRange refreshes every bucket starting in [from, to). Buckets that
// haven't ended (by real time, since no record can have arrived for the
// future) are left open.
func (r *Refresher) RefreshRange(ctx context.Context, from, to int64, trigger string) ([]BucketOutcome, error) {
	r.running.Lock()
	defer r.running.Unlock()

	pol := CurrentPolicy()
	if !pol.Compatible {
		return nil, fmt.Errorf("statistics are off: %s", pol.Problem)
	}
	now := clock.Now()
	oldest, _ := rebuildableFrom(now, pol)
	revisions, err := loadRevisions(ctx, from, to)
	if err != nil {
		return nil, err
	}
	var outcomes []BucketOutcome
	for start := floorHour(from); start < to; start += hourNs {
		out := r.refreshBucket(ctx, start, revisions[start], start >= oldest, now, trigger)
		outcomes = append(outcomes, out)
	}
	r.mu.Lock()
	r.lastRun = time.Now()
	r.lastError = ""
	r.mu.Unlock()
	return outcomes, nil
}

func (r *Refresher) refreshBucket(ctx context.Context, start int64, published Revision, rebuildable bool, now time.Time, trigger string) BucketOutcome {
	out := BucketOutcome{BucketStart: strconv.FormatInt(start, 10)}
	end := start + hourNs
	hasRevision := published.Revision != 0
	if hasRevision {
		out.Revision, out.State, out.Calls = published.Revision, published.State, published.Calls
	}
	switch {
	case end > time.Now().UnixNano():
		out.Outcome = OutcomeOpen
		return out
	case published.State == StateFinal && !strings.Contains(published.Definitions, ShapesContract) && rebuildable:
		return r.addShapes(ctx, out, start, published, trigger)
	case published.State == StateFinal:
		out.Outcome = OutcomeFinal
		return out
	case !rebuildable:
		out.Outcome = OutcomeUnrecoverable
		return out
	}
	state := StateProvisional
	if now.UnixNano() >= end+int64(statsConfig.AllowedLateness) {
		state = StateFinal
	}
	// The fingerprint is read before the content hash, so a row that arrives
	// while the hash is computed changes the next fingerprint.
	fp, fpErr := rawFingerprint(ctx, start)
	remember := func() {
		if fpErr == nil {
			r.mu.Lock()
			r.fingerprints[start] = fp
			r.mu.Unlock()
		}
	}
	cutoff := time.Now().UnixNano()
	var hash uint64
	if fpErr == nil && hasRevision && published.Definitions == Definitions && r.fingerprint(start) == fp {
		// No raw row arrived or expired since this bucket was last found
		// unchanged or rebuilt: its content hash can't have changed.
		hash = published.ContentHash
	} else {
		var err error
		if hash, err = contentHash(ctx, start, cutoff); err != nil {
			return r.failed(out, start, err)
		}
	}
	if hasRevision && published.ContentHash == hash && published.Definitions == Definitions {
		remember()
		if state == StateFinal {
			// The content can't change any more: commit the same rows as final.
			if err := commitRevision(ctx, start, published.Revision, state, published.ReceiptCutoff, hash, published.Calls, published.ErrorEvents, trigger); err != nil {
				return r.failed(out, start, err)
			}
			out.Outcome, out.State = OutcomeFinalized, state
			r.succeeded(start)
			return out
		}
		out.Outcome = OutcomeUnchanged
		r.succeeded(start)
		return out
	}

	revision := time.Now().UnixNano()
	parts := []interval{{start, end}}
	lateness := statsConfig.AllowedLateness
	insertCalls := `INSERT INTO stat_calls SELECT "BucketStart", ` + num(revision) + ` AS "Revision", "ServiceName", "Operation", "SpanKind", "Environment",
		"Version", "Installation", "Segment", "Sampling", "Calls", "Errors", "Ok", "Unset", "ValidDurations", "InvalidDurations", "Conflicts", "Durations"
		FROM (` + rawCalls(parts, cutoff, lateness, "") + `)`
	occurrences := statusOccurrences(parts, cutoff, lateness, "") + ` UNION ALL ` +
		`SELECT "SourceKind", "EventTime", "TraceId", "SpanId", "ServiceName", "Operation", "Environment", "Version", "Installation", "Segment", "ExceptionType", "Frame", "Message" FROM (` +
		exceptionOccurrences(parts, cutoff, lateness, "") + `) UNION ALL ` +
		`SELECT "SourceKind", "EventTime", "TraceId", "SpanId", "ServiceName", "Operation", "Environment", "Version", "Installation", "Segment", "ExceptionType", "Frame", "Message" FROM (` +
		logOccurrences(parts, cutoff, lateness, "") + `)`
	insertErrors := `INSERT INTO stat_errors SELECT "BucketStart", ` + num(revision) + ` AS "Revision", "SourceKind", "ServiceName", "Operation", "Environment",
		"Version", "Installation", "Segment", "Method", "Fingerprint", "ExceptionType", "Frame", "Signature", "Occurrences", "Unlinked", "Traces", "FirstSeen", "LastSeen", "Calls", "HasCalls"
		FROM (` + groupedErrors(occurrences) + `)`
	insertShapes := `INSERT INTO stat_shapes SELECT "BucketStart", ` + num(revision) + ` AS "Revision", ` + shapeColumns + `
		FROM (` + bucketShapes(parts, cutoff, lateness) + `)`
	for _, q := range []string{insertCalls, insertErrors, insertShapes} {
		if err := clickhouseDB.Exec(withRefreshLimits(ctx), q); err != nil {
			return r.failed(out, start, err)
		}
	}
	var calls, errorEvents uint64
	err := clickhouseDB.QueryRow(ctx, `SELECT (SELECT sum("Calls") FROM stat_calls WHERE "BucketStart" = toDateTime(`+num(start/1e9)+`, 'UTC') AND "Revision" = `+num(revision)+`),
		(SELECT sum("Occurrences") FROM stat_errors WHERE "BucketStart" = toDateTime(`+num(start/1e9)+`, 'UTC') AND "Revision" = `+num(revision)+`)`).Scan(&calls, &errorEvents)
	if err != nil {
		return r.failed(out, start, err)
	}
	if r.takeFailure() {
		return r.failed(out, start, fmt.Errorf("simulated failure before commit (test control)"))
	}
	if err := commitRevision(ctx, start, revision, state, cutoff, hash, calls, errorEvents, trigger); err != nil {
		return r.failed(out, start, err)
	}
	out.Outcome, out.Revision, out.State, out.Calls = OutcomeRevised, revision, state, calls
	remember()
	r.succeeded(start)
	return out
}

// rawFingerprint summarizes the raw rows a bucket's content hash reads (the
// same intervals as contentHash): their number and latest receipt time, for
// spans and logs. It reads only the time columns, so it is cheap.
type rawFingerprintValue [4]int64

func rawFingerprint(ctx context.Context, start int64) (fp rawFingerprintValue, err error) {
	spans := windowsCondition(`"UnixTime"`, []interval{{start - int64(eventLookback), start + hourNs + int64(shapeLookahead)}})
	logs := windowsCondition(`"UnixTime"`, []interval{{start, start + hourNs}})
	query := `SELECT (SELECT toInt64(count()) FROM otel_traces WHERE ` + spans + `), (SELECT max("ReceivedTime") FROM otel_traces WHERE ` + spans + `),
		(SELECT toInt64(count()) FROM log_records WHERE ` + logs + `), (SELECT max("ReceivedTime") FROM log_records WHERE ` + logs + `)`
	err = clickhouseDB.QueryRow(withRefreshLimits(ctx), query).Scan(&fp[0], &fp[1], &fp[2], &fp[3])
	return fp, err
}

// fingerprint is the raw fingerprint seen when the bucket was last found
// unchanged or rebuilt by this process (none after a restart).
func (r *Refresher) fingerprint(start int64) rawFingerprintValue {
	r.mu.Lock()
	defer r.mu.Unlock()
	fp, ok := r.fingerprints[start]
	if !ok {
		return rawFingerprintValue{-1}
	}
	return fp
}

// addShapes gives a final revision committed before shapes-v1 its execution
// shapes, computed with the revision's own receipt cutoff while the raw spans
// are retained, and commits the same revision again under the current
// definitions. Its calls and errors are not touched: a final bucket is never
// replaced.
func (r *Refresher) addShapes(ctx context.Context, out BucketOutcome, start int64, published Revision, trigger string) BucketOutcome {
	insert := `INSERT INTO stat_shapes SELECT "BucketStart", ` + num(published.Revision) + ` AS "Revision", ` + shapeColumns + `
		FROM (` + bucketShapes([]interval{{start, start + hourNs}}, published.ReceiptCutoff, statsConfig.AllowedLateness) + `)`
	if err := clickhouseDB.Exec(withRefreshLimits(ctx), insert); err != nil {
		return r.failed(out, start, err)
	}
	if err := commitRevision(ctx, start, published.Revision, StateFinal, published.ReceiptCutoff, published.ContentHash,
		published.Calls, published.ErrorEvents, trigger); err != nil {
		return r.failed(out, start, err)
	}
	out.Outcome = OutcomeExtended
	r.succeeded(start)
	return out
}

func (r *Refresher) takeFailure() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failNext > 0 {
		r.failNext--
		return true
	}
	return false
}

func (r *Refresher) failed(out BucketOutcome, start int64, err error) BucketOutcome {
	out.Outcome, out.Error = OutcomeFailed, err.Error()
	r.mu.Lock()
	r.failures[start] = time.Now().UTC().Format(time.RFC3339) + ": " + err.Error()
	r.mu.Unlock()
	log.Printf("statistics: refresh of bucket %s failed: %v", time.Unix(0, start).UTC().Format(time.RFC3339), err)
	return out
}

func (r *Refresher) succeeded(start int64) {
	r.mu.Lock()
	delete(r.failures, start)
	r.mu.Unlock()
}

func commitRevision(ctx context.Context, start, revision int64, state string, cutoff int64, hash uint64, calls, errorEvents uint64, trigger string) error {
	return clickhouseDB.Exec(ctx, `INSERT INTO stat_revisions ("BucketStart", "Revision", "State", "Definitions", "GeneratedAt", "ReceiptCutoff",
		"ContentHash", "Calls", "ErrorEvents", "UnknownReceipt", "Unassignable", "Trigger")
		VALUES (toDateTime(`+num(start/1e9)+`, 'UTC'), `+num(revision)+`, '`+state+`', '`+Definitions+`', `+num(time.Now().UnixNano())+`, `+num(cutoff)+`,
		`+strconv.FormatUint(hash, 10)+`, `+strconv.FormatUint(calls, 10)+`, `+strconv.FormatUint(errorEvents, 10)+`, 0, 0, '`+trigger+`')`)
}

// contentHash summarizes the canonical eligible records a bucket's
// statistics depend on (spans from eventLookback before it, for exception
// events, to shapeLookahead after it, for execution shapes, and logs), so an
// unchanged bucket isn't stored again.
func contentHash(ctx context.Context, start, cutoff int64) (uint64, error) {
	lateness := statsConfig.AllowedLateness
	spans := statSpans([]interval{{start - int64(eventLookback), start + hourNs + int64(shapeLookahead)}}, cutoff, lateness, "")
	logs := statLogs([]interval{{start, start + hourNs}}, cutoff, lateness, "")
	query := `SELECT cityHash64(
		(SELECT tuple(count(), sum(cityHash64("TraceId", "SpanId", "UnixTime", "Duration", "StatusCode", "ServiceName", "SpanName", "SpanKind", mapSort("ResourceAttributes"), mapSort("SpanAttributes"), "EventNames", arrayMap(m -> mapSort(m), "EventAttrs"), "Payloads"))) FROM (` + spans + `) WHERE "Eligible"),
		(SELECT tuple(count(), sum(cityHash64("RecordId", "UnixTime", "TraceId", "Severity", "ServiceName", "Body", mapSort("Attributes"), "Payloads"))) FROM (` + logs + `) WHERE "Eligible"),
		'` + Definitions + `', ` + num(int64(lateness)) + `)`
	var hash uint64
	err := clickhouseDB.QueryRow(withRefreshLimits(ctx), query).Scan(&hash)
	return hash, err
}

// refreshTimeout is the least max_execution_time of a refresh query. A
// refresh reads a whole hour (two for exception events) in the background or
// in a rebuild request (bounded by its own deadline), so it gets more time
// than an investigation query: an hour of 4.3 M spans took ~10 s, the
// default query timeout.
const refreshTimeout = time.Minute

// withRefreshLimits bounds a refresh query like an API query, except for a
// longer timeout, and lets a grouping that outgrows half the memory limit
// continue on disk instead of failing (an hour's traces are grouped per trace
// for execution shapes).
func withRefreshLimits(ctx context.Context) context.Context {
	q := limits
	if q.Timeout < refreshTimeout {
		q.Timeout = refreshTimeout
	}
	settings := q.settings()
	settings["max_bytes_before_external_group_by"] = q.MaxMemoryBytes / 2
	return clickhouse.Context(ctx, clickhouse.WithSettings(settings))
}
