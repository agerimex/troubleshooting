package data

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// CompareRequest is the body of compare: two explicit populations. Which one
// is the baseline is the user's choice; version text has no order.
type CompareRequest struct {
	Baseline  Population `json:"baseline"`
	Candidate Population `json:"candidate"`
	Title     string     `json:"title"`
	// Regenerates names the analysis this one recomputes; both stay
	// available.
	Regenerates string `json:"regenerates"`
}

// Validate checks both populations.
func (r *CompareRequest) Validate() error {
	if err := r.Baseline.Validate(); err != nil {
		return fmt.Errorf("baseline: %w", err)
	}
	if err := r.Candidate.Validate(); err != nil {
		return fmt.Errorf("candidate: %w", err)
	}
	if r.Regenerates != "" && !analysisIDPattern.MatchString(r.Regenerates) {
		return invalid("regenerates must be an analysis id")
	}
	return nil
}

// ComparisonRow compares one (operation, span kind) of both populations.
type ComparisonRow struct {
	Operation string             `json:"operation"`
	SpanKind  string             `json:"spanKind"`
	Baseline  *CallStats         `json:"baseline"`  // nil: no calls observed (or unavailable, see Analysis)
	Candidate *CallStats         `json:"candidate"` // nil: no calls observed
	Metrics   []MetricComparison `json:"metrics"`
	Notes     []string           `json:"notes"`
}

// Analysis is a generated comparison, stored exactly as shown. An export
// reproduces it; regenerating creates a new analysis with its own id.
type Analysis struct {
	Id              string            `json:"id"`
	Kind            string            `json:"kind"`
	Title           string            `json:"title"`
	Regenerates     string            `json:"regenerates,omitempty"`
	GeneratedAt     string            `json:"generatedAt"`
	Contracts       map[string]string `json:"contracts"`
	Rules           Rules             `json:"rules"`
	Baseline        PopulationStats   `json:"baseline"`
	Candidate       PopulationStats   `json:"candidate"`
	BaselineErrors  ErrorGroups       `json:"baselineErrors"`
	CandidateErrors ErrorGroups       `json:"candidateErrors"`
	Mismatches      []Mismatch        `json:"mismatches"`
	Rows            []ComparisonRow   `json:"rows"`
	ErrorGroups     []GroupComparison `json:"errorGroups"`
	Notices         []Notice          `json:"notices"`
	Interpretation  string            `json:"interpretation"`
}

const interpretation = "Statistics describe recorded spans and logs of the selected populations, not all traffic or customer outcomes. " +
	"Change signals follow the recorded comparison rules; they are heuristics that point to what to investigate, not proof that a deployment caused a change, " +
	"and results within thresholds are not proof of identical behavior. OK span status is technical status, not business success."

// Compare generates and stores a comparison.
func Compare(ctx context.Context, req CompareRequest) (Analysis, error) {
	a := Analysis{
		Id:          newAnalysisID(),
		Kind:        "comparison",
		Title:       req.Title,
		Regenerates: req.Regenerates,
		GeneratedAt: nanos(clock.Now()),
		Contracts: map[string]string{"metrics": MetricsContract, "errors": ErrorsContract, "comparison": ComparisonContract,
			"canonical": CanonicalContract, "percentile": PercentileMethod},
		Rules:          DefaultRules,
		Interpretation: interpretation,
		Notices:        []Notice{},
	}
	if a.Title == "" {
		a.Title = req.Candidate.Label() + " against " + req.Baseline.Label()
	}

	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		a.Baseline = ComputeStats(ctx, StatsRequest{Population: req.Baseline, Series: true})
	}()
	go func() {
		defer wg.Done()
		a.Candidate = ComputeStats(ctx, StatsRequest{Population: req.Candidate, Series: true})
	}()
	go func() { defer wg.Done(); a.BaselineErrors = ComputeErrorGroups(ctx, req.Baseline) }()
	go func() { defer wg.Done(); a.CandidateErrors = ComputeErrorGroups(ctx, req.Candidate) }()
	wg.Wait()

	bDefs := a.Baseline.Definitions
	cDefs := a.Candidate.Definitions
	bDefs["errors"], cDefs["errors"] = a.BaselineErrors.Definitions["errors"], a.CandidateErrors.Definitions["errors"]
	a.Mismatches = populationMismatches(req.Baseline, req.Candidate, samplingOf(a.Baseline.Rows), samplingOf(a.Candidate.Rows), bDefs, cDefs)
	a.Mismatches = append(a.Mismatches, revisionMismatches(a.Baseline.Plan, a.Candidate.Plan)...)
	if a.Mismatches == nil {
		a.Mismatches = []Mismatch{}
	}
	a.Rows = comparisonRows(req, a.Baseline, a.Candidate, a.Rules, mismatchedFor(a.Mismatches, "metrics"))
	a.ErrorGroups = compareGroups(a.BaselineErrors, a.CandidateErrors, mismatchedFor(a.Mismatches, "error_groups"))
	for _, m := range a.Mismatches {
		a.Notices = append(a.Notices, Notice{Code: "mismatch", Message: fmt.Sprintf(
			"The populations differ in %s (baseline %s, candidate %s): conclusions for %v are unavailable until this is reconciled. The statistics remain shown.",
			m.Dimension, m.Baseline, m.Candidate, m.Affects)})
	}
	if a.Baseline.DurationSeconds != a.Candidate.DurationSeconds {
		a.Notices = append(a.Notices, Notice{Code: "window_length", Message: fmt.Sprintf(
			"The windows have different lengths (%s and %s): compare rates rather than raw call counts.",
			time.Duration(a.Baseline.DurationSeconds*float64(time.Second)), time.Duration(a.Candidate.DurationSeconds*float64(time.Second)))})
	}
	if err := storeAnalysis(ctx, a); err != nil {
		return a, err
	}
	return a, nil
}

// revisionMismatches reports buckets computed under different metric
// definitions in the two populations.
func revisionMismatches(b, c []PlanPart) []Mismatch {
	defs := func(plan []PlanPart) string {
		seen := map[string]bool{}
		for _, p := range plan {
			if p.Source == SourceStored {
				seen[p.Definitions] = true
			}
			if p.Source == SourceRaw {
				seen[Definitions] = true
			}
		}
		keys := sortedKeys(seen)
		if len(keys) == 1 {
			return keys[0]
		}
		return fmt.Sprint(keys)
	}
	bd, cd := defs(b), defs(c)
	if bd != cd && bd != "[]" && cd != "[]" {
		return []Mismatch{{Dimension: "statistics definition versions", Baseline: bd, Candidate: cd, Affects: []string{"metrics", "error_groups"}}}
	}
	return nil
}

func comparisonRows(req CompareRequest, b, c PopulationStats, rules Rules, mismatched []string) []ComparisonRow {
	side := func(s PopulationStats, row *CallStats) Side {
		sd := Side{Stats: row, Available: s.Status == StatsOK, Reason: s.Condition, Coverage: s.Coverage.Status, Sampling: samplingOf(s.Rows)}
		if row != nil {
			sd.Sampling = samplingOf([]CallStats{*row})
		}
		return sd
	}
	window := [2]float64{b.DurationSeconds, c.DurationSeconds}
	type key struct{ op, kind string }
	pairs := map[key]*ComparisonRow{}
	var order []key
	get := func(k key) *ComparisonRow {
		if r, ok := pairs[k]; ok {
			return r
		}
		r := &ComparisonRow{Operation: k.op, SpanKind: k.kind}
		pairs[k] = r
		order = append(order, k)
		return r
	}
	explicit := func(p Population) bool { return p.Operation != nil && p.SpanKind != nil }
	if explicit(req.Baseline) && explicit(req.Candidate) {
		// One selected operation on each side: pair them even if their
		// names differ (the mismatch is reported).
		r := get(key{*req.Candidate.Operation, *req.Candidate.SpanKind})
		if len(b.Rows) > 0 && b.Rows[0].Calls > 0 {
			r.Baseline = &b.Rows[0]
		}
		if len(c.Rows) > 0 && c.Rows[0].Calls > 0 {
			r.Candidate = &c.Rows[0]
		}
	} else {
		for i := range b.Rows {
			get(key{b.Rows[i].Operation, b.Rows[i].SpanKind}).Baseline = &b.Rows[i]
		}
		for i := range c.Rows {
			get(key{c.Rows[i].Operation, c.Rows[i].SpanKind}).Candidate = &c.Rows[i]
		}
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].op != order[j].op {
			return order[i].op < order[j].op
		}
		return order[i].kind < order[j].kind
	})
	rows := make([]ComparisonRow, 0, len(order))
	for _, k := range order {
		r := pairs[k]
		r.Metrics = compareMetrics(side(b, r.Baseline), side(c, r.Candidate), window, rules, mismatched)
		r.Notes = []string{}
		if c.Status == StatsOK && (r.Candidate == nil || r.Candidate.Calls == 0) {
			r.Notes = append(r.Notes, "No candidate calls were observed for this operation in the candidate scope. This does not mean it was fixed, became faster or was removed from the application.")
		}
		if b.Status == StatsOK && (r.Baseline == nil || r.Baseline.Calls == 0) {
			r.Notes = append(r.Notes, "No baseline calls were observed for this operation in the baseline scope.")
		}
		rows = append(rows, *r)
	}
	return rows
}

func newAnalysisID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}

// ErrAnalysisNotFound is returned for an unknown or expired analysis id.
var ErrAnalysisNotFound = errors.New("analysis not found (unknown id, or expired with the statistics retention)")

func storeAnalysis(ctx context.Context, a Analysis) error {
	body, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return clickhouseDB.Exec(ctx, `INSERT INTO analyses ("Id", "Kind", "CreatedAt", "Title", "Body") VALUES ({id:String}, {kind:String}, {at:Int64}, {title:String}, {body:String})`,
		clickhouse.Named("id", a.Id), clickhouse.Named("kind", a.Kind),
		clickhouse.Named("at", strconv.FormatInt(time.Now().UnixNano(), 10)),
		clickhouse.Named("title", a.Title), clickhouse.Named("body", string(body)))
}

// LoadAnalysis returns a stored analysis exactly as generated.
func LoadAnalysis(ctx context.Context, id string) (json.RawMessage, error) {
	var body string
	err := clickhouseDB.QueryRow(withLimits(ctx), `SELECT "Body" FROM analyses WHERE "Id" = {id:String} LIMIT 1`, clickhouse.Named("id", id)).Scan(&body)
	if err != nil {
		if errors.Is(err, errNoRows) {
			return nil, ErrAnalysisNotFound
		}
		return nil, err
	}
	return json.RawMessage(body), nil
}

// AnalysisSummary is one row of the analysis list.
type AnalysisSummary struct {
	Id        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
}

// ListAnalyses returns the most recent analyses.
func ListAnalyses(ctx context.Context, limit int) ([]AnalysisSummary, error) {
	rows, err := clickhouseDB.Query(withLimits(ctx), `SELECT "Id", "Kind", "Title", toString("CreatedAt") FROM analyses ORDER BY "CreatedAt" DESC LIMIT `+strconv.Itoa(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AnalysisSummary{}
	for rows.Next() {
		var s AnalysisSummary
		if err := rows.Scan(&s.Id, &s.Kind, &s.Title, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

var analysisIDPattern = regexp.MustCompile(`^[0-9a-f]{8,32}$`)

// ValidAnalysisID says whether id has the form of an analysis id.
func ValidAnalysisID(id string) bool { return analysisIDPattern.MatchString(id) }

var errNoRows = sql.ErrNoRows
