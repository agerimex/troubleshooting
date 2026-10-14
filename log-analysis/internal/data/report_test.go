package data

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

var reportNow = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

func ns(t time.Time) string { return strconv.FormatInt(t.UnixNano(), 10) }

func reportRequest() ReportRequest {
	r := ReportRequest{TraceId: traceA, TimeFrom: ns(reportNow.Add(-time.Hour)), TimeTo: ns(reportNow)}
	if err := r.Validate(); err != nil {
		panic(err)
	}
	return r
}

// fakeSources returns n spans and m logs; a nil error func means success.
type fakeSources struct {
	spans, logs                              int
	spanErr, spanCountErr, logErr, logCntErr error
	gotSpanFilter                            SpanFilter
	gotLogFilter                             LogFilter
}

func (f *fakeSources) sources() ReportSources {
	return ReportSources{
		Spans: func(_ context.Context, filter SpanFilter) (SpanPage, error) {
			f.gotSpanFilter = filter
			if f.spanErr != nil {
				return SpanPage{}, f.spanErr
			}
			page := SpanPage{}
			for i := 0; i < f.spans && i < filter.RowsPerPage; i++ {
				page.Spans = append(page.Spans, &Span{TraceId: traceA, SpanId: spanA})
			}
			page.HasMore = f.spans > filter.RowsPerPage
			return page, nil
		},
		CountSpans: func(context.Context, SpanFilter) (uint64, error) {
			return uint64(f.spans), f.spanCountErr
		},
		Logs: func(_ context.Context, filter LogFilter) (LogPage, error) {
			f.gotLogFilter = filter
			if f.logErr != nil {
				return LogPage{}, f.logErr
			}
			page := LogPage{}
			for i := 0; i < f.logs && i < filter.RowsPerPage; i++ {
				page.Logs = append(page.Logs, &LogRecord{RecordId: strconv.Itoa(i), TraceId: traceA})
			}
			page.HasMore = f.logs > filter.RowsPerPage
			return page, nil
		},
		CountLogs: func(context.Context, LogFilter) (uint64, error) {
			return uint64(f.logs), f.logCntErr
		},
		Now: func() time.Time { return reportNow },
	}
}

func TestReportCombinesBothSourcesForOneTrace(t *testing.T) {
	src := &fakeSources{spans: 2, logs: 3}
	report := BuildReport(context.Background(), reportRequest(), src.sources())

	if !report.Complete || report.Spans.Status != SectionOK || report.Logs.Status != SectionOK {
		t.Fatalf("report = %+v, want complete with both sections ok", report)
	}
	if report.Spans.Shown != 2 || report.Logs.Shown != 3 || *report.Spans.Total != 2 || *report.Logs.Total != 3 {
		t.Fatalf("shown/total = %d/%d spans, %d/%d logs", report.Spans.Shown, *report.Spans.Total, report.Logs.Shown, *report.Logs.Total)
	}
	// Only the trace and the window select evidence: no other filter carries over.
	if f := src.gotSpanFilter; f.TraceId != traceA || f.Scope != ScopeAll || f.Status != "" || f.ServiceName != "" {
		t.Errorf("span filter = %+v", f)
	}
	if f := src.gotLogFilter; f.TraceId != traceA || f.Severity != "" || f.ServiceName != "" || f.Message != "" {
		t.Errorf("log filter = %+v", f)
	}
	if report.TraceId != traceA || report.GeneratedAt != ns(reportNow) || len(report.Warnings) != 0 {
		t.Errorf("report header = %+v", report)
	}
}

// M1-CMP-001: one failed source keeps the other's evidence and is never zero.
func TestReportKeepsEvidenceWhenOneSourceFails(t *testing.T) {
	unreachable := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	for _, failed := range []string{SectionSpans, SectionLogs} {
		src := &fakeSources{spans: 2, logs: 3}
		if failed == SectionSpans {
			src.spanErr = unreachable
		} else {
			src.logErr = unreachable
		}
		report := BuildReport(context.Background(), reportRequest(), src.sources())

		if report.Complete {
			t.Errorf("%s failed: report is complete", failed)
		}
		bad, good := report.Logs.Status, report.Spans.Status
		badTotal, badShown := report.Logs.Total, report.Logs.Shown
		if failed == SectionSpans {
			bad, good = report.Spans.Status, report.Logs.Status
			badTotal, badShown = report.Spans.Total, report.Spans.Shown
		}
		if bad != SectionUnavailable || good != SectionOK {
			t.Errorf("%s failed: statuses %q (failed) and %q (other)", failed, bad, good)
		}
		if badTotal != nil || badShown != 0 {
			t.Errorf("%s failed: the failed section reports a count", failed)
		}
	}
}

// M1-CMP-003: a limit gives exactly the limit, marked truncated only when more match.
func TestReportLimitIsShownAndTruncationIsExact(t *testing.T) {
	for _, tc := range []struct {
		available int
		truncated bool
	}{{3, false}, {4, true}} {
		req := reportRequest()
		req.SpanLimit, req.LogLimit = 3, 3
		src := &fakeSources{spans: tc.available, logs: tc.available}
		report := BuildReport(context.Background(), req, src.sources())
		for name, s := range map[string]struct {
			shown, limit int
			truncated    bool
		}{
			"spans": {report.Spans.Shown, report.Spans.Limit, report.Spans.Truncated},
			"logs":  {report.Logs.Shown, report.Logs.Limit, report.Logs.Truncated},
		} {
			if s.shown != 3 || s.limit != 3 || s.truncated != tc.truncated {
				t.Errorf("%d available %s: shown %d limit %d truncated %v", tc.available, name, s.shown, s.limit, s.truncated)
			}
		}
		if report.Complete == tc.truncated {
			t.Errorf("%d available: complete = %v", tc.available, report.Complete)
		}
	}
}

// M1-CMP-007: a timeout is a limit status, with no partial records.
func TestReportTimeoutIsNotPartial(t *testing.T) {
	src := &fakeSources{spans: 2, logs: 3, logErr: &clickhouse.Exception{Code: chTimeoutExceeded}}
	report := BuildReport(context.Background(), reportRequest(), src.sources())
	if report.Logs.Status != SectionLimit || len(report.Logs.Records) != 0 || report.Complete {
		t.Fatalf("logs = %+v, complete %v", report.Logs, report.Complete)
	}
	if !strings.Contains(report.Logs.Error, "took longer") {
		t.Errorf("error %q doesn't name the timeout", report.Logs.Error)
	}
}

// M1-CMP-008: a failed count is unavailable, never zero, and the records stay.
func TestReportFailedTotalIsUnavailable(t *testing.T) {
	src := &fakeSources{spans: 2, logs: 3, logCntErr: errors.New("boom")}
	report := BuildReport(context.Background(), reportRequest(), src.sources())
	if report.Logs.Status != SectionOK || report.Logs.Shown != 3 || report.Logs.Total != nil || report.Logs.TotalStatus != SectionUnavailable {
		t.Fatalf("logs = %+v", report.Logs)
	}
}

// M1-CMP-006: a window older than the log retention warns about expired logs.
func TestReportWarnsAboutRetention(t *testing.T) {
	SetRetention(RetentionPolicy{Traces: 96 * time.Hour, Logs: 24 * time.Hour})
	defer SetRetention(DefaultRetention)

	req := ReportRequest{TraceId: traceA, TimeFrom: ns(reportNow.Add(-48 * time.Hour)), TimeTo: ns(reportNow.Add(-47 * time.Hour))}
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	report := BuildReport(context.Background(), req, (&fakeSources{spans: 2}).sources())
	if len(report.Warnings) != 1 || report.Warnings[0].Section != SectionLogs || !strings.Contains(report.Warnings[0].Message, "not proof") {
		t.Fatalf("warnings = %+v, want one log retention warning", report.Warnings)
	}
}

func TestReportRetryOfOneSection(t *testing.T) {
	req := reportRequest()
	req.Sections = []string{SectionLogs}
	report := BuildReport(context.Background(), req, (&fakeSources{spans: 2, logs: 3}).sources())
	if report.Spans != nil || report.Logs == nil || !report.Complete {
		t.Fatalf("report = %+v, want only the logs section", report)
	}
}

func TestReportRequestValidate(t *testing.T) {
	for name, req := range map[string]ReportRequest{
		"no trace":        {TimeFrom: "1", TimeTo: "2"},
		"bad trace":       {TraceId: "abc", TimeFrom: "1", TimeTo: "2"},
		"no window":       {TraceId: traceA},
		"unknown section": {TraceId: traceA, TimeFrom: "1", TimeTo: "2", Sections: []string{"metrics"}},
	} {
		if err := req.Validate(); !errors.Is(err, ErrInvalidFilter) {
			t.Errorf("%s: Validate = %v, want ErrInvalidFilter", name, err)
		}
	}
	req := ReportRequest{TraceId: strings.ToUpper(traceA), TimeFrom: "1", TimeTo: "2", SpanLimit: 5000}
	if err := req.Validate(); err != nil || req.TraceId != traceA || req.SpanLimit != MaxRowsPerPage ||
		req.LogLimit != MaxLogRowsPerPage || len(req.Sections) != 2 {
		t.Fatalf("Validate = %v, request %+v", err, req)
	}
}

func TestLogFilterValidate(t *testing.T) {
	for name, f := range map[string]LogFilter{
		"no window":         {},
		"bad trace":         {TimeFrom: "1", TimeTo: "2", TraceId: "xyz"},
		"cursor without id": {TimeFrom: "1", TimeTo: "2", AfterTime: "1"},
	} {
		if err := f.Validate(); !errors.Is(err, ErrInvalidFilter) {
			t.Errorf("%s: Validate = %v, want ErrInvalidFilter", name, err)
		}
	}
	f := LogFilter{TimeFrom: "1", TimeTo: "2", Severity: " error ", RowsPerPage: 99999}
	if err := f.Validate(); err != nil || f.Severity != "ERROR" || f.RowsPerPage != MaxLogRowsPerPage {
		t.Fatalf("Validate = %v, filter %+v", err, f)
	}
}

func TestEveryLogPlaceholderIsBound(t *testing.T) {
	filter := LogFilter{TimeFrom: "1", TimeTo: "2", TraceId: traceA, ServiceName: "s", Severity: "ERROR",
		Message: "m", AfterTime: "1", AfterRecordId: "r"}
	conds, params := filter.conditions(true)
	params = append(params, filter.windowParams()...)
	query := canonicalLogs(conds) + whereClause(conds)

	bound := map[string]bool{}
	for _, param := range params {
		bound[param.(driver.NamedValue).Name] = true
	}
	for _, m := range namedPlaceholder.FindAllStringSubmatch(query, -1) {
		if !bound[m[1]] {
			t.Errorf("placeholder %q is used but not bound", m[1])
		}
	}
	// Message and service are substrings, not LIKE patterns: % and _ are literal.
	if strings.Contains(query, "LIKE") {
		t.Errorf("log search must not use LIKE:\n%s", query)
	}
	if !strings.Contains(query, `LIMIT 1 BY "RecordId"`) {
		t.Errorf("log records are not deduplicated by RecordId:\n%s", query)
	}
}
