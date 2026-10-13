package data

import (
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

var clickhouseDB driver.Conn // db *sql.DB

// New sets the connection and the limits every query runs with.
func New(clickhousePool driver.Conn, queryLimits QueryLimits) Models {
	clickhouseDB = clickhousePool
	limits = queryLimits

	return Models{
		Log: Log{},
	}
}

type Models struct {
	Log Log
}
