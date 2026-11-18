# Improvement plan after the MC5 pilot

Status: proposal, 2026-11-18. Based on the first external pilot of Troubleshooting with MC5
(`C:\dev\agerimex\recstation`, `mc5-api`). The items were worked out together with the Claude
session that integrated MC5 ("MC5 session" below). Troubleshooting work happens in this
repository; MC5-side acceptance tests are written in recstation by the MC5 session.

## What the pilot showed

- **Integration:** MC5 sends traces through log-sender, but only with `TROUBLESHOOTING_ADDR` set.
  The work is on recstation branch `feat/troubleshooting-integration` (commits `0df1c74`,
  `ee82bca`, not pushed). Rules are in `mc5-api/quality.yaml`, and the workload is
  `tests/quality-gate-workload.cjs`.
- **Gate:** passed on real traffic. `POST /api/v1/login` has p95 224 ms and 4 database calls;
  `GET /api/v1/privy/rooms-with-devices` has p95 1.3 ms and 5 database calls. Of those 5, 4 come
  from the auth middleware, and the number varies between 4 and 5.
- **Outage:** mc5 never blocked while the receiver was down. It reconnected by itself, and the
  spans of the outage were dropped.
- **Three privacy leaks.** They came from the integration, not from the gate, and were found only
  by grepping ClickHouse:
  - SQL arguments
  - a login token in WebSocket query strings
  - camera `user:pass` in a span attribute **key**

  MC5 fixed all three before committing. The leaked rows were deleted from the test ClickHouse by
  hand.
- **Troubleshooting bug:** the published log-sender didn't compile in an outside application,
  because of an old `protos` pin.
- **Not proven:** that the gate catches a real regression in MC5. The test database had one room,
  so a query per room couldn't show.

## Stage 1: quick wins

Small, low risk, one PR.

### A. `--from`/`--to` optional with `--run`

The CLI finds the run's time range itself: the earliest and latest entry span of the run within
the last 24 h, extended by the existing read margin. An explicit window still takes precedence. In
the pilot, a window that started 5 s late made the gate unresolved.

- API: `quality/evaluate` accepts a run without a window and answers with the window it used.
  Either the API returns the run's range, or the CLI asks a new `quality/runs/{run}` first.
  Decide during implementation.
- **Acceptance:** a run checked without `--from`/`--to` gives the same verdict as with its exact
  window. A run with no spans at all is `no_executions`, not an error.

### B. Explainable database calls

- The evidence of a `db_calls` check lists the statements of the execution with the most calls:
  `db.statement` with placeholders only, truncated, in call order.
- What counts as a call is documented and shown in the result: CLIENT spans with `db.system`.
  Spans that only report on a call already counted are excluded, namely otelsql's
  `sql:rows_close` / `sql:rows_affected` (rows names `rows` and no statement).
- **Acceptance:** rooms-with-devices recorded with otelsql `TraceRowsClose` gives the same count
  as without it. A drop from 5 calls to 4 shows which statement is missing.

### C. log-sender works as an outside module

- Merge the `protos` pin fix (branch `fix/log-sender-protos-pin`).
- Add a CI job that builds a throwaway module requiring log-sender and protos at the PR's commit,
  with no `replace` and `GOWORK=off`. It compiles a small program that calls `NewTracer`,
  `NewLogExporter` and `Send`.
- **Acceptance:** the job fails on the old pin and passes with the fix.

### D. Wording

- The first use of "guardrail" in the CLI, reports and handbook gets a one-line definition.
- docs/quality-gate.md and the handbook page say in their first paragraph that a pass means "not
  observed in this run".
- A new "Integrate a Go service" guide that starts with **what your application must never send**,
  then how to send. It covers the env vars, the redaction checklist from the pilot, how to choose
  operations and limits, and a workload template.

## Stage 2: privacy by default

The next milestone. It's a prerequisite for using Troubleshooting with clinic data.

### E. Redaction in log-sender, on by default

A redacting step applied to every span and log record before export:

- drops `db.sql.args.*` and `db.query.parameter.*`
- strips userinfo (`user:pass@`) from URLs in attribute **keys and values**
- strips query strings from `http.target`, `url.full`, `url.query` and `http.url`
- applies a configurable denylist of key and value patterns
- marks a cleaned attribute (e.g. `troubleshooting.redacted: "db.sql.args,http.target"`) so the
  UI can say what was removed

Opting out is explicit: `WithSQLArgs()` keeps SQL arguments, and log-sender then logs once at
startup "SQL arguments are sent to <address>". Troubleshooting's own self-instrumentation opts in,
because its UI rebuilds readable SQL from the arguments. This is a behaviour change for existing
log-sender users, so it goes in the release notes.

**Acceptance:** the three pilot leaks, sent through log-sender, arrive cleaned:

- SQL arguments are absent
- `?token=SECRET123` is absent and `http.target` is the path
- `rtsp://root:pw@host/path` as a key and as a value arrives as `rtsp://host/path`

With `WithSQLArgs()`, the arguments arrive and the startup log line appears.

### E2. The same filter in the receiver

For plain OTLP senders. On by default, with the same rules. It can be turned off per installation,
e.g. for a sender that has opted in to SQL arguments, which then marks its spans. The UI fills SQL
arguments back into statements only for spans that carry them, and labels the result.

### F. `troubleshooting-check privacy`

Scans the spans and logs of a run (or a service and window) for:

- `://user:pass@` in keys and values
- `token=`, `access_token=` and `password=` in values
- bearer tokens
- bcrypt and argon2 hashes
- `db.sql.args.*` keys
- the configured denylist

It prints each finding with its span, attribute key and a masked value, and exits 1 on any
finding. A CI job can run it after the gate.

**Acceptance** is a privacy fixture: a run that reproduces the three MC5 leaks plus one clean run.
The MC5 session writes the MC5-side version; Troubleshooting gets a regression-fixture variant
with literal expected findings.

### G. Purge command

`troubleshooting-check purge --service S [--run R] --from --to [--dry-run]`, through an admin API
endpoint that needs a token. It deletes raw spans and log records of that scope and invalidates
statistics buckets that contained them, so a rebuild excludes them. It logs what it deleted:
counts and scope, never the content.

**Acceptance:** after a purge, the scope has 0 rows and no statistics include it. A dry run reports
the same counts without deleting.

## Stage 3: the gate proves scaling

### H. Scaling check across two runs

A new check type, `scaling`:

```yaml
- id: rooms-scale
  type: scaling
  entry: {service: mc5-api, operation: /api/v1/privy/rooms-with-devices, span_kind: server}
  measure: db_calls          # per execution
  small: {run_suffix: -1room, size: 1}
  large: {run_suffix: -10rooms, size: 10}
  max_growth: 0              # database calls may not grow with size (0 = constant)
```

The workload declares its input size as a run attribute (`troubleshooting.input_size`).

**Acceptance:** MC5 with a planted query per room fails, with evidence from both runs. MC5 as it
is passes. The MC5 session provides the 1-room and 10-room databases and the planted regression.
This is also the **missing proof that the gate catches a real MC5 regression**.

## Stage 4: where MC5 actually breaks

MC5's API is fast. Its real bugs live in ffmpeg, MediaMTX and WebRTC child processes and in long
recordings: a frozen picture, a camera offline after the first recording, a recording that stops
after 2 s.

### I1. Supervised child processes

A log-sender helper that wraps an `exec.Cmd`. It produces one span per process lifetime with exit
code, signal, restart count and a redacted, size-capped stderr tail as events. Restarts are linked
to the previous lifetime with span links.

### I2. Long-running operations

A recording span lasting 45 minutes, with its events (start, segment switches, reconnects, stop),
shown on a time axis instead of in a request list. Long spans must not distort latency statistics.
They need their own kind or attribute, which is decided here.

### I3. Span links for background work

A WebSocket command linked to the recorder goroutine it starts. It is shown in Timeline & Sequence
as a link, not a parent.

**Acceptance:** with the MC5 session, reproduce one real MC5 failure (e.g. a recording that stops
early) and find its cause from Troubleshooting alone.

## Decisions for the owner (not scheduled yet)

### J. Offline / light mode

Practice PCs can't run ClickHouse and get no updates after handover. Options:

- log-sender writes a rotating, size-capped, redacted local file (OTLP JSON) that the receiver
  imports later, as a support bundle
- a single-binary receiver plus UI on SQLite or DuckDB for laptops and CI

The second option is a large change to the storage layer.

### K. Child counts as an OpenTelemetry span processor

Child counts are log-sender's real value. As a span processor next to the standard `otlptracegrpc`
exporter, applications would no longer depend on log-sender's own protocol, which is what the
`protos` pin bug cost. The receiver then needs a way to carry the count over OTLP: a span
attribute it reads into `ChildSpanCount`.

### L. Smaller items

- An option in log-sender for rate-limited export error reporting. MC5 had to write its own
  `otel.SetErrorHandler`.
- `troubleshooting-check measure`: runs the entry operations of a run and writes a starter
  `quality.yaml` from today's values, with headroom and comments.

## Order and ownership

| Stage | Items | Troubleshooting (this repo) | MC5 session (recstation) |
|---|---|---|---|
| 1 | A–D | implement, fixture checks, docs | re-run the MC5 gate without `--from`/`--to`; check B's statement list |
| 2 | E, E2, F, G | implement | privacy fixture with the three real leaks; switch MC5 to log-sender's redaction and remove its own where it overlaps |
| 3 | H | check type, docs | 1-room and 10-room databases, planted query per room, CI job |
| 4 | I1–I3 | helpers, UI | instrument ffmpeg/MediaMTX supervision and recordings; reproduce one real failure |
| — | J, K, L | after the owner's decision | packaging input (offline installs) |

Each stage ends as for milestone 3:

- a PR with scenarios in `features/milestone_3/` (or a new milestone file)
- a section in `docs/milestone3-completion.md`
- CI green, including the regression fixture
- the MC5 pilot re-run against the new version
