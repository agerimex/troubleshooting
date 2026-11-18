package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// troubleshooting-check privacy: scans the stored spans and logs of a run or
// a service for what an application must never send (SQL arguments from a
// sender that didn't opt in, user:password in URLs, tokens and keys in query
// strings, bearer tokens, password hashes). It prints each kind of finding
// with its count and one example span, never the value, and exits 0 when
// nothing was found, 1 when something was, 3 on an error. Run it in CI after
// the gate, or once after integrating an application.

type privacyRequest struct {
	Run      string `json:"run,omitempty"`
	Service  string `json:"service,omitempty"`
	TimeFrom string `json:"time_from,omitempty"`
	TimeTo   string `json:"time_to,omitempty"`
}

type privacyFinding struct {
	Kind    string `json:"kind"`
	Source  string `json:"source"`
	Records uint64 `json:"records"`
	TraceID string `json:"trace_id"`
	SpanID  string `json:"span_id"`
	What    string `json:"what"`
}

type privacyResult struct {
	Run      string           `json:"run,omitempty"`
	Service  string           `json:"service,omitempty"`
	TimeFrom string           `json:"time_from"`
	TimeTo   string           `json:"time_to"`
	Spans    uint64           `json:"spans"`
	Logs     uint64           `json:"logs"`
	Findings []privacyFinding `json:"findings"`
}

func runPrivacy(args []string, stdout, stderr io.Writer, now func() time.Time) int {
	fs := flag.NewFlagSet("troubleshooting-check privacy", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o options
	var service, jsonPath string
	fs.StringVar(&o.url, "url", envOr("TROUBLESHOOTING_URL", "http://localhost:8095"), "Troubleshooting URL (the UI origin, which serves /api; or the API itself)")
	fs.StringVar(&o.ui, "ui", "", "UI URL for example links (default: --url); \"none\" for no links")
	fs.StringVar(&o.run, "run", "", "scan the spans and logs of this run (troubleshooting.run)")
	fs.StringVar(&service, "service", "", "scan the spans and logs of this service")
	fs.DurationVar(&o.since, "since", 0, "window: records of this long before now (default: the last 24 h)")
	fs.StringVar(&o.from, "from", "", "window start, RFC 3339")
	fs.StringVar(&o.to, "to", "", "window end, RFC 3339 (default now)")
	fs.StringVar(&jsonPath, "json", "", "write the scan result as JSON to this file")
	fs.DurationVar(&o.timeout, "timeout", 2*time.Minute, "request timeout")
	o.basicAuth = os.Getenv("TROUBLESHOOTING_BASIC_AUTH")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if o.ui == "" {
		o.ui = o.url
	}
	req := privacyRequest{Run: strings.TrimSpace(o.run), Service: strings.TrimSpace(service)}
	if req.Run == "" && req.Service == "" {
		fmt.Fprintln(stderr, "error: --run or --service is required")
		return exitError
	}
	if o.since != 0 || o.from != "" || o.to != "" {
		since := o.since
		if since == 0 {
			since = 24 * time.Hour
		}
		var err error
		if req.TimeFrom, req.TimeTo, err = window(o.from, o.to, since, now()); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return exitError
		}
	}
	res, err := scanPrivacy(o, req)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitError
	}
	if jsonPath != "" {
		b, _ := json.MarshalIndent(res, "", "  ")
		if err := os.WriteFile(jsonPath, append(b, '\n'), 0o644); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return exitError
		}
	}
	fmt.Fprint(stdout, privacyText(res, o.ui))
	if len(res.Findings) > 0 {
		return exitFailed
	}
	return exitPassed
}

func scanPrivacy(o options, req privacyRequest) (privacyResult, error) {
	endpoint := strings.TrimRight(o.url, "/") + "/api/v1/privacy/scan"
	status, raw, err := postJSON(o, "/api/v1/privacy/scan", req, 1<<20)
	if err != nil {
		return privacyResult{}, err
	}
	var out struct {
		Error   bool   `json:"error"`
		Message string `json:"message"`
		Data    *struct {
			Privacy *privacyResult `json:"Privacy"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return privacyResult{}, fmt.Errorf("%s: HTTP %d, not a Troubleshooting API answer", endpoint, status)
	}
	if status == http.StatusNotFound {
		return privacyResult{}, fmt.Errorf("%s: HTTP 404: this Troubleshooting has no privacy scan (update it)", endpoint)
	}
	if status != http.StatusOK || out.Error {
		return privacyResult{}, fmt.Errorf("%s: HTTP %d: %s", endpoint, status, out.Message)
	}
	if out.Data == nil || out.Data.Privacy == nil {
		return privacyResult{}, errors.New(endpoint + ": the answer has no scan result")
	}
	return *out.Data.Privacy, nil
}

func privacyText(res privacyResult, ui string) string {
	var b strings.Builder
	what := "run " + res.Run
	if res.Run == "" {
		what = "service " + res.Service
	} else if res.Service != "" {
		what += ", service " + res.Service
	}
	fmt.Fprintf(&b, "Privacy scan of %s, %s: %d spans, %d log records\n", what, windowText(res.TimeFrom, res.TimeTo), res.Spans, res.Logs)
	if len(res.Findings) == 0 {
		b.WriteString("\nNothing found: no SQL arguments from a sender that didn't opt in, no URL credentials, tokens in query strings, bearer tokens or password hashes.\n")
		b.WriteString("Note: the scan knows these patterns only; names, emails or other personal data in your own attributes need a review.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "\nFOUND %d kinds of leak (values not shown):\n", len(res.Findings))
	for _, f := range res.Findings {
		fmt.Fprintf(&b, "  %-16s %d %s: %s\n", f.Kind, f.Records, f.Source, f.What)
		if f.TraceID != "" {
			fmt.Fprintf(&b, "  %-16s example: trace %s, span %s", "", f.TraceID, f.SpanID)
			if ui != "none" && f.Source == "spans" {
				fmt.Fprintf(&b, "\n  %-16s %s/#/executions?trace=%s&span=%s&from=%s&to=%s", "", strings.TrimRight(ui, "/"), f.TraceID, f.SpanID, res.TimeFrom, res.TimeTo)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\nRemove them in the application before export (log-sender redacts these by default; see the README's integration guide), then purge what was stored.\n")
	return b.String()
}
