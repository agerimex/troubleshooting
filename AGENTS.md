# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Overview

Troubleshooting is an MVP observability system: Go services send OpenTelemetry traces (and zerolog logs) over gRPC into ClickHouse, and a Vue 3 UI browses them. It is a monorepo of **independent Go modules** (no `go.work` is committed — it's in `.gitignore`) plus one npm project.

## Commands

Full stack (Docker):
```
make all              # builds docker-log-analysis:1, docker-log-ui:1, docker-log-receiver:1 and creates the external `troubleshooting_network`
docker-compose up     # clickhouse + services; UI at http://localhost:8095
```
`make all` fails on the final step if `troubleshooting_network` already exists — the images are built by then, so this is harmless on re-runs.

Go modules (run from inside each module directory):
```
cd log-receiver && go build ./cmd/grpc_api
cd log-analysis && go build ./cmd/rest_api
go test ./...                                  # in any module
go test ./internal/data -run TestAddFilter     # a single test
```
Tests are unit tests only (no ClickHouse needed). On Windows, `gofmt -l` lists every file because git checks them out with CRLF; pipe through `tr -d '\r'` to check real formatting.

UI (`log-ui/`):
```
npm install
npm run dev           # vite dev server; talks to the API at VITE_LOGS_APP_API_URL from .env (http://localhost:8094)
npm run build         # vue-tsc type-check + production build in parallel
npm run type-check
```

Regenerating protobuf code (`protos/logs/*.pb.go`) — needs a local clone of `open-telemetry/opentelemetry-proto` on the include path (see `protos/README.md`):
```
cd protos
protoc -I<path-to>/opentelemetry-proto -I. --go_out=. --go-grpc_out=. logs.proto
```

## Architecture

Data flow:
```
instrumented Go app ── log-sender (OTel exporter) ──gRPC :50055──▶ log-receiver ──▶ ClickHouse (otel_traces, logs)
                                                                                         ▲
browser ── Caddy :8095 (log-ui static /dist) ── /api/* ──▶ log-analysis :8094 (chi REST) ─┘
```

- **protos/** — `logs.proto` defines `LogService` (`LogMessage`, `SendSpans`) and a flattened `OneSpan` message. Module `github.com/agerimex/troubleshooting/protos`.
- **log-sender/** — library imported by client apps (module `github.com/agerimex/troubleshooting/log-sender`). `NewTracer(svcName, opts...)` installs a global OTel `TracerProvider` with a batch processor and a `CustomExporter` (one long-lived gRPC connection) that converts `ReadOnlySpan`s to `pb.OneSpan` (`spanToProto`) and calls `SendSpans`; `Shutdown(ctx)` flushes it. Address: `WithAddress` > the legacy `addrTrace` flag (read once in `NewTracer`; the library must never call `flag.Parse`). Token: `WithToken` > `TROUBLESHOOTING_TOKEN`. Export errors are returned to the span processor — the library must never exit or panic in the host app. `Duration` is sent in **microseconds**. Also contains an unfinished zerolog integration (`logger.go`).
- **log-receiver/** (module `log-receiver`) — gRPC server on `LISTEN_ADDR` (`:50055`). On startup it runs `CREATE TABLE IF NOT EXISTS` for `otel_traces` and `logs` (`internal/driver/clickhouse.go`), so the **schema lives here** (note: `driver.СreateLogs` starts with a Cyrillic `С`). Spans are inserted per request; the column order in `InsertTraceData` must match the table definition exactly, and the `Events.*` / `Links.*` arrays must have equal lengths (ClickHouse Nested). Log messages go through `internal/batch` (flush at 100 items or every second, drained on graceful shutdown). With `TROUBLESHOOTING_TOKEN` set, a unary interceptor (`cmd/grpc_api/auth.go`) requires `authorization: Bearer <token>`.
- **log-analysis/** (module `logs-backend`) — chi REST API on `LISTEN_ADDR` (`:8094`): `POST /api/v1/view-spans`, `POST /api/v1/count-spans`, `GET /api/v1/view-logs`. It instruments itself via log-sender (sends its own traces to `TRACE_RECEIVER_ADDR`). `internal/data/logs.go` builds SQL dynamically: `addFilter` reflects over non-zero `SpanFilter` fields to add WHERE clauses bound with ClickHouse named params. When adding a filter field, update the `SpanFilter` struct, the `switch` in `addFilter`, and `filterParams` (shared by the list and count queries; `TestFilterParamsBindEveryPlaceholder` catches a missing binding). It also reconstructs readable SQL for DB spans by substituting `db.sql.args.N` attributes into `db.statement` (`fillSQLArgs`), and `db.query.parameter.<name>` attributes into ClickHouse `{name:Type}` placeholders (`fillNamedParams`). Its own ClickHouse queries are traced as child spans of the request (`internal/data/tracing.go`), so handlers must pass `r.Context()` down, not `context.Background()`.
- **log-ui/** — Vue 3 + TypeScript + PrimeVue + Tailwind. Nearly all UI logic is in `src/components/TracePage.vue`. API access goes through a service/impl indirection: `src/api/*.api.ts` (singleton wrappers) are initialized with `src/logs-api/*.impl.ts` (fetch implementations). Built into a Caddy image; `Caddyfile` serves `/dist` and proxies `/api/*` to log-analysis (production `.env.production` uses empty, i.e. same-origin, API URLs). Optional basic auth is selected by `import auth_{$UI_AUTH:off}`; the UI must not send an `Authorization` header unless it has a real token, or it would override the browser's basic-auth credentials.

Key conventions spanning multiple components:
- **Root spans** are those with `ParentSpanId = "0000000000000000"`; log-analysis defaults `parent_id` to this when empty. The UI loads children lazily by requesting spans with `parent_id` = the expanded node's span id.
- **Cursor pagination**: there's no offset; the UI passes the last row's `UnixTime` as `time_from` and its `SpanId` as `after_span_id`, and the backend returns rows after `(UnixTime, SpanId)`, ordered by both. `UnixTime` is Int64 nanoseconds (the UI appends `'000000'` to JS millisecond timestamps). Paging only moves forward.
- Status is sent as a string (`unset`/`error`/`ok`) and mapped to `StatusCode` 0/1/2 via `StatusCodeMap`.
- Tables have a 3-day TTL (`ttl_only_drop_parts`).
- All configuration is environment variables with defaults matching docker-compose (table in README.md). Run a service outside Docker against the compose ClickHouse with `CLICKHOUSE_ADDR=localhost:19000`.
- Event/link attribute maps travel as one JSON-encoded `pb.Attribute` per event/link (documented in `logs.proto`), to avoid a proto change.

## Cross-module dependencies

log-analysis and log-receiver use the **local** `log-sender` and `protos` through `replace => ../...` directives in their `go.mod`, so changes to those modules take effect in the services immediately. Because of that, the Go Dockerfiles must be built with the **repository root** as context (`docker build -f log-analysis/Dockerfile .`, as the Makefile does); building from inside a module directory fails. If a service starts using another sibling module, add a `replace` and matching `COPY` lines in its Dockerfile. The root `.dockerignore` keeps `.git`, `node_modules` and `*.tar` out of the context.

External applications import `log-sender` from GitHub (the `replace` in `log-sender/go.mod` is ignored for them), so they only get changes once they're pushed and the application updates its dependency. Keep `log-sender`'s public API backward compatible. Module paths were renamed to the `agerimex` GitHub account — keep imports on `github.com/agerimex/troubleshooting/...`.
