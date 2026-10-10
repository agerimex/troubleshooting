package data

import (
	"testing"
	"time"

	pb "github.com/agerimex/troubleshooting/protos/logs"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestEventsFromProto(t *testing.T) {
	ts := time.Unix(1700000000, 0).UTC()
	span := &pb.OneSpan{
		Events_Timestamp: []*timestamppb.Timestamp{timestamppb.New(ts), timestamppb.New(ts)},
		Events_Name:      []string{"exception"}, // shorter than timestamps
		Events_Attributes: []*pb.Attribute{
			{Key: "exception", Value: `{"exception.message":"boom"}`},
			{Key: "broken", Value: "not json"},
		},
	}

	got := eventsFromProto(span)

	if len(got.timestamps) != 2 || len(got.names) != 2 || len(got.attributes) != 2 {
		t.Fatalf("nested arrays must have equal length, got %d/%d/%d", len(got.timestamps), len(got.names), len(got.attributes))
	}
	if !got.timestamps[0].Equal(ts) || got.names[0] != "exception" || got.names[1] != "" {
		t.Errorf("unexpected events: %+v", got)
	}
	if got.attributes[0]["exception.message"] != "boom" {
		t.Errorf("attributes[0] = %v", got.attributes[0])
	}
	if got.attributes[1] == nil || len(got.attributes[1]) != 0 {
		t.Errorf("malformed attributes must become an empty map, got %v", got.attributes[1])
	}
}

func TestSpansFromOldSendersHaveNoEventsOrLinks(t *testing.T) {
	span := &pb.OneSpan{}

	events := eventsFromProto(span)
	links := linksFromProto(span)

	if len(events.timestamps)+len(events.names)+len(events.attributes) != 0 {
		t.Errorf("expected no events, got %+v", events)
	}
	if links.traceIDs == nil || len(links.traceIDs)+len(links.spanIDs)+len(links.attributes) != 0 {
		t.Errorf("expected empty, non-nil link arrays, got %+v", links)
	}
}
