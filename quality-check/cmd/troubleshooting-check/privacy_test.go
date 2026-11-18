package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func privacyServer(t *testing.T, findings string, got *privacyRequest) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/privacy/scan" {
			w.WriteHeader(404)
			io.WriteString(w, `{"error":true,"message":"404"}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, got)
		io.WriteString(w, `{"error":false,"data":{"Privacy":{"service":"app","time_from":"1794859200000000000","time_to":"1794945600000000000","spans":12,"logs":3,"findings":`+findings+`}}}`)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestPrivacyCommand(t *testing.T) {
	var got privacyRequest
	s := privacyServer(t, `[{"kind":"url_credentials","source":"spans","records":2,"trace_id":"aa","span_id":"bb","what":"a URL with user and password"}]`, &got)
	var out, errOut bytes.Buffer
	if code := run([]string{"privacy", "--url", s.URL, "--service", "app"}, &out, &errOut, fixedNow); code != 1 {
		t.Fatalf("findings: exit %d %s", code, errOut.String())
	}
	if got.Service != "app" || got.TimeFrom != "" {
		t.Errorf("request %+v: no window means the API's default", got)
	}
	for _, want := range []string{"Privacy scan of service app", "12 spans, 3 log records", "FOUND 1 kinds", "url_credentials  2 spans", "example: trace aa, span bb", "/#/executions?trace=aa&span=bb"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout is missing %q:\n%s", want, out.String())
		}
	}

	clean := privacyServer(t, `[]`, &got)
	out.Reset()
	if code := run([]string{"privacy", "--url", clean.URL, "--run", "ci-1", "--since", "2h"}, &out, io.Discard, fixedNow); code != 0 || !strings.Contains(out.String(), "Nothing found") {
		t.Errorf("clean: exit %d %s", code, out.String())
	}
	if got.Run != "ci-1" || got.TimeFrom == "" {
		t.Errorf("request %+v", got)
	}
	if code := run([]string{"privacy", "--url", clean.URL}, io.Discard, io.Discard, fixedNow); code != 3 {
		t.Errorf("no run or service: exit %d", code)
	}
	old := httptest.NewServer(http.NotFoundHandler())
	defer old.Close()
	errOut.Reset()
	if code := run([]string{"privacy", "--url", old.URL, "--service", "app"}, io.Discard, &errOut, fixedNow); code != 3 {
		t.Errorf("old API: exit %d %s", code, errOut.String())
	}
}
