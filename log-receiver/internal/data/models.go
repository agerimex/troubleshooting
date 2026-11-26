package data

import (
	"context"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

var clickhouseDB driver.Conn // db *sql.DB

func New(clickhousePool driver.Conn) Models {
	clickhouseDB = clickhousePool

	return Models{
		Log: Log{},
	}
}

// AsyncInsert makes ClickHouse collect the inserts of many requests into one
// part (async_insert) instead of writing a part per request, which under a
// steady stream of small requests saves the merges that cost memory and CPU.
// wait_for_async_insert keeps the contract: an insert returns only after its
// rows are written, so nothing is confirmed before it is stored. The receiver
// turns it off with CLICKHOUSE_ASYNC_INSERT=off.
var AsyncInsert = true

// insertContext carries the insert settings to the driver.
func insertContext(ctx context.Context) context.Context {
	return clickhouse.Context(ctx, clickhouse.WithSettings(insertSettings()))
}

func insertSettings() clickhouse.Settings {
	if !AsyncInsert {
		return clickhouse.Settings{}
	}
	return clickhouse.Settings{"async_insert": 1, "wait_for_async_insert": 1}
}

type Models struct {
	Log Log
}

// Heartbeat records that this receiver is running and can write. Minutes
// without any heartbeat are shown as collection gaps in statistics.
func (l *Log) Heartbeat(ctx context.Context, receiver string) error {
	return clickhouseDB.Exec(ctx, `INSERT INTO collection_heartbeats ("Time", "Receiver") VALUES (now(), {receiver:String})`,
		clickhouse.Named("receiver", receiver))
}
