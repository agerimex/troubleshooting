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

type Models struct {
	Log Log
}

// Heartbeat records that this receiver is running and can write. Minutes
// without any heartbeat are shown as collection gaps in statistics.
func (l *Log) Heartbeat(ctx context.Context, receiver string) error {
	return clickhouseDB.Exec(ctx, `INSERT INTO collection_heartbeats ("Time", "Receiver") VALUES (now(), {receiver:String})`,
		clickhouse.Named("receiver", receiver))
}
