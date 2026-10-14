package data

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// Detection rules decide which error groups, slow calls and release
// comparisons the problem workspace lists as problems. Every save is a new
// revision in detection_rules (no TTL); a save names the revision it edited,
// so a concurrent save is a conflict instead of a silent overwrite.

// RulesScope is the only scope so far: one set of rules per installation.
const RulesScope = "installation"

// ErrorRule opens an error problem from an error group.
type ErrorRule struct {
	Enabled bool `json:"enabled"`
	// MinAffectedCalls: distinct recorded calls of the group in the window.
	MinAffectedCalls int `json:"minAffectedCalls"`
	// Grouping is the grouping basis; only errors-v1 (operation + signature).
	Grouping string `json:"grouping"`
}

// SlowRule opens a slow-call problem per operation.
type SlowRule struct {
	Enabled    bool    `json:"enabled"`
	DurationMs float64 `json:"durationMs"` // calls slower than this
	MinCalls   int     `json:"minCalls"`
}

// RegressionRule opens a release regression from a generated comparison.
type RegressionRule struct {
	Enabled     bool    `json:"enabled"`
	P95Relative float64 `json:"p95RelativePct"`
	P95Absolute float64 `json:"p95AbsoluteMs"`
}

// DetectionRules is one revision. Revision 0 is the built-in default, never
// saved.
type DetectionRules struct {
	Scope       string         `json:"scope"`
	Revision    int64          `json:"revision"`
	SavedAt     string         `json:"savedAt,omitempty"`
	EffectiveAt string         `json:"effectiveAt,omitempty"`
	Errors      ErrorRule      `json:"errors"`
	Slow        SlowRule       `json:"slow"`
	Regressions RegressionRule `json:"regressions"`
}

// GroupingErrorsV1 is the errors-v1 grouping: operation and normalized error
// signature, kept across releases.
const GroupingErrorsV1 = "operation_signature"

// DefaultDetectionRules apply until rules are saved.
var DefaultDetectionRules = DetectionRules{
	Scope:       RulesScope,
	Errors:      ErrorRule{Enabled: true, MinAffectedCalls: 1, Grouping: GroupingErrorsV1},
	Slow:        SlowRule{Enabled: true, DurationMs: 500, MinCalls: 5},
	Regressions: RegressionRule{Enabled: true, P95Relative: 20, P95Absolute: 20},
}

// FieldError is a validation failure of one field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Validate checks the enabled templates; a disabled one keeps its stored
// values unchecked.
func (r DetectionRules) Validate() []FieldError {
	var out []FieldError
	if r.Errors.Enabled {
		if r.Errors.MinAffectedCalls < 1 || r.Errors.MinAffectedCalls > 1_000_000 {
			out = append(out, FieldError{"errors.minAffectedCalls", "Enter a whole number of calls from 1 to 1,000,000."})
		}
		if r.Errors.Grouping != GroupingErrorsV1 {
			out = append(out, FieldError{"errors.grouping", "Only grouping by operation and error signature is supported."})
		}
	}
	if r.Slow.Enabled {
		if !(r.Slow.DurationMs > 0) || r.Slow.DurationMs > 3_600_000 {
			out = append(out, FieldError{"slow.durationMs", "Enter a duration above 0 ms and at most one hour."})
		}
		if r.Slow.MinCalls < 1 || r.Slow.MinCalls > 1_000_000 {
			out = append(out, FieldError{"slow.minCalls", "Enter a whole number of calls from 1 to 1,000,000."})
		}
	}
	if r.Regressions.Enabled {
		if !(r.Regressions.P95Relative > 0) || r.Regressions.P95Relative > 10_000 {
			out = append(out, FieldError{"regressions.p95RelativePct", "Enter a percentage above 0."})
		}
		if !(r.Regressions.P95Absolute > 0) || r.Regressions.P95Absolute > 3_600_000 {
			out = append(out, FieldError{"regressions.p95AbsoluteMs", "Enter a duration above 0 ms."})
		}
	}
	return out
}

// LoadDetectionRules returns the latest saved revision, or the defaults.
func LoadDetectionRules(ctx context.Context) (DetectionRules, error) {
	var body string
	var revision, savedAt, effectiveAt int64
	err := clickhouseDB.QueryRow(withLimits(ctx), `SELECT "Body", "Revision", "SavedAt", "EffectiveAt" FROM detection_rules
		WHERE "Scope" = {scope:String} ORDER BY "Revision" DESC LIMIT 1`, clickhouse.Named("scope", RulesScope)).
		Scan(&body, &revision, &savedAt, &effectiveAt)
	if errors.Is(err, errNoRows) {
		return DefaultDetectionRules, nil
	}
	if err != nil {
		return DetectionRules{}, err
	}
	var r DetectionRules
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		return DetectionRules{}, err
	}
	r.Scope, r.Revision = RulesScope, revision
	r.SavedAt, r.EffectiveAt = strconv.FormatInt(savedAt, 10), strconv.FormatInt(effectiveAt, 10)
	return r, nil
}

// ErrRulesConflict means another save happened since the edited revision.
var ErrRulesConflict = errors.New("the rules were changed by another save")

// saveMu serializes saves of this API instance, so a read-check-insert can't
// interleave with another one here. (Several API instances would need a
// stronger guard; this installation runs one.)
var saveMu sync.Mutex

// SaveDetectionRules stores r as the revision after expected. It returns the
// stored revision, or ErrRulesConflict with the current one.
func SaveDetectionRules(ctx context.Context, r DetectionRules, expected int64) (DetectionRules, error) {
	saveMu.Lock()
	defer saveMu.Unlock()
	current, err := LoadDetectionRules(ctx)
	if err != nil {
		return DetectionRules{}, err
	}
	if current.Revision != expected {
		return current, ErrRulesConflict
	}
	now := time.Now().UnixNano()
	r.Scope, r.Revision = RulesScope, expected+1
	r.SavedAt, r.EffectiveAt = "", ""
	body, err := json.Marshal(r)
	if err != nil {
		return DetectionRules{}, err
	}
	err = clickhouseDB.Exec(ctx, `INSERT INTO detection_rules ("Scope", "Revision", "SavedAt", "EffectiveAt", "Body")
		VALUES ({scope:String}, {revision:Int64}, {at:Int64}, {at:Int64}, {body:String})`,
		clickhouse.Named("scope", RulesScope), clickhouse.Named("revision", strconv.FormatInt(r.Revision, 10)),
		clickhouse.Named("at", strconv.FormatInt(now, 10)), clickhouse.Named("body", string(body)))
	if err != nil {
		return DetectionRules{}, err
	}
	// Read it back: the answer is what is stored, not what was sent.
	return LoadDetectionRules(ctx)
}
