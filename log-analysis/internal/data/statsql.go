package data

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// SQL builders for statistics. Times are Unix nanoseconds; numbers are
// formatted by Go and inlined, user-supplied strings are always bound as
// parameters.

// interval is a half-open range [From, To) of Unix nanoseconds.
type interval struct{ From, To int64 }

const hourNs = int64(time.Hour)

// eventLookback is how long before a bucket a span may have started and
// still have an exception event inside the bucket. Exception events are
// counted at their own time; an event more than this after its span's start
// is not counted.
const eventLookback = time.Hour

func num(n int64) string { return strconv.FormatInt(n, 10) }

// windowsCondition matches column in any of the intervals.
func windowsCondition(column string, parts []interval) string {
	conds := make([]string, len(parts))
	for i, p := range parts {
		conds[i] = `(` + column + ` >= ` + num(p.From) + ` AND ` + column + ` < ` + num(p.To) + `)`
	}
	return `(` + strings.Join(conds, ` OR `) + `)`
}

// eligibleCondition is true for a raw row that counts in statistics computed
// with the given receipt cutoff: it arrived before the cutoff and before its
// bucket's deadline (bucket end + allowed lateness). A row received at or
// after the deadline is too late for that bucket. Rows stored before arrival
// times were recorded have ReceivedTime 0 and count, with an unknown-receipt
// notice.
func eligibleCondition(cutoff int64, lateness time.Duration) string {
	return `"ReceivedTime" < least(` + num(cutoff) + `, intDiv("UnixTime", ` + num(hourNs) + `) * ` + num(hourNs) +
		` + ` + num(hourNs+int64(lateness)) + `)`
}

// spanPayloadHash hashes every non-identity column of a span row, as in
// winnerOrder.
const spanPayloadHash = `cityHash64(` + spanPayloadColumns + `)`

// sharedTimes is the times in table that more than one row matching where has.
// Retries and replays repeat a record with its time, so a row at any other
// time is its record's only row and needs no grouping; only rows at shared
// times are grouped per identity. Statistics then hold a few states instead of
// one per record of the hour, which ran out of memory at ~1 M spans an hour.
// Copies of one identity stored with different times are not compared (as in
// the span and log lists, see pageCanonicalSpans).
func sharedTimes(table, where string) string {
	return `SELECT "UnixTime" FROM ` + table + ` WHERE ` + where + ` GROUP BY "UnixTime" HAVING count() > 1`
}

// statSpans is one canonical record per (TraceId, SpanId) among the rows in
// the intervals that arrived before cutoff, chosen among the eligible rows
// with the canonical-v1 rule (winnerOrder as a tuple). Eligible says the
// identity has an eligible row; Payloads counts its distinct eligible
// payloads (more than one is a conflict); UnknownReceipt marks identities
// whose first eligible row has no recorded arrival time. Without a
// preselection a span alone at its start time is passed through without
// grouping (see sharedTimes), and only if it is eligible: every reader keeps
// Eligible identities only.
func statSpans(parts []interval, cutoff int64, lateness time.Duration, preselect string) string {
	pick := func(column, alias string) string {
		return `argMaxIf(` + column + `, r_rank, r_ok) AS "` + alias + `"`
	}
	where := windowsCondition(`"UnixTime"`, parts) + ` AND "ReceivedTime" < ` + num(cutoff)
	if preselect != "" {
		where += ` AND ("TraceId", "SpanId") IN (SELECT "TraceId", "SpanId" FROM otel_traces WHERE ` +
			windowsCondition(`"UnixTime"`, parts) + ` AND ` + preselect + `)`
	}
	// With a preselection only its spans are grouped, in one pass: few
	// enough to hold, and the window isn't read again for shared times.
	groupedWhere := where
	single := ""
	if preselect == "" {
		shared := sharedTimes("otel_traces", where)
		groupedWhere += ` AND "UnixTime" IN (` + shared + `)`
		single = `SELECT "TraceId", "SpanId", "ParentSpanId", "UnixTime", "ServiceName", "SpanName", "SpanKind", "ResourceAttributes", "SpanAttributes",
		"Duration", "StatusCode", "StatusMessage", "Events.Timestamp" AS "EventTimes", "Events.Name" AS "EventNames",
		"Events.Attributes" AS "EventAttrs", toUInt8(1) AS "Eligible", toUInt64(1) AS "Payloads", "ReceivedTime" = 0 AS "UnknownReceipt"
	FROM otel_traces
	WHERE ` + where + ` AND ` + eligibleCondition(cutoff, lateness) + ` AND "UnixTime" NOT IN (` + shared + `)
	UNION ALL
	`
	}
	return single + `SELECT "TraceId", "SpanId", ` + pick("r_parent", "ParentSpanId") + `,
		` + pick("r_time", "UnixTime") + `, ` + pick("r_service", "ServiceName") + `, ` + pick("r_name", "SpanName") + `,
		` + pick("r_kind", "SpanKind") + `, ` + pick("r_res", "ResourceAttributes") + `, ` + pick("r_attrs", "SpanAttributes") + `,
		` + pick("r_duration", "Duration") + `, ` + pick("r_status", "StatusCode") + `, ` + pick("r_message", "StatusMessage") + `,
		` + pick("r_event_times", "EventTimes") + `, ` + pick("r_event_names", "EventNames") + `, ` + pick("r_event_attrs", "EventAttrs") + `,
		countIf(r_ok) > 0 AS "Eligible",
		uniqExactIf(r_payload, r_ok) AS "Payloads",
		minIf("ReceivedTime", r_ok) = 0 AS "UnknownReceipt"
	FROM (
		SELECT "TraceId", "SpanId", "ReceivedTime",
			"ParentSpanId" AS r_parent, "UnixTime" AS r_time, "ServiceName" AS r_service, "SpanName" AS r_name, "SpanKind" AS r_kind,
			"ResourceAttributes" AS r_res, "SpanAttributes" AS r_attrs, "Duration" AS r_duration,
			"StatusCode" AS r_status, "StatusMessage" AS r_message,
			"Events.Timestamp" AS r_event_times, "Events.Name" AS r_event_names, "Events.Attributes" AS r_event_attrs,
			` + spanPayloadHash + ` AS r_payload,
			tuple("StatusCode" = 1, length("Events.Name"), length("Links.SpanId"), r_payload) AS r_rank,
			` + eligibleCondition(cutoff, lateness) + ` AS r_ok
		FROM otel_traces
		WHERE ` + groupedWhere + `
	)
	GROUP BY "TraceId", "SpanId"`
}

// statLogs is one canonical record per RecordId (the row with the largest
// payload hash, as in canonicalLogs) among eligible rows. A record alone at
// its time is passed through without grouping, as in statSpans.
func statLogs(parts []interval, cutoff int64, lateness time.Duration, preselect string) string {
	pick := func(column, alias string) string {
		return `argMaxIf(` + column + `, r_payload, r_ok) AS "` + alias + `"`
	}
	where := windowsCondition(`"UnixTime"`, parts) + ` AND "ReceivedTime" < ` + num(cutoff)
	if preselect != "" {
		where += ` AND "RecordId" IN (SELECT "RecordId" FROM log_records WHERE ` +
			windowsCondition(`"UnixTime"`, parts) + ` AND ` + preselect + `)`
	}
	groupedWhere := where
	single := ""
	if preselect == "" {
		shared := sharedTimes("log_records", where)
		groupedWhere += ` AND "UnixTime" IN (` + shared + `)`
		single = `SELECT "RecordId", "UnixTime", "TraceId", "SpanId", "Severity", "ServiceName", "Body", "Attributes",
		toUInt8(1) AS "Eligible", toUInt64(1) AS "Payloads", "ReceivedTime" = 0 AS "UnknownReceipt"
	FROM log_records
	WHERE ` + where + ` AND ` + eligibleCondition(cutoff, lateness) + ` AND "UnixTime" NOT IN (` + shared + `)
	UNION ALL
	`
	}
	return single + `SELECT "RecordId",
		` + pick("r_time", "UnixTime") + `, ` + pick("r_trace", "TraceId") + `, ` + pick("r_span", "SpanId") + `,
		` + pick("r_severity", "Severity") + `, ` + pick("r_service", "ServiceName") + `, ` + pick("r_body", "Body") + `,
		` + pick("r_attrs", "Attributes") + `,
		countIf(r_ok) > 0 AS "Eligible",
		uniqExactIf(r_payload, r_ok) AS "Payloads",
		minIf("ReceivedTime", r_ok) = 0 AS "UnknownReceipt"
	FROM (
		SELECT "RecordId", "ReceivedTime",
			"UnixTime" AS r_time, "TraceId" AS r_trace, "SpanId" AS r_span, "Severity" AS r_severity,
			"ServiceName" AS r_service, "Body" AS r_body, "Attributes" AS r_attrs,
			cityHash64(` + logPayloadColumns + `) AS r_payload,
			` + eligibleCondition(cutoff, lateness) + ` AS r_ok
		FROM log_records
		WHERE ` + groupedWhere + `
	)
	GROUP BY "RecordId"`
}

const bucketOf = `toStartOfHour(fromUnixTimestamp64Nano("UnixTime"), 'UTC')`

// callColumns are the columns of stat_calls after Revision; the raw call
// aggregate produces the same ones.
const callColumns = `"BucketStart", "ServiceName", "Operation", "SpanKind", "Environment", "Version", "Installation", "Segment", "Sampling",
	"Calls", "Errors", "Ok", "Unset", "ValidDurations", "InvalidDurations", "Conflicts", "Durations"`

// rawCalls aggregates canonical eligible spans per bucket and dimensions.
// A call is a span; a valid duration is non-negative (zero counts); a
// conflict is an identity with more than one eligible payload.
func rawCalls(parts []interval, cutoff int64, lateness time.Duration, preselect string) string {
	return `SELECT ` + bucketOf + ` AS "BucketStart", ` + spanDimensions() + `,
		count() AS "Calls",
		countIf("StatusCode" = 1) AS "Errors",
		countIf("StatusCode" = 2) AS "Ok",
		countIf("StatusCode" = 0) AS "Unset",
		countIf("Duration" >= 0) AS "ValidDurations",
		countIf("Duration" < 0) AS "InvalidDurations",
		countIf("Payloads" > 1) AS "Conflicts",
		quantilesTDigestIfState(0.5, 0.95, 0.99)("Duration" / 1000, "Duration" >= 0) AS "Durations"
	FROM (` + statSpans(parts, cutoff, lateness, preselect) + `)
	WHERE "Eligible"
	GROUP BY "BucketStart", "ServiceName", "Operation", "SpanKind", "Environment", "Version", "Installation", "Segment", "Sampling"`
}

// Error grouping (errors-v1). Source kinds stay separate: an ERROR-status
// span, an exception event and an ERROR/FATAL log can describe the same
// failure, and their counts are never added.
const (
	SourceSpanStatus = "span_status"
	SourceException  = "exception"
	SourceLog        = "log"
)

// Grouping methods: by exception type and application frame, or by a
// normalized message when no exception type was recorded.
const (
	MethodException = "exception"
	MethodMessage   = "message"
)

// messageRules are the configured dynamic identifiers that errors-v1 removes
// from a message before grouping (MessageRulesVersion names this set). Short
// numbers are kept on purpose: status codes such as 403 and 500, durations and
// counts stay distinct; an id after an identifier word ("order 3684958") and
// any number of six or more digits are identifiers, which otherwise made one
// group per record. The same rules run in ClickHouse (RE2) and in Go tests.
var messageRules = []struct {
	pattern, replace string
}{
	{`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`, `<uuid>`},
	{`\b[0-9a-fA-F]{16,}\b`, `<hex>`},
	{`(?i)\b(request|req|session|user|order|trace|span|correlation)[-_:=#][A-Za-z0-9][A-Za-z0-9_-]*`, `$1-<id>`},
	{`(?i)\b(request|req|session|user|order|trace|span|correlation)\s+([0-9]+|[0-9a-fA-F-]{8,})\b`, `$1 <id>`},
	{`\b[0-9]{6,}\b`, `<num>`},
	{`\s+`, ` `},
}

// NormalizeMessage applies messageRules; it is the Go twin of
// normalizedMessage.
func NormalizeMessage(message string) string {
	for _, rule := range messageRules {
		message = regexp.MustCompile(rule.pattern).ReplaceAllString(message, rule.replace)
	}
	return strings.TrimSpace(message)
}

// normalizedMessage is the ClickHouse expression of NormalizeMessage.
func normalizedMessage(expr string) string {
	for _, rule := range messageRules {
		replace := strings.ReplaceAll(rule.replace, "$1", `\\1`)
		expr = `replaceRegexpAll(` + expr + `, ` + quoteString(rule.pattern) + `, '` + replace + `')`
	}
	return `trimBoth(` + expr + `)`
}

// frameOf is the recorded application frame of an exception (attribute map
// expr): code.function.name, code.namespace + code.function, or the first
// line of exception.stacktrace without "at", arguments, line numbers and
// offsets.
func frameOf(attrs string) string {
	a := func(key string) string { return attrs + `[` + quoteString(key) + `]` }
	firstLine := `trimBoth(arrayFirst(l -> trimBoth(l) != '', splitByChar('\n', ` + a("exception.stacktrace") + `)))`
	stackFrame := `trimBoth(replaceRegexpAll(replaceRegexpOne(` + firstLine + `, '^at\\s+', ''), '\\(.*\\)|:[0-9]+|\\s\\+0x[0-9a-fA-F]+', ''))`
	return `multiIf(` + a("code.function.name") + ` != '', ` + a("code.function.name") + `,
		` + a("code.function") + ` != '', if(` + a("code.namespace") + ` != '', concat(` + a("code.namespace") + `, '.', ` + a("code.function") + `), ` + a("code.function") + `),
		` + a("exception.stacktrace") + ` != '', ` + stackFrame + `, '')`
}

// errorColumns are the columns of stat_errors after Revision.
const errorColumns = `"BucketStart", "SourceKind", "ServiceName", "Operation", "Environment", "Version", "Installation", "Segment",
	"Method", "Fingerprint", "ExceptionType", "Frame", "Signature", "Occurrences", "Unlinked", "Traces", "FirstSeen", "LastSeen", "Calls", "HasCalls"`

// fingerprint identifies a group within its source kind, service,
// operation and environment; version is a comparison dimension, not part of
// it.
const fingerprint = `lower(hex(cityHash64('` + ErrorsContract + `', "Method", "ExceptionType", "Frame", "Signature")))`

// groupedErrors aggregates error occurrences (rows with columns
// BucketTime, SourceKind, dims, ExceptionType, Frame, Message, TraceId,
// SpanId). Calls are the distinct (TraceId, SpanId) the occurrences recorded:
// several logs of one span are one affected call, an occurrence without a
// span is none.
func groupedErrors(occurrences string) string {
	return `SELECT toStartOfHour(fromUnixTimestamp64Nano("EventTime"), 'UTC') AS "BucketStart", "SourceKind",
		"ServiceName", "Operation", "Environment", "Version", "Installation", "Segment",
		if("ExceptionType" != '', '` + MethodException + `', '` + MethodMessage + `') AS "Method",
		` + fingerprint + ` AS "Fingerprint",
		"ExceptionType", "Frame",
		if("ExceptionType" != '', '', ` + normalizedMessage(`"Message"`) + `) AS "Signature",
		count() AS "Occurrences",
		countIf("TraceId" = '') AS "Unlinked",
		uniqExactIfState("TraceId", "TraceId" != '') AS "Traces",
		min("EventTime") AS "FirstSeen",
		max("EventTime") AS "LastSeen",
		uniqExactIfState(concat("TraceId", ':', "SpanId"), "TraceId" != '' AND "SpanId" != '') AS "Calls",
		toUInt8(1) AS "HasCalls"
	FROM (` + occurrences + `)
	GROUP BY "BucketStart", "SourceKind", "ServiceName", "Operation", "Environment", "Version", "Installation", "Segment",
		"Method", "Fingerprint", "ExceptionType", "Frame", "Signature"`
}

// spanErrorDimensions are a span's dimensions for error groups.
func spanErrorDimensions() string {
	return `"ServiceName", "SpanName" AS "Operation",
		if(` + spanAttr(AttrEnvironment) + ` != '', ` + spanAttr(AttrEnvironment) + `, ` + spanAttr(AttrEnvironmentOld) + `) AS "Environment",
		` + spanAttr(AttrServiceVersion) + ` AS "Version",
		` + spanAttr(AttrInstallation) + ` AS "Installation",
		` + spanAttr(AttrWorkloadSegment) + ` AS "Segment"`
}

// statusOccurrences are canonical ERROR-status spans, at their start time.
func statusOccurrences(parts []interval, cutoff int64, lateness time.Duration, preselect string) string {
	return `SELECT '` + SourceSpanStatus + `' AS "SourceKind", "UnixTime" AS "EventTime", "TraceId", "SpanId",
		` + spanErrorDimensions() + `, '' AS "ExceptionType", '' AS "Frame", "StatusMessage" AS "Message"
	FROM (` + statSpans(parts, cutoff, lateness, preselect) + `)
	WHERE "Eligible" AND "StatusCode" = 1`
}

// exceptionOccurrences are the exception events of canonical spans, at the
// event's own time. An event belongs to its canonical span and position, so
// replays don't add occurrences. Spans are read from eventLookback before
// each interval.
func exceptionOccurrences(parts []interval, cutoff int64, lateness time.Duration, preselect string) string {
	spanParts := make([]interval, len(parts))
	for i, p := range parts {
		spanParts[i] = interval{p.From - int64(eventLookback), p.To}
	}
	return `SELECT '` + SourceException + `' AS "SourceKind", "EventTime", "TraceId", "SpanId", "EventIndex",
		` + spanErrorDimensions() + `,
		"EventAttrs"["EventIndex"]['exception.type'] AS "ExceptionType",
		` + frameOf(`"EventAttrs"["EventIndex"]`) + ` AS "Frame",
		"EventAttrs"["EventIndex"]['exception.message'] AS "Message"
	FROM (
		SELECT *, "EventIndex", toUnixTimestamp64Nano("EventTimes"["EventIndex"]) AS "EventTime"
		FROM (` + statSpans(spanParts, cutoff, lateness, preselect) + `)
		ARRAY JOIN arrayFilter(i -> "EventNames"[i] = 'exception', arrayEnumerate("EventNames")) AS "EventIndex"
		WHERE "Eligible"
	)
	WHERE ` + windowsCondition(`"EventTime"`, parts)
}

// logOccurrences are canonical ERROR and FATAL log records at their time.
func logOccurrences(parts []interval, cutoff int64, lateness time.Duration, preselect string) string {
	attr := func(key string) string { return `"Attributes"[` + quoteString(key) + `]` }
	return `SELECT '` + SourceLog + `' AS "SourceKind", "UnixTime" AS "EventTime", "TraceId", "SpanId", "RecordId",
		` + logDimensions() + `,
		` + attr("exception.type") + ` AS "ExceptionType",
		` + frameOf(`"Attributes"`) + ` AS "Frame",
		"Body" AS "Message"
	FROM (` + statLogs(parts, cutoff, lateness, preselect) + `)
	WHERE "Eligible" AND "Severity" IN ('ERROR', 'FATAL')`
}

func namedParam(name, value string) any { return clickhouse.Named(name, value) }
