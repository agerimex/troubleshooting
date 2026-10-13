package data

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	pb "github.com/agerimex/troubleshooting/protos/logs"
)

// Log groups the inserts into ClickHouse.
type Log struct{}

// spanColumns lists the inserted columns in the order Append receives them.
// The names are unquoted: the driver matches them to the table's columns and
// only strips backticks.
const spanColumns = "UnixTime, TraceId, SpanId, ParentSpanId, TraceState, SpanName, SpanKind, ServiceName, " +
	"ResourceAttributes, SpanAttributes, Duration, StatusCode, StatusMessage, " +
	"Events.Timestamp, Events.Name, Events.Attributes, " +
	"Links.TraceId, Links.SpanId, Links.TraceState, Links.Attributes, ChildSpanCount, ReceivedTime"

// InsertTraceData stores spans with the time they were received (one value
// per request), which statistics use to decide whether a record arrived in
// time for its bucket.
func (l *Log) InsertTraceData(ctx context.Context, data []*pb.OneSpan) error {
	received := time.Now().UnixNano()
	batch, err := clickhouseDB.PrepareBatch(ctx, "INSERT INTO otel_traces ("+spanColumns+")")
	if err != nil {
		return err
	}
	for _, d := range data {
		events := eventsFromProto(d)
		links := linksFromProto(d)
		err := batch.Append(
			d.Timestamp.AsTime().UnixNano(),
			d.TraceId,
			d.SpanId,
			d.ParentSpanId,
			d.TraceState,
			d.SpanName,
			d.SpanKind,
			d.ServiceName,
			d.ResourceAttributes,
			d.SpanAttributes,
			d.Duration,
			int8(d.StatusCode),
			d.StatusMessage,
			events.timestamps,
			events.names,
			events.attributes,
			links.traceIDs,
			links.spanIDs,
			links.traceStates,
			links.attributes,
			d.ChildSpanCount,
			received,
		)
		if err != nil {
			fmt.Println(err)
		}
	}
	return batch.Send()
}

// ClickHouse treats the "Events.*" and "Links.*" columns as Nested structures,
// so the arrays within each group must have the same length.
type spanEvents struct {
	timestamps []time.Time
	names      []string
	attributes []map[string]string
}

type spanLinks struct {
	traceIDs    []string
	spanIDs     []string
	traceStates []string
	attributes  []map[string]string
}

func eventsFromProto(d *pb.OneSpan) spanEvents {
	n := len(d.Events_Timestamp)
	events := spanEvents{
		timestamps: make([]time.Time, n),
		names:      padStrings(d.Events_Name, n),
		attributes: decodeAttributes(d.Events_Attributes, n),
	}
	for i, ts := range d.Events_Timestamp {
		events.timestamps[i] = ts.AsTime()
	}
	return events
}

func linksFromProto(d *pb.OneSpan) spanLinks {
	n := len(d.Links_TraceId)
	return spanLinks{
		traceIDs:    padStrings(d.Links_TraceId, n),
		spanIDs:     padStrings(d.Links_SpanId, n),
		traceStates: padStrings(d.Links_TraceState, n),
		attributes:  decodeAttributes(d.Links_Attributes, n),
	}
}

func padStrings(values []string, n int) []string {
	result := make([]string, n)
	copy(result, values)
	return result
}

// decodeAttributes reads one attribute map per event/link: log-sender encodes
// each map as JSON in Attribute.Value. Missing or malformed entries become
// empty maps, which keeps spans from older senders insertable.
func decodeAttributes(encoded []*pb.Attribute, n int) []map[string]string {
	result := make([]map[string]string, n)
	for i := range result {
		result[i] = map[string]string{}
		if i < len(encoded) && encoded[i] != nil {
			var attrs map[string]string
			if json.Unmarshal([]byte(encoded[i].Value), &attrs) == nil && attrs != nil {
				result[i] = attrs
			}
		}
	}
	return result
}
