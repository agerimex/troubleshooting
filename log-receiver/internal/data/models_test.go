package data

import "testing"

// Inserts wait for the asynchronous flush, so a request is confirmed only
// after its rows are written; CLICKHOUSE_ASYNC_INSERT=off removes both.
func TestInsertSettings(t *testing.T) {
	defer func(prev bool) { AsyncInsert = prev }(AsyncInsert)

	AsyncInsert = true
	s := insertSettings()
	if s["async_insert"] != 1 || s["wait_for_async_insert"] != 1 {
		t.Errorf("async inserts: settings %v, want async_insert=1 and wait_for_async_insert=1", s)
	}

	AsyncInsert = false
	if s := insertSettings(); len(s) != 0 {
		t.Errorf("CLICKHOUSE_ASYNC_INSERT=off: settings %v, want none", s)
	}
}
