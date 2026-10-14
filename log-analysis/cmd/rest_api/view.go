package main

import (
	"errors"
	"net/http"

	"logs-backend/internal/data"
)

// readSpanFilter reads and validates the body of view-spans and count-spans.
// It answers 400 and returns false if the body is not a valid filter.
func (app *application) readSpanFilter(w http.ResponseWriter, r *http.Request) (data.SpanFilter, bool) {
	var filter data.SpanFilter
	if err := app.readJSON(w, r, &filter); err != nil {
		app.errorJSON(r.Context(), w, err)
		return filter, false
	}
	if err := filter.Validate(); err != nil {
		app.errorJSON(r.Context(), w, err)
		return filter, false
	}
	return filter, true
}

// queryError answers a failed query: 504 when it hit a query limit or the
// request deadline (the message names the limit), 503 when ClickHouse can't
// be reached, 500 otherwise. Either way no partial result is sent.
func (app *application) queryError(w http.ResponseWriter, r *http.Request, err error) {
	app.errorLog.Println(err)
	err = data.ExplainQueryError(r.Context(), err)
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, data.ErrQueryLimit):
		status = http.StatusGatewayTimeout
	case errors.Is(err, data.ErrUnavailable):
		status = http.StatusServiceUnavailable
	}
	app.errorJSON(r.Context(), w, err, status)
}

func (app *application) viewSpans(w http.ResponseWriter, r *http.Request) {
	filter, ok := app.readSpanFilter(w, r)
	if !ok {
		return
	}

	page, err := app.models.Log.SelectSpans(r.Context(), filter)
	if err != nil {
		app.queryError(w, r, err)
		return
	}

	spans := page.Spans
	if spans == nil {
		spans = []*data.Span{}
	}
	payload := jsonResponse{
		Error:   false,
		Message: "success",
		Data:    envelope{"Spans": spans, "HasMore": page.HasMore},
	}

	app.writeJSON(r.Context(), w, http.StatusOK, payload)
}

func (app *application) countSpans(w http.ResponseWriter, r *http.Request) {
	filter, ok := app.readSpanFilter(w, r)
	if !ok {
		return
	}

	count, err := app.models.Log.SelectCountSpans(r.Context(), filter)
	if err != nil {
		app.queryError(w, r, err)
		return
	}

	payload := jsonResponse{
		Error:   false,
		Message: "success",
		Data:    envelope{"Count": count},
	}

	app.writeJSON(r.Context(), w, http.StatusOK, payload)
}

func (app *application) searchLogs(w http.ResponseWriter, r *http.Request) {
	var filter data.LogFilter
	if err := app.readJSON(w, r, &filter); err != nil {
		app.errorJSON(r.Context(), w, err)
		return
	}
	if err := filter.Validate(); err != nil {
		app.errorJSON(r.Context(), w, err)
		return
	}

	page, err := app.models.Log.SelectLogs(r.Context(), filter)
	if err != nil {
		app.queryError(w, r, err)
		return
	}

	logs := page.Logs
	if logs == nil {
		logs = []*data.LogRecord{}
	}
	payload := jsonResponse{
		Error:   false,
		Message: "success",
		Data:    envelope{"Logs": logs, "HasMore": page.HasMore},
	}

	app.writeJSON(r.Context(), w, http.StatusOK, payload)
}

// traceReport answers 200 even when a source failed: the failure is in that
// section's status, next to the evidence of the source that responded.
func (app *application) traceReport(w http.ResponseWriter, r *http.Request) {
	var req data.ReportRequest
	if err := app.readJSON(w, r, &req); err != nil {
		app.errorJSON(r.Context(), w, err)
		return
	}
	if err := req.Validate(); err != nil {
		app.errorJSON(r.Context(), w, err)
		return
	}

	report := data.BuildReport(r.Context(), req, app.models.Log.Sources())
	for _, failure := range []string{sectionError(report.Spans), sectionError(report.Logs)} {
		if failure != "" {
			app.errorLog.Println("trace report:", failure)
		}
	}

	payload := jsonResponse{
		Error:   false,
		Message: "success",
		Data:    envelope{"Report": report},
	}
	app.writeJSON(r.Context(), w, http.StatusOK, payload)
}

func sectionError[T any](s *data.ReportSection[T]) string {
	if s == nil || s.Status == data.SectionOK {
		return ""
	}
	return s.Error
}
