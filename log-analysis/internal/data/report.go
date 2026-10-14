package data

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A trace report covers exactly one TraceId within a half-open window [from, to). Its
// spans and its log records are separate sections, each queried on its own:
// one failing source never hides the evidence of the other, and a failed
// section is never reported as empty.

// Report sections.
const (
	SectionSpans = "spans"
	SectionLogs  = "logs"
)

// Section statuses.
const (
	SectionOK          = "ok"          // the source returned every matching record up to the limit
	SectionUnavailable = "unavailable" // the database could not be reached
	SectionLimit       = "limit"       // a query limit or the deadline was hit; no partial result
	SectionError       = "error"       // any other failure
)

// ReportRequest is the body of trace-report.
type ReportRequest struct {
	TraceId   string   `json:"trace_id"`
	TimeFrom  string   `json:"time_from"`
	TimeTo    string   `json:"time_to"`
	SpanLimit int      `json:"span_limit"` // records shown; default and maximum MaxRowsPerPage
	LogLimit  int      `json:"log_limit"`  // default and maximum MaxLogRowsPerPage
	Sections  []string `json:"sections"`   // empty: both; a retry asks for the failed one
}

// Validate checks the request and fills in the defaults.
func (r *ReportRequest) Validate() error {
	if err := validateWindow(r.TimeFrom, r.TimeTo); err != nil {
		return err
	}
	r.TraceId = strings.ToLower(r.TraceId)
	if !traceIDPattern.MatchString(r.TraceId) {
		return invalid("trace_id must be 32 hex digits")
	}
	r.SpanLimit = clampLimit(r.SpanLimit, MaxRowsPerPage)
	r.LogLimit = clampLimit(r.LogLimit, MaxLogRowsPerPage)
	if len(r.Sections) == 0 {
		r.Sections = []string{SectionSpans, SectionLogs}
	}
	for _, s := range r.Sections {
		if s != SectionSpans && s != SectionLogs {
			return invalid("unknown section %q", s)
		}
	}
	return nil
}

func clampLimit(limit, max int) int {
	if limit <= 0 || limit > max {
		return max
	}
	return limit
}

func (r *ReportRequest) wants(section string) bool {
	for _, s := range r.Sections {
		if s == section {
			return true
		}
	}
	return false
}

// ReportSection is one source's evidence. Shown is the number of records
// returned; Truncated says more matched (read from one extra row). Total is
// the number of matching logical records, nil when counting failed.
type ReportSection[T any] struct {
	Status      string  `json:"status"`
	Error       string  `json:"error,omitempty"`
	Records     []T     `json:"records"`
	Limit       int     `json:"limit"`
	Shown       int     `json:"shown"`
	Truncated   bool    `json:"truncated"`
	Total       *uint64 `json:"total"`
	TotalStatus string  `json:"totalStatus"`
	TotalError  string  `json:"totalError,omitempty"`
}

// Warning is a limitation of the evidence that isn't a failure.
type Warning struct {
	Code    string `json:"code"`
	Section string `json:"section"`
	Message string `json:"message"`
}

// Report is the answer of trace-report. Complete means every requested section
// returned all matching records of this trace within the window; it doesn't
// mean every real event was instrumented or delivered.
type Report struct {
	TraceId     string                     `json:"traceId"`
	TimeFrom    string                     `json:"timeFrom"`
	TimeTo      string                     `json:"timeTo"`
	GeneratedAt string                     `json:"generatedAt"` // Unix nanoseconds
	Spans       *ReportSection[*Span]      `json:"spans,omitempty"`
	Logs        *ReportSection[*LogRecord] `json:"logs,omitempty"`
	Warnings    []Warning                  `json:"warnings"`
	Complete    bool                       `json:"complete"`
}

// RetentionPolicy is how long each source keeps records (the TTLs of
// otel_traces and log_records, 3 days by default).
type RetentionPolicy struct {
	Traces time.Duration
	Logs   time.Duration
}

var DefaultRetention = RetentionPolicy{Traces: 72 * time.Hour, Logs: 72 * time.Hour}

var retention = DefaultRetention

// SetRetention sets the retention the report warns about.
func SetRetention(r RetentionPolicy) { retention = r }

// ReportSources are the queries a report runs; tests replace them.
type ReportSources struct {
	Spans      func(context.Context, SpanFilter) (SpanPage, error)
	CountSpans func(context.Context, SpanFilter) (uint64, error)
	Logs       func(context.Context, LogFilter) (LogPage, error)
	CountLogs  func(context.Context, LogFilter) (uint64, error)
	Now        func() time.Time
}

// Sources returns the ClickHouse queries.
func (l *Log) Sources() ReportSources {
	return ReportSources{
		Spans:      selectSpans,
		CountSpans: l.SelectCountSpans,
		Logs:       l.SelectLogs,
		CountLogs:  l.SelectCountLogs,
		Now:        time.Now,
	}
}

// BuildReport queries the requested sections concurrently. It never fails as
// a whole: each section carries its own status.
func BuildReport(ctx context.Context, req ReportRequest, src ReportSources) Report {
	now := src.Now()
	report := Report{
		TraceId:     req.TraceId,
		TimeFrom:    req.TimeFrom,
		TimeTo:      req.TimeTo,
		GeneratedAt: strconv.FormatInt(now.UnixNano(), 10),
		Warnings:    []Warning{},
	}

	var wg sync.WaitGroup
	if req.wants(SectionSpans) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			filter := SpanFilter{TimeFrom: req.TimeFrom, TimeTo: req.TimeTo, TraceId: req.TraceId, Scope: ScopeAll, RowsPerPage: req.SpanLimit}
			section := &ReportSection[*Span]{Limit: req.SpanLimit, Records: []*Span{}}
			page, err := src.Spans(ctx, filter)
			if err == nil {
				section.Records = nonNil(page.Spans)
				section.Truncated = page.HasMore
				var total uint64
				total, err = src.CountSpans(ctx, filter)
				section.setTotal(ctx, total, err)
				err = nil
			}
			section.finish(ctx, err)
			report.Spans = section
		}()
	}
	if req.wants(SectionLogs) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			filter := LogFilter{TimeFrom: req.TimeFrom, TimeTo: req.TimeTo, TraceId: req.TraceId, RowsPerPage: req.LogLimit}
			section := &ReportSection[*LogRecord]{Limit: req.LogLimit, Records: []*LogRecord{}}
			page, err := src.Logs(ctx, filter)
			if err == nil {
				section.Records = nonNil(page.Logs)
				section.Truncated = page.HasMore
				var total uint64
				total, err = src.CountLogs(ctx, filter)
				section.setTotal(ctx, total, err)
				err = nil
			}
			section.finish(ctx, err)
			report.Logs = section
		}()
	}
	wg.Wait()

	report.Warnings = retentionWarnings(req, now)
	report.Complete = (report.Spans == nil || report.Spans.complete()) && (report.Logs == nil || report.Logs.complete())
	return report
}

func nonNil[T any](records []T) []T {
	if records == nil {
		return []T{}
	}
	return records
}

func (s *ReportSection[T]) setTotal(ctx context.Context, total uint64, err error) {
	if err != nil {
		s.TotalStatus = SectionUnavailable
		_, s.TotalError = classify(ctx, err)
		return
	}
	s.Total = &total
	s.TotalStatus = SectionOK
}

func (s *ReportSection[T]) finish(ctx context.Context, err error) {
	if err != nil {
		s.Status, s.Error = classify(ctx, err)
		s.Records = []T{}
		s.Truncated = false
		s.Total = nil
		s.TotalStatus = SectionUnavailable
		return
	}
	s.Status = SectionOK
	s.Shown = len(s.Records)
}

func (s *ReportSection[T]) complete() bool {
	return s.Status == SectionOK && !s.Truncated
}

// classify turns a query error into a section status and a message.
func classify(ctx context.Context, err error) (string, string) {
	err = ExplainQueryError(ctx, err)
	switch {
	case errors.Is(err, ErrQueryLimit):
		return SectionLimit, err.Error()
	case errors.Is(err, ErrUnavailable):
		return SectionUnavailable, err.Error()
	default:
		return SectionError, err.Error()
	}
}

// retentionWarnings warns when the window starts before a source's retention
// horizon: records there may have been deleted, so their absence proves
// nothing.
func retentionWarnings(req ReportRequest, now time.Time) []Warning {
	warnings := []Warning{}
	from, err := strconv.ParseInt(req.TimeFrom, 10, 64)
	if err != nil {
		return warnings
	}
	start := time.Unix(0, from)
	if req.wants(SectionSpans) && start.Before(now.Add(-retention.Traces)) {
		warnings = append(warnings, Warning{Code: "retention", Section: SectionSpans, Message: fmt.Sprintf(
			"The window starts more than %s ago; spans older than the retention period (%s) may have expired. Missing spans there are not evidence that nothing was recorded.",
			hours(retention.Traces), hours(retention.Traces))})
	}
	if req.wants(SectionLogs) && start.Before(now.Add(-retention.Logs)) {
		warnings = append(warnings, Warning{Code: "retention", Section: SectionLogs, Message: fmt.Sprintf(
			"The window starts more than %s ago; related logs may have expired under the configured retention policy (%s). Their absence is not proof that no error was logged.",
			hours(retention.Logs), hours(retention.Logs))})
	}
	return warnings
}

func hours(d time.Duration) string {
	return strconv.FormatFloat(d.Hours(), 'f', -1, 64) + " h"
}
