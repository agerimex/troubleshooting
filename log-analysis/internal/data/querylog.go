package data

import (
	"context"
	"log"
	"strconv"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// Resources of this API's own ClickHouse queries, read when spans are shown.
//
// Every query span records the query id it ran with (AttrDBQueryID). The
// figures ClickHouse sends during a query (profile events) leave out CPU time
// of threads that finish after the last packet, so the CPU time recorded on
// the span would be too low; system.query_log has the query's final figures.
// When spans are read, the query_log rows of their query ids are looked up in
// the ClickHouse this API reads (so this works for Troubleshooting's own
// queries, and for an application's queries only if they ran on the same
// server) and their figures replace the recorded ones, marked with
// AttrDBServerSource. A query whose row is gone (query_log has its own TTL)
// keeps the recorded memory and rows.
const (
	AttrDBQueryID      = "db.clickhouse.query_id"
	AttrDBServerResult = "db.server.result_rows"
	AttrDBServerSource = "db.server.source"
)

// maxQueryLogIDs bounds one lookup; a page or report has fewer query spans.
const maxQueryLogIDs = 2000

func tagValue(tags []map[string]string, key string) string {
	for _, t := range tags {
		if t["key"] == key {
			return t["value"]
		}
	}
	return ""
}

func setTag(s *Span, key, value string) {
	for _, t := range s.Tags {
		if t["key"] == key {
			t["value"] = value
			return
		}
	}
	s.Tags = append(s.Tags, map[string]string{"key": key, "value": value})
}

type queryLogFigures struct {
	cpu, readRows, readBytes, resultRows uint64
	memory                               int64
}

// addQueryLogResources fills the server figures of the query spans among
// spans (timeFrom/timeTo: the validated window, Unix nanoseconds as text). A
// failed lookup leaves the spans as recorded: it never fails a read.
func addQueryLogResources(ctx context.Context, timeFrom, timeTo string, spans []*Span) {
	bySpan := make(map[string][]*Span)
	var ids []string
	for _, s := range spans {
		id := tagValue(s.Tags, AttrDBQueryID)
		if id == "" {
			continue
		}
		if _, seen := bySpan[id]; !seen {
			if len(ids) == maxQueryLogIDs {
				break
			}
			ids = append(ids, id)
		}
		bySpan[id] = append(bySpan[id], s)
	}
	if len(ids) == 0 {
		return
	}
	figures, err := queryLogFiguresOf(ctx, timeFrom, timeTo, ids)
	if err != nil {
		log.Printf("query_log figures of %d queries: %v", len(ids), err)
		return
	}
	for id, f := range figures {
		for _, s := range bySpan[id] {
			setTag(s, AttrDBServerCPU, strconv.FormatUint(f.cpu, 10))
			setTag(s, AttrDBServerMemory, strconv.FormatInt(f.memory, 10))
			setTag(s, AttrDBServerReadRows, strconv.FormatUint(f.readRows, 10))
			setTag(s, AttrDBServerReadSize, strconv.FormatUint(f.readBytes, 10))
			setTag(s, AttrDBServerResult, strconv.FormatUint(f.resultRows, 10))
			setTag(s, AttrDBServerSource, "query_log")
		}
	}
}

// queryLogFiguresOf reads the finished queries among ids. query_log is
// ordered by date and time, so the window (widened by a day: a query ends
// after its span starts) bounds what is read.
func queryLogFiguresOf(ctx context.Context, timeFrom, timeTo string, ids []string) (_ map[string]queryLogFigures, err error) {
	query := `SELECT query_id,
	 sum(ProfileEvents['UserTimeMicroseconds'] + ProfileEvents['SystemTimeMicroseconds']),
	 toInt64(max(memory_usage)), sum(read_rows), sum(read_bytes), sum(result_rows)
	FROM system.query_log
	WHERE type = 'QueryFinish'
	 AND event_date >= toDate(fromUnixTimestamp64Nano(toInt64({timeFrom:Int64})))
	 AND event_date <= toDate(fromUnixTimestamp64Nano(toInt64({timeTo:Int64}))) + 1
	 AND query_id IN ` + arrayLiteral(ids) + `
	GROUP BY query_id`
	params := []any{clickhouse.Named("timeFrom", timeFrom), clickhouse.Named("timeTo", timeTo)}

	ctx, span := startQuerySpan(ctx, "SELECT query_log", query, params)
	defer func() { endQuerySpan(span, err) }()

	rows, err := clickhouseDB.Query(withLimits(ctx), query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]queryLogFigures, len(ids))
	for rows.Next() {
		var id string
		var f queryLogFigures
		if err := rows.Scan(&id, &f.cpu, &f.memory, &f.readRows, &f.readBytes, &f.resultRows); err != nil {
			return nil, err
		}
		out[id] = f
	}
	return out, rows.Err()
}
