package main

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCORSAllowsOnlyConfiguredOrigins(t *testing.T) {
	app := &application{serviceConfig: ServiceConfig{allowedOrigins: []string{"http://localhost:5173"}}}
	handler := app.routers()

	for origin, wantAllowed := range map[string]bool{
		"http://localhost:5173": true,
		"https://evil.example":  false,
	} {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/view-spans", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		allowed := rec.Header().Get("Access-Control-Allow-Origin") == origin
		if allowed != wantAllowed {
			t.Errorf("origin %s: allowed=%v, want %v", origin, allowed, wantAllowed)
		}
		if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Errorf("origin %s: credentials must not be allowed", origin)
		}
	}
}

func TestInvalidSpanFilterIsRejectedBeforeQuerying(t *testing.T) {
	app := &application{errorLog: log.New(io.Discard, "", 0), queryTimeout: time.Second}
	handler := app.routers()

	for _, body := range []string{
		`{}`, // no time window
		`{"time_from":"1","time_to":"2","parent_id":"0123456789abcdef"}`, // parent without trace
		`{"time_from":"1","time_to":"2","scope":"everything"}`,
	} {
		for _, path := range []string{"/api/v1/view-spans", "/api/v1/count-spans"} {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req) // no database is set: a query would panic

			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s %s: status %d, want 400", path, body, rec.Code)
			}
		}
	}
}

func TestInvalidLogAndReportRequestsAreRejectedBeforeQuerying(t *testing.T) {
	app := &application{errorLog: log.New(io.Discard, "", 0), queryTimeout: time.Second}
	handler := app.routers()

	for path, bodies := range map[string][]string{
		"/api/v1/search-logs": {`{}`, `{"time_from":"1","time_to":"2","trace_id":"nothex"}`},
		"/api/v1/trace-report": {`{"time_from":"1","time_to":"2"}`, `{"trace_id":"0123456789abcdef0123456789abcdef"}`,
			`{"trace_id":"0123456789abcdef0123456789abcdef","time_from":"1","time_to":"2","sections":["x"]}`},
	} {
		for _, body := range bodies {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req) // no database is set: a query would panic

			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s %s: status %d, want 400", path, body, rec.Code)
			}
		}
	}
}
