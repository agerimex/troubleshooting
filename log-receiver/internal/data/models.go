package data

import (
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
