package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
)

// The fixture's view of the milestone 2 API (statistics, error groups,
// examples, comparisons and the test controls).

type population struct {
	Service      string  `json:"service"`
	Operation    *string `json:"operation,omitempty"`
	SpanKind     *string `json:"span_kind,omitempty"`
	Environment  *string `json:"environment,omitempty"`
	Version      *string `json:"version,omitempty"`
	Installation *string `json:"installation,omitempty"`
	Segment      *string `json:"segment,omitempty"`
	TimeFrom     string  `json:"time_from"`
	TimeTo       string  `json:"time_to"`
}

func sp(s string) *string { return &s }

type apiNotice struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type apiPercentile struct {
	ValueMs *float64 `json:"valueMs"`
	Status  string   `json:"status"`
	Minimum int      `json:"minimum"`
	Samples int      `json:"samples"`
}

type apiCallStats struct {
	Operation        string                   `json:"operation"`
	SpanKind         string                   `json:"spanKind"`
	Version          *string                  `json:"version"`
	Calls            int                      `json:"calls"`
	Errors           int                      `json:"errors"`
	Ok               int                      `json:"ok"`
	Unset            int                      `json:"unset"`
	ErrorFraction    *float64                 `json:"errorFraction"`
	RatePerMinute    float64                  `json:"ratePerMinute"`
	ValidDurations   int                      `json:"validDurations"`
	InvalidDurations int                      `json:"invalidDurations"`
	Percentiles      map[string]apiPercentile `json:"percentiles"`
	Conflicts        int                      `json:"conflicts"`
	Sampling         []string                 `json:"sampling"`
}

type apiPlanPart struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Source      string `json:"source"`
	WholeBucket bool   `json:"wholeBucket"`
	Revision    int64  `json:"revision"`
	State       string `json:"state"`
	Reason      string `json:"reason"`
}

type apiStats struct {
	Status          string         `json:"status"`
	Condition       string         `json:"condition"`
	DurationSeconds float64        `json:"durationSeconds"`
	SuggestedFrom   string         `json:"suggestedFrom"`
	SuggestedTo     string         `json:"suggestedTo"`
	Rows            []apiCallStats `json:"rows"`
	Series          []struct {
		From      string `json:"from"`
		To        string `json:"to"`
		Source    string `json:"source"`
		Coverage  string `json:"coverage"`
		Calls     *int   `json:"calls"`
		Available bool   `json:"available"`
	} `json:"series"`
	Coverage struct {
		Status string `json:"status"`
	} `json:"coverage"`
	Plan         []apiPlanPart     `json:"plan"`
	Notices      []apiNotice       `json:"notices"`
	LateExcluded int               `json:"lateExcluded"`
	Cutoff       string            `json:"cutoff"`
	Definitions  map[string]string `json:"definitions"`
}

func (s apiStats) notice(code string) (apiNotice, bool) {
	for _, n := range s.Notices {
		if n.Code == code {
			return n, true
		}
	}
	return apiNotice{}, false
}

// only returns the single row of a fully selected population.
func (s apiStats) only() (apiCallStats, error) {
	if s.Status != "ok" {
		return apiCallStats{}, fmt.Errorf("status %s (%s)", s.Status, s.Condition)
	}
	if len(s.Rows) != 1 {
		return apiCallStats{}, fmt.Errorf("%d rows, want 1", len(s.Rows))
	}
	return s.Rows[0], nil
}

type apiErrorGroup struct {
	Key            string `json:"key"`
	SourceKind     string `json:"sourceKind"`
	Service        string `json:"service"`
	Operation      string `json:"operation"`
	Environment    string `json:"environment"`
	Method         string `json:"method"`
	Fingerprint    string `json:"fingerprint"`
	ExceptionType  string `json:"exceptionType"`
	Frame          string `json:"frame"`
	Signature      string `json:"signature"`
	Occurrences    int    `json:"occurrences"`
	Unlinked       int    `json:"unlinked"`
	AffectedTraces int    `json:"affectedTraces"`
	AffectedCalls  *int   `json:"affectedCalls"`
	FirstSeen      string `json:"firstSeen"`
	LastSeen       string `json:"lastSeen"`
	Versions       []struct {
		Version     string `json:"version"`
		Occurrences int    `json:"occurrences"`
	} `json:"versions"`
}

type apiErrorGroups struct {
	Status  string `json:"status"`
	Sources map[string]struct {
		Status   string `json:"status"`
		Coverage struct {
			Status string `json:"status"`
		} `json:"coverage"`
	} `json:"sources"`
	Groups      []apiErrorGroup   `json:"groups"`
	Notices     []apiNotice       `json:"notices"`
	Definitions map[string]string `json:"definitions"`
}

func (g apiErrorGroups) find(kind string, match func(apiErrorGroup) bool) []apiErrorGroup {
	var out []apiErrorGroup
	for _, x := range g.Groups {
		if x.SourceKind == kind && match(x) {
			out = append(out, x)
		}
	}
	return out
}

type groupRef struct {
	SourceKind  string `json:"source_kind"`
	Service     string `json:"service"`
	Operation   string `json:"operation"`
	Environment string `json:"environment"`
	Fingerprint string `json:"fingerprint"`
}

func refOf(g apiErrorGroup) *groupRef {
	return &groupRef{SourceKind: g.SourceKind, Service: g.Service, Operation: g.Operation, Environment: g.Environment, Fingerprint: g.Fingerprint}
}

type examplesRequest struct {
	Population  population `json:"population"`
	Kind        string     `json:"kind"`
	ThresholdMs float64    `json:"threshold_ms,omitempty"`
	Group       *groupRef  `json:"group,omitempty"`
	Limit       int        `json:"limit,omitempty"`
	Cutoff      string     `json:"cutoff,omitempty"`
}

type apiExample struct {
	SourceKind string   `json:"sourceKind"`
	TraceID    string   `json:"traceId"`
	SpanID     string   `json:"spanId"`
	RecordID   string   `json:"recordId"`
	Time       string   `json:"time"`
	DurationMs *float64 `json:"durationMs"`
	Version    string   `json:"version"`
	Message    string   `json:"message"`
}

type apiExamples struct {
	Status    string       `json:"status"`
	Condition string       `json:"condition"`
	Examples  []apiExample `json:"examples"`
	Limit     int          `json:"limit"`
	Shown     int          `json:"shown"`
	Total     int          `json:"total"`
	Truncated bool         `json:"truncated"`
}

type apiMetric struct {
	Metric         string   `json:"metric"`
	Baseline       *float64 `json:"baseline"`
	Candidate      *float64 `json:"candidate"`
	Absolute       *float64 `json:"absolute"`
	Relative       *float64 `json:"relative"`
	Classification string   `json:"classification"`
	Reasons        []string `json:"reasons"`
}

type apiScope struct {
	Occurrences *int   `json:"occurrences"`
	FirstSeen   string `json:"firstSeen"`
	LastSeen    string `json:"lastSeen"`
}

type apiAnalysis struct {
	ID          string   `json:"id"`
	Regenerates string   `json:"regenerates"`
	GeneratedAt string   `json:"generatedAt"`
	Baseline    apiStats `json:"baseline"`
	Candidate   apiStats `json:"candidate"`
	Rules       struct {
		Version string `json:"version"`
	} `json:"rules"`
	Mismatches []struct {
		Dimension string `json:"dimension"`
	} `json:"mismatches"`
	Rows []struct {
		Operation string        `json:"operation"`
		SpanKind  string        `json:"spanKind"`
		Baseline  *apiCallStats `json:"baseline"`
		Candidate *apiCallStats `json:"candidate"`
		Metrics   []apiMetric   `json:"metrics"`
		Notes     []string      `json:"notes"`
	} `json:"rows"`
	ErrorGroups []struct {
		apiErrorGroup
		Label     string   `json:"label"`
		Baseline  apiScope `json:"baseline"`
		Candidate apiScope `json:"candidate"`
	} `json:"errorGroups"`
	Notices        []apiNotice `json:"notices"`
	Interpretation string      `json:"interpretation"`
}

func (a apiAnalysis) metric(row int, name string) (apiMetric, error) {
	if row >= len(a.Rows) {
		return apiMetric{}, fmt.Errorf("analysis has %d rows", len(a.Rows))
	}
	for _, m := range a.Rows[row].Metrics {
		if m.Metric == name {
			return m, nil
		}
	}
	return apiMetric{}, fmt.Errorf("no metric %s", name)
}

type compareRequest struct {
	Baseline    population `json:"baseline"`
	Candidate   population `json:"candidate"`
	Regenerates string     `json:"regenerates,omitempty"`
}

type bucketOutcome struct {
	BucketStart string `json:"bucketStart"`
	Outcome     string `json:"outcome"`
	Revision    int64  `json:"revision"`
	State       string `json:"state"`
	Error       string `json:"error"`
}

type coverageDeclaration struct {
	Source   string `json:"source"`
	TimeFrom string `json:"time_from"`
	TimeTo   string `json:"time_to"`
	Status   string `json:"status"`
	Reason   string `json:"reason"`
}

type apiPolicy struct {
	TracesSeconds int64 `json:"tracesSeconds"`
	LogsSeconds   int64 `json:"logsSeconds"`
	StatsSeconds  int64 `json:"statsSeconds"`
	Compatible    bool  `json:"compatible"`
}

func (a *api) stats(ctx context.Context, p population, byVersion, series bool) (apiStats, error) {
	var data struct {
		Stats apiStats `json:"Stats"`
	}
	err := a.post(ctx, "/api/v1/stats/calls", map[string]any{"population": p, "group_by_version": byVersion, "series": series}, &data)
	return data.Stats, err
}

func (a *api) errorGroups(ctx context.Context, p population) (apiErrorGroups, error) {
	var data struct {
		ErrorGroups apiErrorGroups `json:"ErrorGroups"`
	}
	err := a.post(ctx, "/api/v1/stats/error-groups", map[string]any{"population": p}, &data)
	return data.ErrorGroups, err
}

func (a *api) examples(ctx context.Context, req examplesRequest) (apiExamples, error) {
	var data struct {
		Examples apiExamples `json:"Examples"`
	}
	err := a.post(ctx, "/api/v1/stats/examples", req, &data)
	return data.Examples, err
}

func (a *api) compare(ctx context.Context, req compareRequest) (apiAnalysis, error) {
	var data struct {
		Analysis apiAnalysis `json:"Analysis"`
	}
	err := a.post(ctx, "/api/v1/compare", req, &data)
	return data.Analysis, err
}

func (a *api) rebuild(ctx context.Context, from, to string) ([]bucketOutcome, error) {
	var data struct {
		Buckets []bucketOutcome `json:"Buckets"`
	}
	err := a.post(ctx, "/api/v1/stats/rebuild", map[string]string{"time_from": from, "time_to": to}, &data)
	return data.Buckets, err
}

func (a *api) declare(ctx context.Context, d coverageDeclaration) error {
	var data any
	return a.post(ctx, "/api/v1/coverage", d, &data)
}

func (a *api) setClock(ctx context.Context, offsetSeconds int64) error {
	var data any
	return a.post(ctx, "/api/v1/admin/clock", map[string]int64{"offset_seconds": offsetSeconds}, &data)
}

func (a *api) failNext(ctx context.Context, n int) error {
	var data any
	return a.post(ctx, "/api/v1/admin/stats/fail-next", map[string]int{"count": n}, &data)
}

// get reads a GET endpoint's data into out.
func (a *api) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.base+path, nil)
	if err != nil {
		return err
	}
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
	return json.Unmarshal(envelope.Data, out)
}

func (a *api) analysis(ctx context.Context, id string) (apiAnalysis, error) {
	var data struct {
		Analysis apiAnalysis `json:"Analysis"`
	}
	err := a.get(ctx, "/api/v1/analyses/"+id, &data)
	return data.Analysis, err
}

func (a *api) policy(ctx context.Context) (apiPolicy, error) {
	var data struct {
		Policy apiPolicy `json:"Policy"`
	}
	err := a.get(ctx, "/api/v1/retention", &data)
	return data.Policy, err
}

func (a *api) refresherFailures(ctx context.Context) (map[string]string, error) {
	var data struct {
		Status struct {
			Failures map[string]string `json:"failures"`
		} `json:"Status"`
	}
	err := a.get(ctx, "/api/v1/stats/status", &data)
	return data.Status.Failures, err
}

// near checks a value within tolerance.
func near(what string, got *float64, want, tol float64) error {
	if got == nil {
		return fmt.Errorf("%s is unavailable, want %g", what, want)
	}
	if math.Abs(*got-want) > tol {
		return fmt.Errorf("%s = %g, want %g ± %g", what, *got, want, tol)
	}
	return nil
}
