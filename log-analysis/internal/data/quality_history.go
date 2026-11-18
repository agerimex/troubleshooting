package data

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// Evaluation history (quality gate, stage 5; docs/quality-gate.md). An
// evaluation sent with record is stored in quality_evaluations (created by
// log-receiver's EnsureSchema) with the exact rules text and the evidence
// snapshots, under the rules' project. Stored evaluations are never changed:
// a later evaluation of the same commit is a new record. The history has no
// TTL; it grows by one row (a few kB to a few hundred kB with snapshots) per
// recorded evaluation.

// ErrQualityEvaluationNotFound is returned for an unknown evaluation id.
var ErrQualityEvaluationNotFound = errors.New("evaluation not found")

// Limits of history reads.
const (
	DefaultQualityHistory = 50
	MaxQualityHistory     = 200
	// CatalogueHistory is the number of evaluations a requirement's history
	// shows.
	CatalogueHistory = 30
)

// QualityStatusRef is the status of one check or requirement in a summary.
type QualityStatusRef struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	// Criteria is a requirement's criteria_sha256 (empty for checks and in
	// summaries recorded before criteria existed).
	Criteria string `json:"criteria,omitempty"`
}

// QualityEvaluationSummary is one row of the history: what was tested, the
// gate, and the status of every check and requirement.
type QualityEvaluationSummary struct {
	ID           string             `json:"id"`
	Project      string             `json:"project"`
	RecordedAt   string             `json:"recorded_at"`
	Run          string             `json:"run"`
	Gate         string             `json:"gate"`
	Source       QualitySource      `json:"source"`
	Checks       []QualityStatusRef `json:"checks"`
	Requirements []QualityStatusRef `json:"requirements"`
	// cursor is RecordedAt in Unix ns (the list's paging key).
	cursor int64
}

// qualityEvaluationID is the same for the same evaluation inputs (run,
// windows, rules, what was tested), so a retried request records once. A new
// CI attempt names a new CI run, so it is a new record; reevaluated (the
// evaluation time of a deliberate re-evaluation, else "") makes one too.
func qualityEvaluationID(req QualityRequest, rulesSHA, reevaluated string) string {
	h := sha256.New()
	src := QualitySource{}
	if req.Source != nil {
		src = *req.Source
	}
	parts := []string{req.Rules.Project, req.Run, req.TimeFrom, req.TimeTo, req.Baseline, req.BaselineFrom, req.BaselineTo,
		rulesSHA, src.Commit, src.PRHead, src.Ref, src.CIRun, src.RulesRef}
	if reevaluated != "" {
		// Only then: ids recorded before re-evaluations existed stay the same.
		parts = append(parts, reevaluated)
	}
	for _, v := range parts {
		h.Write([]byte(v))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:24]
}

var qualityIDPattern = regexp.MustCompile(`^[0-9a-f]{24}$`)

// ValidQualityEvaluationID says whether id has the form of an evaluation id.
func ValidQualityEvaluationID(id string) bool { return qualityIDPattern.MatchString(id) }

func summaryOf(res QualityResult) QualityEvaluationSummary {
	s := QualityEvaluationSummary{ID: res.ID, Project: res.Project, RecordedAt: res.RecordedAt, Run: res.Run, Gate: res.Gate,
		Checks: []QualityStatusRef{}, Requirements: []QualityStatusRef{}}
	if res.Source != nil {
		s.Source = *res.Source
	}
	for _, c := range res.Checks {
		s.Checks = append(s.Checks, QualityStatusRef{ID: c.ID, Status: c.Status})
	}
	for _, r := range res.Requirements {
		s.Requirements = append(s.Requirements, QualityStatusRef{r.ID, r.Status, r.Criteria})
	}
	return s
}

// qualityStore is where recorded evaluations live (replaceable in tests).
type qualityStore interface {
	// lookup returns the recorded body of an evaluation, found false if none.
	// With several rows under one id (recorded before inserts were
	// deduplicated) it is the earliest recorded one, as every read takes.
	lookup(ctx context.Context, project, id string) (body string, found bool, err error)
	// insert stores a row at most once per id: a second insert of the same id
	// is dropped by the store, also when two arrive at once.
	insert(ctx context.Context, id string, row []any) error
}

type clickhouseQualityStore struct{}

func (clickhouseQualityStore) lookup(ctx context.Context, project, id string) (string, bool, error) {
	var body string
	err := clickhouseDB.QueryRow(withLimits(ctx), `SELECT "Body" FROM quality_evaluations WHERE "Project" = {project:String} AND "Id" = {id:String} ORDER BY "RecordedAt", cityHash64("Body") LIMIT 1`,
		clickhouse.Named("project", project), clickhouse.Named("id", id)).Scan(&body)
	if errors.Is(err, errNoRows) {
		return "", false, nil
	}
	return body, err == nil, err
}

func (clickhouseQualityStore) insert(ctx context.Context, id string, row []any) error {
	return insertQualityEvaluation(ctx, id, row)
}

// recordLocks serializes the recording of one evaluation id in this process,
// so concurrent retries to one API instance insert once. Across instances
// the store's deduplication by id is the boundary (insertQualityEvaluation).
type recordLocks struct {
	mu    sync.Mutex
	locks map[string]*recordLock
}

type recordLock struct {
	sync.Mutex
	users int
}

func (l *recordLocks) lock(id string) (unlock func()) {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = map[string]*recordLock{}
	}
	k := l.locks[id]
	if k == nil {
		k = &recordLock{}
		l.locks[id] = k
	}
	k.users++
	l.mu.Unlock()
	k.Lock()
	return func() {
		k.Unlock()
		l.mu.Lock()
		if k.users--; k.users == 0 {
			delete(l.locks, id)
		}
		l.mu.Unlock()
	}
}

var qualityRecordLocks recordLocks

// RecordQuality stores an evaluation of a validated request with record. A
// failure is reported in res.RecordError: the result itself stays valid.
func RecordQuality(ctx context.Context, req QualityRequest, res *QualityResult) {
	recordQuality(ctx, clickhouseQualityStore{}, &qualityRecordLocks, req, res, clock.Now().UTC())
}

// recordQuality: an evaluation already recorded under the same id (a retried
// request) is answered with that record, exactly as recorded, never with the
// result just computed: one id has one verdict. A deliberate re-evaluation
// (reevaluate) gets an id of its own.
//
// Concurrent requests with one id are serialized in this process; between
// processes the store keeps the first insert of an id and drops the others,
// and every request answers with what the store kept after its own insert, so
// all of them answer with one record.
func recordQuality(ctx context.Context, store qualityStore, locks *recordLocks, req QualityRequest, res *QualityResult, now time.Time) {
	sum := sha256.Sum256([]byte(req.rulesText))
	sha := hex.EncodeToString(sum[:])
	reevaluated := ""
	if req.Reevaluate {
		reevaluated = num(now.UnixNano())
	}
	id := qualityEvaluationID(req, sha, reevaluated)
	defer locks.lock(id)()
	body, found, err := store.lookup(ctx, res.Project, id)
	if err != nil {
		res.RecordError = "the evaluation was not recorded: " + err.Error()
		return
	}
	if found {
		answerWithRecord(res, id, body)
		return
	}
	// A copy: res.Source is the request's, which recording must not change.
	src := QualitySource{}
	if res.Source != nil {
		src = *res.Source
	}
	src.RulesSHA256 = sha // of the text this server evaluated
	res.Source = &src
	res.ID, res.RecordedAt, res.Reevaluated = id, now.Format(time.RFC3339Nano), req.Reevaluate
	raw, _ := json.Marshal(res)
	summary, _ := json.Marshal(summaryOf(*res))
	// A batch sends the values as native columns: the rules text has newlines,
	// which a {name:String} query parameter can't carry.
	err = store.insert(ctx, id, []any{res.ID, res.Project, now.UnixNano(), res.Run, res.Source.Ref, res.Source.Commit,
		res.Gate, sha, req.rulesText, string(summary), string(raw)})
	if err != nil {
		res.ID, res.RecordedAt, res.Reevaluated, res.RecordError = "", "", false, "the evaluation was not recorded: "+err.Error()
		return
	}
	// Another instance may have recorded this id at the same time: the store
	// kept one of the inserts, and that one is the verdict.
	kept, found, err := store.lookup(ctx, res.Project, id)
	switch {
	case err != nil:
		res.RecordError = "the evaluation " + id + " was sent to the history, but reading it back failed (" + err.Error() + "): open it by its id for the recorded verdict"
	case !found:
		res.RecordError = "the evaluation " + id + " was sent to the history, but it is not there"
	case kept != string(raw):
		answerWithRecord(res, id, kept)
	}
}

// answerWithRecord replaces res with the record stored under id.
func answerWithRecord(res *QualityResult, id, body string) {
	var stored QualityResult
	if err := json.Unmarshal([]byte(body), &stored); err != nil {
		res.RecordError = "the evaluation " + id + " is recorded, but its record can't be read: " + err.Error()
		res.ID, res.RecordedAt, res.Reevaluated = "", "", false
		return
	}
	*res = stored
	res.RecordedBefore = true
}

// insertQualityEvaluation inserts with the id as deduplication token: the
// table remembers the tokens of its latest inserts
// (non_replicated_deduplication_window, set by the receiver) and drops an
// insert whose token it has, also one that arrives at the same time.
func insertQualityEvaluation(ctx context.Context, id string, row []any) error {
	ctx = clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
		"insert_deduplicate": 1, "insert_deduplication_token": "quality-evaluation-" + id}))
	batch, err := clickhouseDB.PrepareBatch(ctx, `INSERT INTO quality_evaluations (Id, Project, RecordedAt, Run, Ref, Commit, Gate, RulesSHA256, Rules, Summary, Body)`)
	if err != nil {
		return err
	}
	if err := batch.Append(row...); err != nil {
		_ = batch.Abort()
		return err
	}
	return batch.Send()
}

// QualityHistoryFilter selects recorded evaluations, newest first.
type QualityHistoryFilter struct {
	Project string
	Ref     string // exact branch or tag; empty = any
	Before  int64  // RecordedAt in Unix ns, exclusive; 0 = now
	Limit   int
}

// ParseQualityHistoryFilter reads the query parameters project, ref, before
// and limit.
func ParseQualityHistoryFilter(get func(string) string, needProject bool) (QualityHistoryFilter, error) {
	f := QualityHistoryFilter{Project: strings.TrimSpace(get("project")), Ref: strings.TrimSpace(get("ref")), Limit: DefaultQualityHistory}
	if f.Project == "" && needProject {
		return f, invalid("project is required")
	}
	if f.Project != "" && !projectPattern.MatchString(f.Project) {
		return f, invalid("project must be 1-128 letters, digits, '_', '.', '/' or '-'")
	}
	if len(f.Ref) > 512 || strings.ContainsAny(f.Ref, "\x00\n\r") {
		return f, invalid("ref must be one line of at most 512 characters")
	}
	if v := get("before"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return f, invalid("before must be Unix nanoseconds")
		}
		f.Before = n
	}
	if v := get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > MaxQualityHistory {
			return f, invalid("limit must be between 1 and %d", MaxQualityHistory)
		}
		f.Limit = n
	}
	return f, nil
}

// ListQualityEvaluations returns recorded evaluations, newest first, and the
// cursor for the next page ("" when there is none).
func ListQualityEvaluations(ctx context.Context, f QualityHistoryFilter) ([]QualityEvaluationSummary, string, error) {
	where := []string{"1"}
	params := []any{}
	if f.Project != "" {
		where = append(where, `"Project" = {project:String}`)
		params = append(params, clickhouse.Named("project", f.Project))
	}
	if f.Ref != "" {
		where = append(where, `"Ref" = {ref:String}`)
		params = append(params, clickhouse.Named("ref", f.Ref))
	}
	having := "1"
	if f.Before > 0 {
		having = `"At" < {before:Int64}`
		params = append(params, clickhouse.Named("before", num(f.Before)))
	}
	// One row per id, the earliest recorded (as lookup and LoadQualityEvaluation).
	rows, err := clickhouseDB.Query(withLimits(ctx), `SELECT min("RecordedAt") AS "At", argMin("Summary", ("RecordedAt", cityHash64("Body")))
		FROM quality_evaluations WHERE `+strings.Join(where, " AND ")+` GROUP BY "Id" HAVING `+having+`
		ORDER BY "At" DESC, "Id" DESC LIMIT `+strconv.Itoa(f.Limit+1), params...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []QualityEvaluationSummary{}
	for rows.Next() {
		var at int64
		var raw string
		if err := rows.Scan(&at, &raw); err != nil {
			return nil, "", err
		}
		var s QualityEvaluationSummary
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return nil, "", err
		}
		s.cursor = at
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > f.Limit {
		out = out[:f.Limit]
		next = num(out[len(out)-1].cursor)
	}
	return out, next, nil
}

// QualityRecord is one stored evaluation exactly as recorded, with its rules.
type QualityRecord struct {
	Evaluation json.RawMessage `json:"evaluation"`
	Rules      string          `json:"rules"`
}

// LoadQualityEvaluation returns a recorded evaluation.
func LoadQualityEvaluation(ctx context.Context, id string) (QualityRecord, error) {
	var body, rules string
	err := clickhouseDB.QueryRow(withLimits(ctx), `SELECT "Body", "Rules" FROM quality_evaluations WHERE "Id" = {id:String} ORDER BY "RecordedAt", cityHash64("Body") LIMIT 1`,
		clickhouse.Named("id", id)).Scan(&body, &rules)
	if errors.Is(err, errNoRows) {
		return QualityRecord{}, ErrQualityEvaluationNotFound
	}
	if err != nil {
		return QualityRecord{}, err
	}
	return QualityRecord{Evaluation: json.RawMessage(body), Rules: rules}, nil
}

// QualityProject is one project with recorded evaluations.
type QualityProject struct {
	Project     string `json:"project"`
	Evaluations int64  `json:"evaluations"`
	LastAt      string `json:"last_recorded_at"`
	LastGate    string `json:"last_gate"`
}

// ListQualityProjects returns the projects with recorded evaluations, the
// most recently recorded first.
func ListQualityProjects(ctx context.Context) ([]QualityProject, error) {
	rows, err := clickhouseDB.Query(withLimits(ctx), `SELECT "Project", uniqExact("Id"), max("RecordedAt"), argMax("Gate", "RecordedAt")
		FROM quality_evaluations GROUP BY "Project" ORDER BY max("RecordedAt") DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []QualityProject{}
	for rows.Next() {
		var p QualityProject
		var n uint64
		var at int64
		if err := rows.Scan(&p.Project, &n, &at, &p.LastGate); err != nil {
			return nil, err
		}
		p.Evaluations, p.LastAt = int64(n), time.Unix(0, at).UTC().Format(time.RFC3339Nano)
		out = append(out, p)
	}
	return out, rows.Err()
}

// QualityHistoryPoint is a requirement's (or check's) status in one recorded
// evaluation; Status "absent" when that evaluation's rules didn't have it.
type QualityHistoryPoint struct {
	EvaluationID string `json:"evaluation_id"`
	RecordedAt   string `json:"recorded_at"`
	Commit       string `json:"commit,omitempty"`
	Ref          string `json:"ref,omitempty"`
	RulesSHA256  string `json:"rules_sha256,omitempty"`
	// Criteria is the requirement's criteria_sha256 in that evaluation.
	Criteria string `json:"criteria_sha256,omitempty"`
	Status   string `json:"status"`
}

// QualityStreak is how long the current status has held under the current
// criteria: the oldest evaluation of the newest run of equal statuses with
// equal criteria, and how many in a row. A change of the requirement's
// criteria ends it (CriteriaChanged), even when the status stayed the same:
// two passes against different limits are not one streak.
type QualityStreak struct {
	Since           QualityHistoryPoint `json:"since"`
	Evaluations     int                 `json:"evaluations"`
	AtLeast         bool                `json:"at_least,omitempty"` // the streak reaches the end of the history read
	StatusBefore    string              `json:"status_before,omitempty"`
	CriteriaChanged bool                `json:"criteria_changed,omitempty"`
}

// QualityCatalogueEntry is one requirement as the latest evaluation states
// it, with its history.
type QualityCatalogueEntry struct {
	QualityRequirementResult
	Streak  QualityStreak         `json:"streak"`
	History []QualityHistoryPoint `json:"history"`
}

// QualityCatalogue is a project's requirements: the latest recorded
// evaluation's requirements and checks, each requirement with its history.
type QualityCatalogue struct {
	Project string `json:"project"`
	Ref     string `json:"ref,omitempty"`
	// Latest is nil when nothing was recorded for the project (and ref).
	Latest       *QualityEvaluationSummary `json:"latest"`
	Requirements []QualityCatalogueEntry   `json:"requirements"`
	Checks       []QualityCheckResult      `json:"checks"`
	// Unlinked are checks no requirement lists.
	Unlinked []string `json:"unlinked"`
}

// buildCatalogue combines the latest evaluation (summaries[0], whose full
// result is latest) with the statuses of the older ones.
func buildCatalogue(project, ref string, latest QualityResult, summaries []QualityEvaluationSummary) QualityCatalogue {
	cat := QualityCatalogue{Project: project, Ref: ref, Requirements: []QualityCatalogueEntry{}, Checks: latest.Checks, Unlinked: []string{}}
	if len(summaries) == 0 {
		cat.Checks = []QualityCheckResult{}
		return cat
	}
	cat.Latest = &summaries[0]
	for i := range cat.Checks {
		for j := range cat.Checks[i].Evidence {
			cat.Checks[i].Evidence[j].Snapshot = nil // the catalogue is a list: open the evaluation for snapshots
		}
	}
	linked := map[string]bool{}
	for _, r := range latest.Requirements {
		for _, id := range append(append([]string{}, r.Checks...), r.Guardrails...) {
			linked[id] = true
		}
		e := QualityCatalogueEntry{QualityRequirementResult: r, History: []QualityHistoryPoint{}}
		for _, s := range summaries {
			status, criteria := "absent", ""
			for _, x := range s.Requirements {
				if x.ID == r.ID {
					status, criteria = x.Status, x.Criteria
					break
				}
			}
			e.History = append(e.History, QualityHistoryPoint{EvaluationID: s.ID, RecordedAt: s.RecordedAt, Commit: s.Source.Commit,
				Ref: s.Source.Ref, RulesSHA256: s.Source.RulesSHA256, Criteria: criteria, Status: status})
		}
		e.Streak = streakOf(e.History)
		cat.Requirements = append(cat.Requirements, e)
	}
	for _, c := range latest.Checks {
		if !linked[c.ID] {
			cat.Unlinked = append(cat.Unlinked, c.ID)
		}
	}
	return cat
}

// SameCriteria says whether two history points of one requirement were
// evaluated against the same criteria: by their criteria hashes, or, for
// evaluations recorded before those existed, by the whole rules file's sha256
// (any edit of the rules then counts as a change). Unknown on either side:
// the same.
func SameCriteria(a, b QualityHistoryPoint) bool {
	if a.Criteria != "" && b.Criteria != "" {
		return a.Criteria == b.Criteria
	}
	if a.RulesSHA256 != "" && b.RulesSHA256 != "" {
		return a.RulesSHA256 == b.RulesSHA256
	}
	return true
}

// streakOf: history is newest first.
func streakOf(h []QualityHistoryPoint) QualityStreak {
	if len(h) == 0 {
		return QualityStreak{}
	}
	n := 1
	// Absent and not evaluated aren't decided by criteria: only their status counts.
	assessed := h[0].Status != "absent" && h[0].Status != RequirementNotEvaluated
	for n < len(h) && h[n].Status == h[0].Status && (!assessed || SameCriteria(h[n-1], h[n])) {
		n++
	}
	s := QualityStreak{Since: h[n-1], Evaluations: n}
	if n == len(h) {
		s.AtLeast = true
	} else {
		s.StatusBefore = h[n].Status
		s.CriteriaChanged = assessed && h[n].Status != "absent" && h[n].Status != RequirementNotEvaluated && !SameCriteria(h[n-1], h[n])
	}
	return s
}

// QualityRequirementCatalogue reads a project's catalogue (for one ref, or
// any).
func QualityRequirementCatalogue(ctx context.Context, project, ref string) (QualityCatalogue, error) {
	summaries, _, err := ListQualityEvaluations(ctx, QualityHistoryFilter{Project: project, Ref: ref, Limit: CatalogueHistory})
	if err != nil {
		return QualityCatalogue{}, err
	}
	if len(summaries) == 0 {
		return buildCatalogue(project, ref, QualityResult{}, nil), nil
	}
	rec, err := LoadQualityEvaluation(ctx, summaries[0].ID)
	if err != nil {
		return QualityCatalogue{}, err
	}
	var latest QualityResult
	if err := json.Unmarshal(rec.Evaluation, &latest); err != nil {
		return QualityCatalogue{}, err
	}
	return buildCatalogue(project, ref, latest, summaries), nil
}
