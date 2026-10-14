package data

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// Examples are a bounded list of recorded calls or error occurrences of a
// population, to open in the combined trace-and-log report. They are
// examples, not the contributing population: a percentile is approximate and
// need not equal any one call.

// Example kinds.
const (
	ExampleSlow  = "slow"
	ExampleError = "error"
)

// MaxExamples caps one examples request.
const MaxExamples = 100

// ExamplesRequest is the body of examples.
type ExamplesRequest struct {
	Population Population `json:"population"`
	Kind       string     `json:"kind"`
	// ThresholdMs: slow examples are calls at least this slow (the displayed
	// slow-example threshold, e.g. the candidate p95).
	ThresholdMs float64 `json:"threshold_ms"`
	// Group selects error examples of one group (from error groups).
	Group *GroupRef `json:"group"`
	Limit int       `json:"limit"`
	// Cutoff is the analysis cutoff (Unix ns): examples must have arrived
	// before it, like the statistics they illustrate. Empty: now.
	Cutoff string `json:"cutoff"`
}

// GroupRef identifies an error group.
type GroupRef struct {
	SourceKind  string `json:"source_kind"`
	Service     string `json:"service"`
	Operation   string `json:"operation"`
	Environment string `json:"environment"`
	Fingerprint string `json:"fingerprint"`
}

// Validate checks the request.
func (r *ExamplesRequest) Validate() error {
	if err := r.Population.Validate(); err != nil {
		return err
	}
	switch r.Kind {
	case ExampleSlow:
		if r.ThresholdMs < 0 {
			return invalid("threshold_ms must not be negative")
		}
	case ExampleError:
		if r.Group == nil {
			return invalid("error examples need a group")
		}
		switch r.Group.SourceKind {
		case SourceSpanStatus, SourceException, SourceLog:
		default:
			return invalid("unknown source kind %q", r.Group.SourceKind)
		}
	default:
		return invalid("kind must be slow or error")
	}
	if r.Limit <= 0 || r.Limit > MaxExamples {
		r.Limit = 10
	}
	if r.Cutoff != "" {
		if _, err := strconv.ParseInt(r.Cutoff, 10, 64); err != nil {
			return invalid("cutoff must be Unix nanoseconds")
		}
	}
	return nil
}

// Example is one recorded call or error occurrence.
type Example struct {
	SourceKind string   `json:"sourceKind"` // span (slow), span_status, exception, log
	TraceId    string   `json:"traceId"`    // "" for a log without trace
	SpanId     string   `json:"spanId"`
	RecordId   string   `json:"recordId,omitempty"`
	EventIndex int      `json:"eventIndex,omitempty"`
	Time       string   `json:"time"`
	DurationMs *float64 `json:"durationMs,omitempty"`
	Status     string   `json:"status,omitempty"`
	Service    string   `json:"service"`
	Operation  string   `json:"operation"`
	Version    string   `json:"version"`
	Message    string   `json:"message"` // as recorded
}

// ExamplesResult answers examples.
type ExamplesResult struct {
	Status      string    `json:"status"`
	Condition   string    `json:"condition"`
	Examples    []Example `json:"examples"`
	Limit       int       `json:"limit"`
	Shown       int       `json:"shown"`
	Total       uint64    `json:"total"`
	Truncated   bool      `json:"truncated"`
	ThresholdMs float64   `json:"thresholdMs,omitempty"`
	Description string    `json:"description"`
	Cutoff      string    `json:"cutoff"`
}

// FindExamples returns examples from canonical raw records. When the raw
// evidence of the window has expired, the result says so; examples are never
// reconstructed from statistics.
func FindExamples(ctx context.Context, req ExamplesRequest) ExamplesResult {
	now := clock.Now()
	pol := policyFor(ctx)
	cutoff := time.Now().UnixNano()
	if req.Cutoff != "" {
		cutoff, _ = strconv.ParseInt(req.Cutoff, 10, 64)
	}
	res := ExamplesResult{Examples: []Example{}, Limit: req.Limit, Cutoff: strconv.FormatInt(cutoff, 10), ThresholdMs: req.ThresholdMs}
	horizon := rawHorizon(now, pol.TracesSeconds)
	if req.Kind == ExampleError && req.Group.SourceKind == SourceLog {
		horizon = rawHorizon(now, pol.LogsSeconds)
	}
	if req.Population.from() < horizon {
		res.Status = StatsOutsideRetention
		res.Condition = "The raw spans and logs of this window are outside retention: statistics remain, but individual traces, log messages and stack traces can no longer be shown. Nothing is reconstructed from aggregate data."
		return res
	}
	var query string
	var params []any
	parts := []interval{{req.Population.from(), req.Population.to()}}
	lateness := statsConfig.AllowedLateness
	pre := `"ServiceName" = {service:String}`
	switch req.Kind {
	case ExampleSlow:
		conds, p := req.Population.callConditions("")
		params = p
		// Only spans with a row at or above the threshold can qualify.
		pre += ` AND "Duration" >= ` + num(int64(req.ThresholdMs*1000))
		query = `SELECT 'span', "TraceId", "SpanId", '', toInt64(0), toString("UnixTime"), toFloat64("Duration") / 1000, toString("StatusCode"),
			"ServiceName", "Operation", "Version", "StatusMessage"
		FROM (SELECT *, ` + spanDerivedDimensions() + ` FROM (` + statSpans(parts, cutoff, lateness, pre) + `) WHERE "Eligible")` +
			whereClause(append(conds, `"Duration" >= `+num(int64(req.ThresholdMs*1000)))) + `
		ORDER BY "Duration" DESC, "UnixTime", "TraceId", "SpanId"`
		res.Description = "Observed slow calls at or above the threshold, slowest first. They are examples of the population, not the full set of contributing calls, and not a single \"p95 trace\"."
	case ExampleError:
		conds, p := req.Population.errorConditions("")
		params = append(p,
			namedParam("gservice", req.Group.Service), namedParam("goperation", req.Group.Operation),
			namedParam("genvironment", req.Group.Environment), namedParam("gfingerprint", req.Group.Fingerprint))
		conds = append(conds, `"ServiceName" = {gservice:String}`, `"Operation" = {goperation:String}`,
			`"Environment" = {genvironment:String}`, `"Fingerprint" = {gfingerprint:String}`)
		var occurrences string
		switch req.Group.SourceKind {
		case SourceSpanStatus:
			occurrences = `SELECT *, '' AS "RecordId", 0 AS "EventIndex" FROM (` + statusOccurrences(parts, cutoff, lateness, pre) + `)`
		case SourceException:
			occurrences = `SELECT *, '' AS "RecordId" FROM (` + exceptionOccurrences(parts, cutoff, lateness, pre) + `)`
		case SourceLog:
			occurrences = `SELECT *, 0 AS "EventIndex" FROM (` + logOccurrences(parts, cutoff, lateness, pre) + `)`
		}
		query = `SELECT "SourceKind", "TraceId", "SpanId", "RecordId", toInt64("EventIndex"), toString("EventTime"), CAST(NULL AS Nullable(Float64)), '',
			"ServiceName", "Operation", "Version", "Message"
		FROM (
			SELECT *, if("ExceptionType" != '', '` + MethodException + `', '` + MethodMessage + `') AS "Method",
				if("ExceptionType" != '', '', ` + normalizedMessage(`"Message"`) + `) AS "Signature",
				` + fingerprint + ` AS "Fingerprint"
			FROM (` + occurrences + `)
		)` + whereClause(conds) + `
		ORDER BY "EventTime", "TraceId", "SpanId", "RecordId"`
		res.Description = "Recorded occurrences of this error group, oldest first. They are examples, not every occurrence."
	}

	countQuery := `SELECT count() FROM (` + query + `)`
	if err := clickhouseDB.QueryRow(withLimits(ctx), countQuery, params...).Scan(&res.Total); err != nil {
		res.Status, res.Condition = statsStatus(ctx, err)
		return res
	}
	rows, err := clickhouseDB.Query(withLimits(ctx), query+` LIMIT `+strconv.Itoa(req.Limit), params...)
	if err != nil {
		res.Status, res.Condition = statsStatus(ctx, err)
		return res
	}
	defer rows.Close()
	for rows.Next() {
		var e Example
		var index int64
		var status string
		if err := rows.Scan(&e.SourceKind, &e.TraceId, &e.SpanId, &e.RecordId, &index, &e.Time, &e.DurationMs, &status,
			&e.Service, &e.Operation, &e.Version, &e.Message); err != nil {
			res.Status, res.Condition = statsStatus(ctx, err)
			res.Examples = []Example{}
			return res
		}
		e.EventIndex = int(index)
		if status != "" {
			e.Status = strings.ToLower(StatusCodeToString(status))
		}
		res.Examples = append(res.Examples, e)
	}
	if err := rows.Err(); err != nil {
		res.Status, res.Condition = statsStatus(ctx, err)
		res.Examples = []Example{}
		return res
	}
	res.Shown = len(res.Examples)
	res.Truncated = res.Total > uint64(res.Shown)
	res.Status, res.Condition = StatsOK, "ok"
	if res.Total == 0 {
		res.Condition = "no retained examples match"
	}
	return res
}
