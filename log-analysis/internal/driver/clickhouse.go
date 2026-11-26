package driver

import (
	"fmt"
	"os"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type Config struct {
	Addr     string
	Database string
	Username string
	Password string
}

// ConfigFromEnv reads CLICKHOUSE_ADDR, CLICKHOUSE_DATABASE, CLICKHOUSE_USER and
// CLICKHOUSE_PASSWORD. The defaults match docker-compose; for local runs
// against the compose ClickHouse use CLICKHOUSE_ADDR=localhost:19000.
func ConfigFromEnv() Config {
	return Config{
		Addr:     getenv("CLICKHOUSE_ADDR", "clickhouse-server:9000"),
		Database: getenv("CLICKHOUSE_DATABASE", "default"),
		Username: getenv("CLICKHOUSE_USER", "default"),
		Password: os.Getenv("CLICKHOUSE_PASSWORD"),
	}
}

func getenv(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}

// Connect opens a pool of at most maxConns connections: the number of queries
// it can run at once. A query that finds every connection busy waits for one
// (until its context ends or the dial timeout passes).
func Connect(cfg Config, maxConns int) (driver.Conn, error) {
	return clickhouse.Open(&clickhouse.Options{
		MaxOpenConns: maxConns,
		MaxIdleConns: maxConns,
		Addr:         []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Debugf: func(format string, v ...any) {
			fmt.Printf("CLICKHOUSE: "+format+"\n", v...)
		},
	})
}
