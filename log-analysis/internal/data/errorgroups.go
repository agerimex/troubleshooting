package data

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrorGroup is one recurring error within one source kind (errors-v1).
// Occurrences are events of that kind; AffectedTraces counts distinct
// recorded TraceIds, AffectedCalls distinct (TraceId, SpanId), Unlinked the
// occurrences without a TraceId. Counts of
// different groups or source kinds are not additive.
type ErrorGroup struct {
	Key            string `json:"key"` // stable across versions and windows
	SourceKind     string `json:"sourceKind"`
	Service        string `json:"service"`
	Operation      string `json:"operation"` // "" = not recorded
	Environment    string `json:"environment"`
	Method         string `json:"method"` // exception or message
	Fingerprint    string `json:"fingerprint"`
	ExceptionType  string `json:"exceptionType"`
	Frame          string `json:"frame"`
	Signature      string `json:"signature"`
	Occurrences    uint64 `json:"occurrences"`
	Unlinked       uint64 `json:"unlinked"`
	AffectedTraces uint64 `json:"affectedTraces"`
	// AffectedCalls are the distinct recorded (TraceId, SpanId) of the
	// occurrences; nil when part of the window was summarized before call
	// counts were recorded.
	AffectedCalls *uint64        `json:"affectedCalls"`
	FirstSeen     string         `json:"firstSeen"`
	LastSeen      string         `json:"lastSeen"`
	Versions      []VersionCount `json:"versions"`
}

// VersionCount is a group's occurrences recorded with one version ("" = not
// recorded).
type VersionCount struct {
	Version     string `json:"version"`
	Occurrences uint64 `json:"occurrences"`
}

// SourceResult is the status of one evidence source of error groups: spans
// (ERROR status and exception events) or logs. A failed source is
// unavailable, never zero.
type SourceResult struct {
	Status   string             `json:"status"`
	Error    string             `json:"error,omitempty"`
	Coverage CoverageAssessment `json:"coverage"`
}

// ErrorGroups answers an error-groups request.
type ErrorGroups struct {
	Population  Population              `json:"population"`
	Label       string                  `json:"label"`
	Status      string                  `json:"status"`
	Condition   string                  `json:"condition"`
	Sources     map[string]SourceResult `json:"sources"` // spans, logs
	Groups      []ErrorGroup            `json:"groups"`
	Plan        []PlanPart              `json:"plan"`
	Notices     []Notice                `json:"notices"`
	Cutoff      string                  `json:"cutoff"`
	GeneratedAt string                  `json:"generatedAt"`
	Definitions map[string]string       `json:"definitions"`
}

func errorDefinitions() map[string]string {
	return map[string]string{
		"errors":     ErrorsContract,
		"canonical":  CanonicalContract,
		"grouping":   "exception: recorded exception.type + application frame (code.function.name, code.namespace.code.function, or the first stack frame without arguments, line numbers and offsets); message: the message with UUIDs, long hex ids and request/session/user/order/trace/span/correlation identifiers replaced; numbers such as status codes are kept",
		"timeBasis":  "UTC; logs and exception events by their own time, ERROR-status spans by start time",
		"sourceKind": "span_status, exception and log occurrences can describe the same failure; their counts are never added",
	}
}

// ErrorSourceSpans and ErrorSourceLogs are the sources of error groups.
const (
	ErrorSourceSpans = "spans"
	ErrorSourceLogs  = "logs"
)

func errorsUnion(plan []PlanPart, cutoff int64, kinds []string, raw func([]interval) string) string {
	var branches []string
	kindList := make([]string, len(kinds))
	for i, k := range kinds {
		kindList[i] = quoteString(k)
	}
	if cond := storedRevisions(plan); cond != "" {
		branches = append(branches, `SELECT `+errorColumns+` FROM stat_errors WHERE `+cond+` AND "SourceKind" IN (`+strings.Join(kindList, ", ")+`)`)
	}
	if parts := partsBy(plan, SourceRaw); len(parts) > 0 {
		branches = append(branches, `SELECT `+errorColumns+` FROM (`+groupedErrors(raw(parts))+`)`)
	}
	return strings.Join(branches, " UNION ALL ")
}

type groupRow struct {
	ErrorGroup
	version string
	first   int64
	last    int64
}

func selectErrorGroups(ctx context.Context, pop Population, union string) (_ []groupRow, err error) {
	if union == "" {
		return nil, nil
	}
	conds, params := pop.errorConditions("")
	query := `SELECT "SourceKind", "ServiceName", "Operation", "Environment", "Method", "Fingerprint", "Version",
		any("ExceptionType"), any("Frame"), any("Signature"),
		sum("Occurrences"), sum("Unlinked"), uniqExactIfMerge("Traces"), min("FirstSeen"), max("LastSeen")
	FROM (` + union + `)` + whereClause(conds) + `
	GROUP BY "SourceKind", "ServiceName", "Operation", "Environment", "Method", "Fingerprint", "Version"`
	ctx, span := startQuerySpan(ctx, "SELECT error groups", query, params)
	defer func() { endQuerySpan(span, err) }()
	rows, err := clickhouseDB.Query(withLimits(ctx), query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []groupRow
	for rows.Next() {
		var g groupRow
		if err := rows.Scan(&g.SourceKind, &g.Service, &g.Operation, &g.Environment, &g.Method, &g.Fingerprint, &g.version,
			&g.ExceptionType, &g.Frame, &g.Signature, &g.Occurrences, &g.Unlinked, &g.AffectedTraces, &g.first, &g.last); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// distinctCounts are a group's distinct traces and calls over all versions.
type distinctCounts struct {
	traces uint64
	calls  *uint64
}

// selectAffectedTraces counts distinct traces and calls per group over all
// versions (distinct counts of versions can't be added).
func selectAffectedTraces(ctx context.Context, pop Population, union string) (_ map[string]distinctCounts, err error) {
	if union == "" {
		return map[string]distinctCounts{}, nil
	}
	conds, params := pop.errorConditions("")
	query := `SELECT "SourceKind", "ServiceName", "Operation", "Environment", "Fingerprint", uniqExactIfMerge("Traces"),
		uniqExactIfMerge("Calls"), min("HasCalls")
	FROM (` + union + `)` + whereClause(conds) + `
	GROUP BY "SourceKind", "ServiceName", "Operation", "Environment", "Fingerprint"`
	ctx, span := startQuerySpan(ctx, "SELECT error group traces", query, params)
	defer func() { endQuerySpan(span, err) }()
	rows, err := clickhouseDB.Query(withLimits(ctx), query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]distinctCounts{}
	for rows.Next() {
		var g ErrorGroup
		var n, calls uint64
		var hasCalls uint8
		if err := rows.Scan(&g.SourceKind, &g.Service, &g.Operation, &g.Environment, &g.Fingerprint, &n, &calls, &hasCalls); err != nil {
			return nil, err
		}
		c := distinctCounts{traces: n}
		if hasCalls == 1 {
			c.calls = &calls
		}
		out[groupKey(g)] = c
	}
	return out, rows.Err()
}

// groupKey identifies a group across versions and windows.
func groupKey(g ErrorGroup) string {
	return strings.Join([]string{g.SourceKind, g.Service, g.Operation, g.Environment, g.Fingerprint}, "|")
}

// mergeGroupRows folds per-version rows into groups.
func mergeGroupRows(rows []groupRow, traces map[string]distinctCounts) []ErrorGroup {
	byKey := map[string]*ErrorGroup{}
	first := map[string]int64{}
	last := map[string]int64{}
	var order []string
	for _, r := range rows {
		key := groupKey(r.ErrorGroup)
		g, ok := byKey[key]
		if !ok {
			copy := r.ErrorGroup
			copy.Key = key
			copy.Occurrences, copy.Unlinked = 0, 0
			copy.Versions = []VersionCount{}
			g = &copy
			byKey[key] = g
			order = append(order, key)
			first[key], last[key] = r.first, r.last
		}
		g.Occurrences += r.Occurrences
		g.Unlinked += r.Unlinked
		g.Versions = append(g.Versions, VersionCount{Version: r.version, Occurrences: r.Occurrences})
		first[key] = min(first[key], r.first)
		last[key] = max(last[key], r.last)
	}
	out := make([]ErrorGroup, 0, len(order))
	for _, key := range order {
		g := byKey[key]
		g.AffectedTraces = traces[key].traces
		g.AffectedCalls = traces[key].calls
		g.FirstSeen = strconv.FormatInt(first[key], 10)
		g.LastSeen = strconv.FormatInt(last[key], 10)
		sort.Slice(g.Versions, func(i, j int) bool { return g.Versions[i].Version < g.Versions[j].Version })
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Occurrences != out[j].Occurrences {
			return out[i].Occurrences > out[j].Occurrences
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// ComputeErrorGroups groups the population's error evidence. Spans (ERROR
// status and exception events) and logs are queried separately: one failing
// source is unavailable while the other's groups remain.
func ComputeErrorGroups(ctx context.Context, pop Population) ErrorGroups {
	now := clock.Now()
	cutoff := time.Now().UnixNano()
	pol := policyFor(ctx)
	res := ErrorGroups{
		Population: pop, Label: pop.Label(), Groups: []ErrorGroup{}, Notices: []Notice{}, Plan: []PlanPart{},
		Sources: map[string]SourceResult{}, Cutoff: strconv.FormatInt(cutoff, 10), GeneratedAt: nanos(now),
		Definitions: errorDefinitions(),
	}
	revisions, err := loadRevisions(ctx, pop.from(), pop.to())
	if err != nil {
		res.Status, _ = statsStatus(ctx, err)
		res.Condition = conditionOf(res.Status)
		for _, s := range []string{ErrorSourceSpans, ErrorSourceLogs} {
			st, msg := statsStatus(ctx, err)
			res.Sources[s] = SourceResult{Status: st, Error: msg, Coverage: CoverageAssessment{Status: CoverageUnknown, Reasons: []string{}}}
		}
		return res
	}
	rawFrom := max(rawHorizon(now, pol.TracesSeconds), rawHorizon(now, pol.LogsSeconds))
	plan := planWindow(pop.from(), pop.to(), revisions, rawFrom, statsHorizon(now, pol))
	res.Plan = plan
	res.Notices = append(res.Notices, planNotices(plan)...)
	for _, p := range plan {
		if p.Source == SourceUnavailable {
			res.Status = StatsOutsideRetention
			res.Condition = "unavailable: part of the window is outside retained coverage (" + p.Reason + ")"
			for _, s := range []string{ErrorSourceSpans, ErrorSourceLogs} {
				res.Sources[s] = SourceResult{Status: StatsOutsideRetention, Error: p.Reason, Coverage: CoverageAssessment{Status: CoverageUnknown, Reasons: []string{}}}
			}
			return res
		}
	}

	ev, covErr := loadCoverage(ctx, pop.from(), pop.to())
	lateness := statsConfig.AllowedLateness
	type sourceQuery struct {
		name, coverage string
		kinds          []string
		raw            func([]interval) string
	}
	sources := []sourceQuery{
		{ErrorSourceSpans, "traces", []string{SourceSpanStatus, SourceException}, func(parts []interval) string {
			return statusOccurrences(parts, cutoff, lateness, `"ServiceName" = {service:String}`) + ` UNION ALL ` +
				`SELECT "SourceKind", "EventTime", "TraceId", "SpanId", "ServiceName", "Operation", "Environment", "Version", "Installation", "Segment", "ExceptionType", "Frame", "Message" FROM (` +
				exceptionOccurrences(parts, cutoff, lateness, `"ServiceName" = {service:String}`) + `)`
		}},
		{ErrorSourceLogs, "logs", []string{SourceLog}, func(parts []interval) string {
			return `SELECT "SourceKind", "EventTime", "TraceId", "SpanId", "ServiceName", "Operation", "Environment", "Version", "Installation", "Segment", "ExceptionType", "Frame", "Message" FROM (` +
				logOccurrences(parts, cutoff, lateness, `"ServiceName" = {service:String}`) + `)`
		}},
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var rows []groupRow
	traces := map[string]distinctCounts{}
	for _, src := range sources {
		wg.Add(1)
		go func(src sourceQuery) {
			defer wg.Done()
			result := SourceResult{Status: StatsOK, Coverage: CoverageAssessment{Status: CoverageUnknown, Reasons: []string{}}}
			if covErr == nil {
				var parts []CoverageAssessment
				for _, p := range plan {
					parts = append(parts, ev.assess(p.from, p.to, src.coverage, now))
				}
				result.Coverage = combineCoverage(parts)
			}
			union := errorsUnion(plan, cutoff, src.kinds, src.raw)
			r, err := selectErrorGroups(ctx, pop, union)
			var t map[string]distinctCounts
			if err == nil {
				t, err = selectAffectedTraces(ctx, pop, union)
			}
			if err != nil {
				result.Status, result.Error = statsStatus(ctx, err)
			}
			mu.Lock()
			defer mu.Unlock()
			res.Sources[src.name] = result
			if err == nil {
				rows = append(rows, r...)
				for k, v := range t {
					traces[k] = v
				}
			}
		}(src)
	}
	wg.Wait()
	if covErr != nil {
		res.Notices = append(res.Notices, Notice{Code: "coverage", Message: "collection coverage could not be read: " + covErr.Error()})
	}
	res.Groups = mergeGroupRows(rows, traces)
	res.Status, res.Condition = StatsOK, "ok"
	for _, name := range []string{ErrorSourceSpans, ErrorSourceLogs} {
		if s := res.Sources[name]; s.Status != StatsOK {
			res.Notices = append(res.Notices, Notice{Code: "source_unavailable", Message: "Error evidence from " + name +
				" is unavailable (" + s.Error + "): its groups are missing, not zero."})
			res.Condition = "partial: " + name + " unavailable"
		}
	}
	if len(res.Groups) == 0 && res.Condition == "ok" {
		res.Condition = "no recorded error evidence in this scope"
	}
	return res
}
