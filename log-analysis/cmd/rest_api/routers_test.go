package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
