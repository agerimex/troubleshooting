package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// The fixture's own view of the log-analysis API. It deliberately doesn't
// import log-analysis: expected answers must not come from production code.

type apiEvent struct {
	TimeUnixNano string            `json:"timeUnixNano"`
	Name         string            `json:"name"`
	Attributes   map[string]string `json:"attributes"`
}

type apiLink struct {
	TraceID    string            `json:"traceId"`
	SpanID     string            `json:"spanId"`
	Attributes map[string]string `json:"attributes"`
}

type apiSpan struct {
	TimeStamp      string              `json:"timeStamp"`
	Name           string              `json:"name"`
	Service        string              `json:"service"`
	TraceID        string              `json:"traceId"`
	SpanID         string              `json:"spanId"`
	ParentSpanID   string              `json:"parentSpanId"`
	Tags           []map[string]string `json:"tags"`
	ServiceTags    []map[string]string `json:"serviceTags"`
	ChildSpanCount int                 `json:"childSpanCount"`
	Status         string              `json:"status"`
	StatusMessage  string              `json:"statusMessage"`
	Duration       int64               `json:"duration"` // microseconds
	Events         []apiEvent          `json:"events"`
	Links          []apiLink           `json:"links"`
}

func (s apiSpan) key() string { return s.TraceID + ":" + s.SpanID }

func tagValue(tags []map[string]string, key string) string {
	for _, t := range tags {
		if t["key"] == key {
			return t["value"]
		}
	}
	return ""
}

type filter struct {
	TimeFrom     string `json:"time_from"`
	TimeTo       string `json:"time_to"`
	Scope        string `json:"scope,omitempty"`
	TraceID      string `json:"trace_id,omitempty"`
	ParentID     string `json:"parent_id,omitempty"`
	Status       string `json:"status,omitempty"`
	ServiceName  string `json:"service_name,omitempty"`
	MethodName   string `json:"method_name,omitempty"`
	RowsPerPage  int    `json:"rows_per_page,omitempty"`
	AfterTime    string `json:"after_time,omitempty"`
	AfterTraceID string `json:"after_trace_id,omitempty"`
	AfterSpanID  string `json:"after_span_id,omitempty"`
}

func ns(t time.Time) string { return strconv.FormatInt(t.UnixNano(), 10) }

func windowFilter(from, to time.Time) filter {
	return filter{TimeFrom: ns(from), TimeTo: ns(to)}
}

type api struct {
	base   string
	client *http.Client
}

func newAPI(base string) *api {
	return &api{base: base, client: &http.Client{Timeout: 60 * time.Second}}
}

// apiError is a non-2xx answer or an {"error": true} body.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message) }

func (a *api) post(ctx context.Context, path string, body any, data any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var envelope struct {
		Error   bool            `json:"error"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return &apiError{Status: resp.StatusCode, Message: "not JSON: " + string(payload)}
	}
	if resp.StatusCode/100 != 2 || envelope.Error {
		return &apiError{Status: resp.StatusCode, Message: envelope.Message}
	}
	return json.Unmarshal(envelope.Data, data)
}

// spans returns one page. Every page is checked for duplicate identities,
// which the UI would show as duplicate nodes.
func (a *api) spans(ctx context.Context, f filter) ([]apiSpan, bool, error) {
	var data struct {
		Spans   []apiSpan `json:"Spans"`
		HasMore bool      `json:"HasMore"`
	}
	if err := a.post(ctx, "/api/v1/view-spans", f, &data); err != nil {
		return nil, false, err
	}
	seen := map[string]bool{}
	for _, s := range data.Spans {
		if seen[s.key()] {
			return nil, false, fmt.Errorf("page contains %s twice", s.key())
		}
		seen[s.key()] = true
	}
	return data.Spans, data.HasMore, nil
}

// all pages through f with the cursor and returns every row.
func (a *api) all(ctx context.Context, f filter) ([]apiSpan, error) {
	var out []apiSpan
	for page := 0; ; page++ {
		if page > 10000 {
			return nil, fmt.Errorf("paging does not end")
		}
		rows, more, err := a.spans(ctx, f)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		if !more {
			return out, nil
		}
		if len(rows) == 0 {
			return nil, fmt.Errorf("HasMore with an empty page")
		}
		last := rows[len(rows)-1]
		f.AfterTime, f.AfterTraceID, f.AfterSpanID = last.TimeStamp, last.TraceID, last.SpanID
	}
}

func (a *api) count(ctx context.Context, f filter) (int, error) {
	var data struct {
		Count int `json:"Count"`
	}
	f.AfterTime, f.AfterTraceID, f.AfterSpanID = "", "", ""
	err := a.post(ctx, "/api/v1/count-spans", f, &data)
	return data.Count, err
}

type apiLog struct {
	TimeUnixNano string            `json:"timeUnixNano"`
	RecordID     string            `json:"recordId"`
	TraceID      string            `json:"traceId"`
	SpanID       string            `json:"spanId"`
	Severity     string            `json:"severity"`
	Service      string            `json:"service"`
	Message      string            `json:"message"`
	Attributes   map[string]string `json:"attributes"`
}

type logFilter struct {
	TimeFrom      string `json:"time_from"`
	TimeTo        string `json:"time_to"`
	TraceID       string `json:"trace_id,omitempty"`
	ServiceName   string `json:"service_name,omitempty"`
	Severity      string `json:"severity,omitempty"`
	Message       string `json:"message,omitempty"`
	RowsPerPage   int    `json:"rows_per_page,omitempty"`
	AfterTime     string `json:"after_time,omitempty"`
	AfterRecordID string `json:"after_record_id,omitempty"`
}

// logs returns one page of log records; a record id twice in a page is an error.
func (a *api) logs(ctx context.Context, f logFilter) ([]apiLog, bool, error) {
	var data struct {
		Logs    []apiLog `json:"Logs"`
		HasMore bool     `json:"HasMore"`
	}
	if err := a.post(ctx, "/api/v1/search-logs", f, &data); err != nil {
		return nil, false, err
	}
	seen := map[string]bool{}
	for _, l := range data.Logs {
		if seen[l.RecordID] {
			return nil, false, fmt.Errorf("page contains record %s twice", l.RecordID)
		}
		seen[l.RecordID] = true
	}
	return data.Logs, data.HasMore, nil
}

// allLogs pages through f with the cursor.
func (a *api) allLogs(ctx context.Context, f logFilter) ([]apiLog, error) {
	var out []apiLog
	for page := 0; ; page++ {
		if page > 10000 {
			return nil, fmt.Errorf("paging does not end")
		}
		rows, more, err := a.logs(ctx, f)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		if !more {
			return out, nil
		}
		if len(rows) == 0 {
			return nil, fmt.Errorf("HasMore with an empty page")
		}
		last := rows[len(rows)-1]
		f.AfterTime, f.AfterRecordID = last.TimeUnixNano, last.RecordID
	}
}

type reportRequest struct {
	TraceID   string   `json:"trace_id"`
	TimeFrom  string   `json:"time_from"`
	TimeTo    string   `json:"time_to"`
	SpanLimit int      `json:"span_limit,omitempty"`
	LogLimit  int      `json:"log_limit,omitempty"`
	Sections  []string `json:"sections,omitempty"`
}

type apiSection[T any] struct {
	Status      string `json:"status"`
	Error       string `json:"error"`
	Records     []T    `json:"records"`
	Limit       int    `json:"limit"`
	Shown       int    `json:"shown"`
	Truncated   bool   `json:"truncated"`
	Total       *int   `json:"total"`
	TotalStatus string `json:"totalStatus"`
}

type apiReport struct {
	TraceID     string               `json:"traceId"`
	TimeFrom    string               `json:"timeFrom"`
	TimeTo      string               `json:"timeTo"`
	GeneratedAt string               `json:"generatedAt"`
	Spans       *apiSection[apiSpan] `json:"spans"`
	Logs        *apiSection[apiLog]  `json:"logs"`
	Warnings    []struct {
		Code    string `json:"code"`
		Section string `json:"section"`
		Message string `json:"message"`
	} `json:"warnings"`
	Complete bool `json:"complete"`
}

func (a *api) report(ctx context.Context, req reportRequest) (apiReport, error) {
	var data struct {
		Report apiReport `json:"Report"`
	}
	err := a.post(ctx, "/api/v1/trace-report", req, &data)
	return data.Report, err
}
