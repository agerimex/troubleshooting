package data

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A statistics query is answered per one-hour bucket from exactly one
// representation, so an interval never counts twice (RET-007):
//   - a whole bucket with a committed revision uses that revision;
//   - otherwise its part of the window is computed from canonical raw
//     records, if they are still retained;
//   - otherwise it is unavailable, never zero.
// A partial bucket can only come from raw records: retained statistics have
// whole-bucket resolution and are never prorated (RET-009).

// Plan part sources.
const (
	SourceStored      = "stored"
	SourceRaw         = "raw"
	SourceUnavailable = "unavailable"
)

// Revision is the committed revision of one bucket.
type Revision struct {
	BucketStart   int64  `json:"bucketStart"` // Unix ns
	Revision      int64  `json:"revision"`
	State         string `json:"state"` // provisional or final
	Definitions   string `json:"definitions"`
	GeneratedAt   int64  `json:"generatedAt"`
	ReceiptCutoff int64  `json:"receiptCutoff"`
	ContentHash   uint64 `json:"-"`
	Calls         uint64 `json:"calls"`
	ErrorEvents   uint64 `json:"errorEvents"`
}

// Revision states.
const (
	StateProvisional = "provisional"
	StateFinal       = "final"
)

// PlanPart is one interval of a statistics window and where its values come
// from.
type PlanPart struct {
	From          string `json:"from"`
	To            string `json:"to"`
	Source        string `json:"source"`
	WholeBucket   bool   `json:"wholeBucket"`
	Revision      int64  `json:"revision,omitempty"`
	State         string `json:"state,omitempty"`
	Definitions   string `json:"definitions,omitempty"`
	GeneratedAt   string `json:"generatedAt,omitempty"`
	ReceiptCutoff string `json:"receiptCutoff,omitempty"`
	Reason        string `json:"reason,omitempty"`

	from, to int64
}

// loadRevisions returns the committed revision of each bucket starting in
// [from, to): a final revision wins over provisional ones, then the newest.
func loadRevisions(ctx context.Context, from, to int64) (map[int64]Revision, error) {
	by := `tuple("State" = 'final', "Revision", "GeneratedAt")`
	query := `SELECT toInt64(toUnixTimestamp("BucketStart")), argMax("Revision", ` + by + `), argMax("State", ` + by + `),
		argMax("Definitions", ` + by + `), argMax("GeneratedAt", ` + by + `), argMax("ReceiptCutoff", ` + by + `),
		argMax("ContentHash", ` + by + `), argMax("Calls", ` + by + `), argMax("ErrorEvents", ` + by + `)
	FROM stat_revisions
	WHERE "BucketStart" >= toDateTime(` + num(floorHour(from)/1e9) + `, 'UTC') AND "BucketStart" < toDateTime(` + num(ceilHour(to)/1e9) + `, 'UTC')
	GROUP BY "BucketStart"`
	rows, err := clickhouseDB.Query(withLimits(ctx), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	revisions := map[int64]Revision{}
	for rows.Next() {
		var r Revision
		var start int64
		if err := rows.Scan(&start, &r.Revision, &r.State, &r.Definitions, &r.GeneratedAt, &r.ReceiptCutoff,
			&r.ContentHash, &r.Calls, &r.ErrorEvents); err != nil {
			return nil, err
		}
		r.BucketStart = start * 1e9
		revisions[r.BucketStart] = r
	}
	return revisions, rows.Err()
}

func floorHour(ns int64) int64 { return ns - ((ns%hourNs)+hourNs)%hourNs }
func ceilHour(ns int64) int64 {
	if f := floorHour(ns); f != ns {
		return f + hourNs
	}
	return ns
}

// planWindow splits [from, to) by bucket. rawFrom is the oldest event time
// whose raw records are retained for the sources needed; statsFrom the
// oldest retained bucket.
func planWindow(from, to int64, revisions map[int64]Revision, rawFrom, statsFrom int64) []PlanPart {
	var parts []PlanPart
	for start := floorHour(from); start < to; start += hourNs {
		p := PlanPart{from: max(from, start), to: min(to, start+hourNs)}
		p.WholeBucket = p.from == start && p.to == start+hourNs
		rev, stored := revisions[start]
		switch {
		case p.WholeBucket && stored && start >= statsFrom:
			p.Source = SourceStored
			p.Revision, p.State, p.Definitions = rev.Revision, rev.State, rev.Definitions
			p.GeneratedAt = strconv.FormatInt(rev.GeneratedAt, 10)
			p.ReceiptCutoff = strconv.FormatInt(rev.ReceiptCutoff, 10)
		case p.from >= rawFrom:
			p.Source = SourceRaw
		case start < statsFrom:
			p.Source = SourceUnavailable
			p.Reason = "outside retained coverage: both the raw records and the statistics of this interval have expired"
		case !p.WholeBucket && stored:
			p.Source = SourceUnavailable
			p.Reason = "exact bounds unavailable: only whole-hour statistics remain for this interval (its raw records have expired)"
		default:
			p.Source = SourceUnavailable
			p.Reason = "outside retained coverage: the raw records of this interval have expired and no statistics were retained for it"
		}
		p.From, p.To = strconv.FormatInt(p.from, 10), strconv.FormatInt(p.to, 10)
		parts = append(parts, p)
	}
	return parts
}

// wholeBucketAlternative returns the whole-bucket bounds around a window
// whose partial buckets can't be computed exactly, when every bucket in them
// has a retained revision; ok is false otherwise.
func wholeBucketAlternative(from, to int64, revisions map[int64]Revision, statsFrom int64) (int64, int64, bool) {
	start, end := floorHour(from), ceilHour(to)
	for b := start; b < end; b += hourNs {
		if _, ok := revisions[b]; !ok || b < statsFrom {
			return 0, 0, false
		}
	}
	return start, end, true
}

func partsBy(plan []PlanPart, source string) []interval {
	var out []interval
	for _, p := range plan {
		if p.Source == source {
			out = append(out, interval{p.from, p.to})
		}
	}
	return out
}

// storedRevisions is a ClickHouse condition selecting the rows of the
// planned revisions of a stat table.
func storedRevisions(plan []PlanPart) string {
	var tuples []string
	for _, p := range plan {
		if p.Source == SourceStored {
			tuples = append(tuples, `(`+num(p.from/1e9)+`, `+num(p.Revision)+`)`)
		}
	}
	if len(tuples) == 0 {
		return ""
	}
	return `(toInt64(toUnixTimestamp("BucketStart")), "Revision") IN (` + strings.Join(tuples, ", ") + `)`
}

// planNotices explains the plan to people.
func planNotices(plan []PlanPart) []Notice {
	var notices []Notice
	reasons := map[string]bool{}
	provisional := 0
	for _, p := range plan {
		if p.Source == SourceUnavailable && !reasons[p.Reason] {
			reasons[p.Reason] = true
			notices = append(notices, Notice{Code: "coverage", Message: p.Reason})
		}
		if p.Source == SourceStored && p.State == StateProvisional {
			provisional++
		}
	}
	if provisional > 0 {
		notices = append(notices, Notice{Code: "provisional", Message: strconv.Itoa(provisional) +
			" bucket(s) come from provisional statistics: records may still arrive within the allowed lateness and a later revision can change them."})
	}
	return notices
}

// Notice is a limitation or qualification of a statistic.
type Notice struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func nanos(t time.Time) string { return strconv.FormatInt(t.UnixNano(), 10) }
