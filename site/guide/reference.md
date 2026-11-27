---
description: Configuration variables, what to do when something looks wrong, and a glossary of traces, spans, windows, shapes and populations.
---

# Reference

## Configuration

All configuration is environment variables; the defaults match `docker-compose.yml`.

| Variable | Used by | Default |
| --- | --- | --- |
| `CLICKHOUSE_ADDR` | receiver, API | `clickhouse-server:9000` (`localhost:19000` outside Docker) |
| `LISTEN_ADDR` | receiver, API | `:50055`, `:8094` |
| `OTLP_LISTEN_ADDR` | receiver (OTLP/gRPC) | `:4317` |
| `TROUBLESHOOTING_TOKEN` | receiver, log-sender | empty = no token check |
| `TROUBLESHOOTING_RUN` | API: tags its own spans with `troubleshooting.run` (for checking the API itself with the quality gate) | empty = untagged |
| `CORS_ALLOWED_ORIGINS` | API: browser origins allowed to call it directly | `http://localhost:5173,http://127.0.0.1:5173` (Vite dev server) |
| `UI_AUTH`, `UI_USER`, `UI_PASSWORD_HASH` | UI (Caddy basic auth) | `off` |
| `QUERY_TIMEOUT` | API, per query | `10s` |
| `QUERY_MAX_ROWS_TO_READ` | API, per query | `50000000` |
| `QUERY_MAX_MEMORY_BYTES` | API, per query | unset: an equal share of the ClickHouse server's memory limit per possible concurrent query, at most 1 GiB |
| `CLICKHOUSE_MAX_QUERIES` | API: queries interactive requests run at once (the statistics refresh has its own pool and a lower ClickHouse priority) | `4` |
| `CLICKHOUSE_ASYNC_INSERT` | receiver: `off` writes one part per request instead of ClickHouse async inserts | on |
| `MAX_SEARCH_WINDOW` | API: widest search window | `96h` |
| `TRACE_RETENTION`, `LOG_RETENTION` | receiver: raw spans and logs | `72h` |
| `STATS_RETENTION` | receiver: statistics and saved comparisons | `720h` (30 days) |
| `STATS_ALLOWED_LATENESS` | receiver and API (set both the same) | `24h` |
| `STATS_REFRESH_INTERVAL`, `STATS_REFRESH` | API: statistics rebuild | `1m`, on |

## When something looks wrong

| You see | Why, and what to do |
| --- | --- |
| Spans you just sent are missing | Check the window: an old From/To, or a narrow window, hides them. The searched window is printed under the fields |
| “Total unavailable” | The count failed or hit a limit. The rows are still correct; click retry |
| HTTP 504 / “query limit exceeded” | A query hit `QUERY_TIMEOUT`, row or memory limits. Narrow the window or add filters |
| HTTP 503 “busy; retry” or “short of memory right now; retry” | Every database connection was busy, or ClickHouse was at its total memory limit. Retry; narrowing the window doesn't help |
| A report section says unavailable or limit | Use that section's Retry, or Widen the window |
| Percentile shows “—” | Too few valid durations (20 / 200 / 1000 for p50 / p95 / p99) |
| “provisional statistics” notice | Hours less than a day old can still change when late records arrive |
| A problem's example won't open | Its raw records have passed raw retention; the counts stay |
| Logs not under any span | The log carries no span id, or one not in this trace and window. They are shown in the trace-level group, never guessed |

## Glossary

| Term | Meaning |
| --- | --- |
| Trace / execution | All spans and logs of one request, identified by a 32-hex trace id |
| Span | One operation in a trace, identified by trace id + span id |
| Root span | A span with no parent; the entry of an execution |
| Window | A half-open time range `[from, to)` every search and report uses |
| Shape | Entry operation + the set of distinct caller → callee calls of an execution |
| Population | Service + window + optional dimensions, the unit statistics are computed for |
| Bucket | One UTC hour of statistics; provisional until the allowed lateness has passed, then final |
| Revision | A stored version of a comparison, statistics bucket or rule set |
| Run | The spans of one test run, tagged with one `troubleshooting.run` id; the quality gate checks a run against `quality.yaml` |
| Gate | The quality gate's verdict for a run: failed, unresolved (not enough data) or passed |
| Requirement | A named intent in `quality.yaml` with an owner, a reason and the checks that enforce it: met, violated, not enough data, or not evaluated when no check enforces it yet |
| Recorded evaluation | A quality gate result kept by Troubleshooting with its rules and evidence snapshots (`--record`, the Action's `record`): the history of a project's requirements |
