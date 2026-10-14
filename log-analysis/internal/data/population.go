package data

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// Contract versions recorded with every statistic and analysis
// (METRIC_CONTRACT.md of milestone 2). Change the version when the meaning of
// a value changes, so old and new results are never compared as equal.
const (
	MetricsContract    = "metrics-v1"
	ErrorsContract     = "errors-v1"
	ComparisonContract = "comparison-v1"
	// CanonicalContract is the canonical-record rule of milestone 1: one
	// record per (TraceId, SpanId) or RecordId, chosen by winnerOrder.
	CanonicalContract = "canonical-v1"
	// MessageRulesVersion names the configured identifiers of errors-v1
	// (messageRules); 2 added ids after a space and numbers of 6+ digits.
	MessageRulesVersion = "message-rules-2"
	// Definitions is stored with each statistics revision.
	Definitions = MetricsContract + ";" + ErrorsContract + ";" + MessageRulesVersion + ";" + CanonicalContract + ";" + ShapesContract
	// PercentileMethod names the percentile calculation.
	PercentileMethod = "ClickHouse quantilesTDigest (approximate t-digest), durations in milliseconds"
)

// Attribute keys statistics group by. A span's value is read from its
// resource attributes, then its span attributes; a log record's from its own
// attributes. Nothing is filled in from other records: a record without one is
// in an explicit "not recorded" group.
const (
	AttrServiceVersion  = "service.version"
	AttrEnvironment     = "deployment.environment.name"
	AttrEnvironmentOld  = "deployment.environment"
	AttrInstallation    = "troubleshooting.installation"
	AttrWorkloadSegment = "troubleshooting.workload_segment"
	AttrSamplingPolicy  = "troubleshooting.sampling_policy"
	AttrOperation       = "operation.name"
)

// Population selects recorded calls: canonical spans of one service whose
// start time is in [TimeFrom, TimeTo). Optional dimensions are pointers: nil
// means any value, "" means "not recorded" (an explicit unknown group).
//
// Operation and SpanKind are part of the definition; without them the
// statistics are broken down per (operation, span kind) and never summed
// across them, because SERVER, CONSUMER and CLIENT spans measure different
// things.
type Population struct {
	Service      string  `json:"service"`
	Operation    *string `json:"operation"`
	SpanKind     *string `json:"span_kind"`
	Environment  *string `json:"environment"`
	Version      *string `json:"version"`
	Installation *string `json:"installation"`
	Segment      *string `json:"segment"`
	TimeFrom     string  `json:"time_from"`
	TimeTo       string  `json:"time_to"`
}

// MaxStatsWindow bounds a statistics window (it can be served from retained
// statistics, so it may be much longer than an investigation window).
const MaxStatsWindow = 92 * 24 * time.Hour

// Validate checks the population and normalizes its span kind.
func (p *Population) Validate() error {
	from, err := strconv.ParseInt(p.TimeFrom, 10, 64)
	if err != nil {
		return invalid("time_from must be Unix nanoseconds")
	}
	to, err := strconv.ParseInt(p.TimeTo, 10, 64)
	if err != nil {
		return invalid("time_to must be Unix nanoseconds")
	}
	if to <= from {
		return invalid("time_to must be after time_from (the window is [time_from, time_to))")
	}
	if time.Duration(to-from) > MaxStatsWindow {
		return invalid("the statistics window is longer than %s", MaxStatsWindow)
	}
	p.Service = strings.TrimSpace(p.Service)
	if p.Service == "" {
		return invalid("service is required: statistics describe an explicitly selected population")
	}
	if p.SpanKind != nil {
		kind := strings.ToLower(strings.TrimSpace(*p.SpanKind))
		p.SpanKind = &kind
	}
	return nil
}

func (p Population) from() int64 { n, _ := strconv.ParseInt(p.TimeFrom, 10, 64); return n }
func (p Population) to() int64   { n, _ := strconv.ParseInt(p.TimeTo, 10, 64); return n }

// Duration of the window.
func (p Population) Duration() time.Duration { return time.Duration(p.to() - p.from()) }

// dimension is one population dimension with its column in the statistics
// tables (and the raw subqueries, which produce the same columns).
type dimension struct {
	name   string
	column string
	value  *string
}

func (p Population) dimensions() []dimension {
	return []dimension{
		{"operation", "Operation", p.Operation},
		{"span kind", "SpanKind", p.SpanKind},
		{"environment", "Environment", p.Environment},
		{"version", "Version", p.Version},
		{"installation", "Installation", p.Installation},
		{"workload segment", "Segment", p.Segment},
	}
}

// callConditions filters rows of stat_calls or of the raw call aggregate,
// which have the same columns.
func (p Population) callConditions(prefix string) ([]string, []any) {
	conds := []string{`"ServiceName" = {` + prefix + `service:String}`}
	params := []any{clickhouse.Named(prefix+"service", p.Service)}
	for i, d := range p.dimensions() {
		if d.value == nil {
			continue
		}
		name := fmt.Sprintf("%sdim%d", prefix, i)
		conds = append(conds, `"`+d.column+`" = {`+name+`:String}`)
		params = append(params, clickhouse.Named(name, *d.value))
	}
	return conds, params
}

// errorConditions filters error rows. Operation is left out on purpose: a
// log without a recorded operation is in an unknown-operation group, which an
// operation filter would hide; groups carry their operation instead.
func (p Population) errorConditions(prefix string) ([]string, []any) {
	conds := []string{`"ServiceName" = {` + prefix + `service:String}`}
	params := []any{clickhouse.Named(prefix+"service", p.Service)}
	for i, d := range p.dimensions() {
		if d.value == nil || d.column == "Operation" || d.column == "SpanKind" {
			continue
		}
		name := fmt.Sprintf("%sdim%d", prefix, i)
		conds = append(conds, `"`+d.column+`" = {`+name+`:String}`)
		params = append(params, clickhouse.Named(name, *d.value))
	}
	return conds, params
}

// Label describes the population for people, e.g.
// recording-worker / upload_recording / consumer / env fixture / version 1.1.0.
func (p Population) Label() string {
	parts := []string{p.Service}
	for _, d := range p.dimensions() {
		switch {
		case d.value == nil:
			continue
		case *d.value == "":
			parts = append(parts, d.name+" not recorded")
		case d.column == "Operation" || d.column == "SpanKind":
			parts = append(parts, *d.value)
		default:
			parts = append(parts, d.name+" "+*d.value)
		}
	}
	return strings.Join(parts, " / ")
}

// spanAttr reads a span attribute from the resource, then the span.
func spanAttr(key string) string {
	k := quoteString(key)
	return `if("ResourceAttributes"[` + k + `] != '', "ResourceAttributes"[` + k + `], "SpanAttributes"[` + k + `])`
}

// spanDimensions are the dimension columns computed from a canonical span.
func spanDimensions() string {
	return `"ServiceName", "SpanKind", ` + spanDerivedDimensions()
}

// spanDerivedDimensions are the dimension columns a canonical span doesn't
// have as columns.
func spanDerivedDimensions() string {
	return `"SpanName" AS "Operation",
		if(` + spanAttr(AttrEnvironment) + ` != '', ` + spanAttr(AttrEnvironment) + `, ` + spanAttr(AttrEnvironmentOld) + `) AS "Environment",
		` + spanAttr(AttrServiceVersion) + ` AS "Version",
		` + spanAttr(AttrInstallation) + ` AS "Installation",
		` + spanAttr(AttrWorkloadSegment) + ` AS "Segment",
		` + spanAttr(AttrSamplingPolicy) + ` AS "Sampling"`
}

// logDimensions are the dimension columns of a canonical log record.
func logDimensions() string {
	attr := func(key string) string { return `"Attributes"[` + quoteString(key) + `]` }
	return `"ServiceName", ` + attr(AttrOperation) + ` AS "Operation",
		if(` + attr(AttrEnvironment) + ` != '', ` + attr(AttrEnvironment) + `, ` + attr(AttrEnvironmentOld) + `) AS "Environment",
		` + attr(AttrServiceVersion) + ` AS "Version",
		` + attr(AttrInstallation) + ` AS "Installation",
		` + attr(AttrWorkloadSegment) + ` AS "Segment"`
}
