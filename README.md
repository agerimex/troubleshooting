# Troubleshooting

**Find out why a request failed or slowed down.** Troubleshooting shows what one request did across your services: its spans, database calls and correlated log records, as a table, a timeline or a sequence of calls. It compares releases to show what changed. You can start investigating right away, with no rules or CI setup. If your team already knows what must stay true, the quality gate checks those requirements against the traces of pull-request and release test runs. Self-hosted, built on OpenTelemetry traces.

Where to start:

| You have… | Start here |
| --- | --- |
| a trace ID, a slow operation or a release regression | **Investigate an execution** → [Investigating a failure](#investigating-a-failure), and the worked walkthrough [Investigate a slow request](https://agerimex.github.io/troubleshooting/guide/investigate-a-slow-request) |
| behaviour that must stay true, such as "at most 3 database calls per request" | **Check runtime requirements in CI** → [Checking runtime requirements in CI](#checking-runtime-requirements-in-ci), and `make demo` |

Install
------

```
make all      # builds the images and creates the troubleshooting_network network; safe to repeat
make up       # starts the stack (docker compose up -d --wait); UI on http://localhost:8095
make down     # stops the stack and keeps the data
```

ClickHouse keeps its data in the named volume `troubleshooting_clickhouse-data`, so it survives `docker compose down`, `up` and container recreation; only `docker compose down -v` (or `docker volume rm`) deletes it. On Docker Desktop the very first start of ClickHouse on an empty volume can take several minutes; `make up` waits for it.

By default every port is published on `127.0.0.1` only, and there is no authentication. To receive spans from other machines or open the UI to them, see [Security](#security).

Built with: ClickHouse (storage, raw data kept 3 days), Go (receiver and API), Vue 3 + TypeScript (UI), OpenTelemetry and gRPC.

Investigating a failure
------

A worked example with screenshots, from a release regression to its cause: [Investigate a slow request](https://agerimex.github.io/troubleshooting/guide/investigate-a-slow-request).
Start from what you have: a slow operation, a failed request, a recurring error, a trace ID, or a difference between two releases. The UI has six pages: **Log Analysis**, **Timeline & Sequence**, **Problems**, **Releases**, **Quality** and **Settings**. An investigation ends in one **report**: one trace's spans and its correlated log records together, within an explicit time window.

1. Find an entry record:
   * **Log Analysis** (the default page): set the window (From/To; an empty To means now) and filter spans by name, status, service or minimum duration, or pick exact services and span names in **Services and span names…** (services first, then their span names). As soon as you filter, **Root spans only** is switched off and the search covers spans at any depth, so a failed step under a successful request is found; tick it again to list only trace roots. Expanding a span shows its child spans and the log records that recorded exactly that span, inline in the same table and in time order; trace-level logs and logs naming an unknown span are a separate group under the root span. Filters select spans; they never search log text, and an OK span keeps its ERROR logs. The row's "⋯" button, right-click or the keyboard offer **Open timeline**, **Open sequence** and **Copy investigation link**.
   * **Timeline & Sequence**: opens on the sequence library, the distinct shapes of execution in a window (`docs/sequence-library.md`), each opening a representative execution; or open a trace by its id.
   * **Problems**: a recurring error group or slow call has retained examples; each opens in Timeline & Sequence. A log without a recorded TraceId can be inspected but opens no report: a SpanId or a timestamp alone never identifies a trace.
2. The report keeps the window of where it was opened and takes only the trace from the entry record: filters don't narrow it, and the record it was opened for is selected. It shows:
   * the scope (one trace, the window, when it was generated) and whether it is **Complete** or **Incomplete**, with the reasons; "Back to …" returns to the page it was opened from, as it was;
   * **Timeline** (default): every span on one time axis in parent order, with status and duration, and "parent missing" when a parent isn't in the report ("Widen by 1 h on each side" regenerates with a wider window); or **Sequence**: calls between services as arrows (a CLIENT span with exactly one SERVER child of another service), operation starts and ends, and ERROR log markers. The last chosen view is remembered;
   * next to it the selected span's logs (as task lanes or a list) or **All trace logs**, each with its log severity and association: the span it recorded, "No span identifier recorded", or "Referenced span not found within the window". A log is never attached to a span by its time. The span's events (exception type, message, stack trace) and links are one click away; a linked trace is marked "not inspected" and can be opened as its own report.
3. Each source has its own status. If spans or logs could not be read (database unavailable, query limit or timeout), that section says so and offers Retry; it is never shown as zero records, and the report is Incomplete. A section cut at its limit says how many records are shown and that more match. A window older than the retention period warns that records may have expired.
4. **Export HTML** or **Export JSON** saves the report as displayed (not a new query), including its limitations, the full trace whatever is selected, the view and the generation time. The HTML file has no scripts and shows recorded text as text. **Regenerate** queries again. **Investigation link** gives a link with the trace, window and selection (no log text); opening it generates the report again.

A span's status and a log's severity are what the application recorded. A successful span is not proof that the business task behind it succeeded, and a complete report does not prove every event was instrumented or delivered. A report covers one trace; other traces of the same business operation are not merged into it.

Checking runtime requirements in CI
------

A change can pass every functional test and still add twenty database calls per request. Static analysis can't see that, because the queries happen at runtime. When you know which behaviour must stay true, write it down next to the code and check every test run against it: database calls per request, services an operation must not call, failed requests, latency. Troubleshooting reads the traces of your pull request's or release candidate's test run, and every failed check links to the execution that broke it.

<img src="img/quality-pr-comment.png" alt="The quality gate's comment on a pull request: two requirements met, six checks passed, with values, limits and executions" width="560">

*The gate's comment on one of this repository's own pull requests. It checks how many ClickHouse queries each API request runs.*

### Try it locally

You need Docker, `make` and Go. The first start of ClickHouse on Docker Desktop can take several minutes; later starts take seconds. The example is a small orders service (`examples/quality-gate/`) with a planted regression: prices are read one by one (a query per item) and a retired service is called. Every functional test still passes.

One command does all of it: builds and starts the stack, runs the example's tests twice, and checks each run. The gate passes the good version and fails the regression (up to 9 database calls where the limit is 3, and a call to `legacy-pricing`). The reports are written to `.demo/`:

```
git clone https://github.com/agerimex/troubleshooting.git
cd troubleshooting
make demo
```

The same steps by hand:

```
git clone https://github.com/agerimex/troubleshooting.git
cd troubleshooting
make all && make up          # builds and starts the stack; UI on http://localhost:8095
go install github.com/agerimex/troubleshooting/quality-check/cmd/troubleshooting-check@v0.1.1

cd examples/quality-gate
export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4317 OTEL_EXPORTER_OTLP_INSECURE=true

# 1. The good version: the gate passes (exit code 0)
OTEL_RESOURCE_ATTRIBUTES=troubleshooting.run=demo-good go test -count=1 ./...
troubleshooting-check --url http://localhost:8095 --rules quality.yaml --run demo-good

# 2. The regression: tests still pass, the gate fails (exit code 1)
EXAMPLE_REGRESSION=1 OTEL_RESOURCE_ATTRIBUTES=troubleshooting.run=demo-bad go test -count=1 ./...
troubleshooting-check --url http://localhost:8095 --rules quality.yaml --run demo-bad
```

The second check names the broken rules (9 database calls where the limit is 3, a call to `legacy-pricing`) and links to a request that broke them. Open the link to see that request's timeline. If a check says "not enough data", wait a few seconds for the spans to arrive and run it again.

### How it works

```
your tests ──▶ your application ──OpenTelemetry traces──▶ Troubleshooting ──▶ ClickHouse
                                                                │
                              quality.yaml (your rules) ──▶ troubleshooting-check
                                                                │
                                         pass / fail / not enough data + evidence
                                         (PR comment, report, link to the request)
```

1. Your application sends traces while its tests run. Each span of the run carries one tag: `troubleshooting.run=<run id>`.
2. `troubleshooting-check` reads the run and checks it against `quality.yaml`.
3. The result is pass, fail or not enough data. Every failure names the request behind it.
4. When the gate flags a violation, the UI shows the execution behind it: its spans and logs as a timeline or a sequence of calls between services.

The rule types:

| Rule | Example | Flags in a test run |
| --- | --- | --- |
| `db_calls` | at most 3 database calls per request | N+1 queries, a query per row |
| `forbidden_call` | `start_session` must never call `directory-api` | broken architecture rules, calls to retired services |
| `errors` | no failed requests | new failures that tests didn't assert on |
| `latency` | p95 under 800 ms | slow operations (noisy in CI, so often advisory) |

Counts depend on the code and the test data, not on the machine, so `db_calls`, `forbidden_call` and `errors` make good hard gates. They can still vary a little: an application that caches a lookup makes one call fewer on some requests. Set the limit at the highest count you measured, not at a typical one. Latency depends on the machine, so this repository uses it as an advisory check.

### Words used in this guide

| Term | Meaning |
| --- | --- |
| **Run** | One test run, identified by its `troubleshooting.run` tag |
| **Entry** | The operation a check starts from, such as `POST /api/v1/login` (the `entry:` of `quality.yaml`) |
| **Execution** | One run of the entry and everything it caused: usually one request, or one session in a desktop app. A trace can contain several |
| **Check** | One measurable rule, for example "at most 3 database calls per execution" |
| **Requirement** | Why checks exist: a title, an owner and a reason. It is met only when its checks pass |
| **Result** | Pass, fail, or not enough data |
| **Evidence** | The executions behind a result, saved with the report so they survive after the raw data expires |

### In your CI

```yaml
- id: troubleshooting
  uses: agerimex/troubleshooting/quality-check/stack@quality-check/v0.1.1   # starts a private Troubleshooting inside the job
- run: go test ./...                                          # or your own test command
  env:
    OTEL_EXPORTER_OTLP_ENDPOINT: ${{ steps.troubleshooting.outputs.otlp-endpoint }}
    OTEL_EXPORTER_OTLP_INSECURE: "true"
    OTEL_RESOURCE_ATTRIBUTES: troubleshooting.run=ci-${{ github.run_id }}-${{ github.run_attempt }}
- uses: agerimex/troubleshooting/quality-check@quality-check/v0.1.1   # a release; @main for the newest
  with:
    url: ${{ steps.troubleshooting.outputs.url }}
    ui: ${{ steps.troubleshooting.outputs.ui }}
    run: ci-${{ github.run_id }}-${{ github.run_attempt }}
    rules-from: base                                          # a pull request can't loosen its own limits
```

The step posts one comment on the pull request (updated on re-runs), writes a summary to the job page, uploads the reports, and fails the job when the gate fails or has not enough data. No Troubleshooting server is needed: the `stack` action starts one inside the job. The full workflow, the rules file format and the exit codes are in [docs/quality-gate.md](docs/quality-gate.md).

This repository gates its own pull requests the same way (`.github/quality.yaml`: database queries per API request).

### What a pass means, and what it doesn't

- **A pass means the limits were not broken in this run.** Nothing more. Requests your tests didn't make were not checked.
- **A missing measurement is never a pass.** Too few executions, missing spans or no database calls to count give "not enough data" (exit code 2), not a pass.
- **The workload must be able to show the problem.** A list with one row hides a query per row. Use test data big enough to expose N+1.
- **Only recorded calls count.** Sampling or missing instrumentation can hide calls.
- **A successful span is not proof that the business task succeeded.** Span status and log severity are what the application recorded.
- **Advisory checks and the baseline are shown but never decide the result.**

### Static analysis vs. runtime checks

| Static analysis | Troubleshooting |
| --- | --- |
| Reads the source code | Reads the traces of a test run |
| Rules and quality gate | Rules in `quality.yaml`, next to your code |
| Comment on the pull request | Comment on the pull request |
| Issue → the line of code | Failed check → the request that broke it, as a timeline |
| New code compared with the main branch | `--baseline` compares with a main-branch run; `rules-from: base` stops a pull request from loosening its own limits |
| Project history | Requirement history: when it broke, which commit fixed it |

One difference matters: static analysis only needs the code. Troubleshooting needs the code to **run**. Your tests are the workload. If the run didn't produce enough requests to judge, the result is **not enough data**, never a pass.

Integrating your application
------
The usual way to integrate is to give the prompt below to your coding agent (Claude Code, Codex or another) in your project's repository. It reads the project first and reports before changing anything. The application sends nothing unless an environment variable names the receiver. The agent keeps personal data and secrets out of the spans, verifies that against the stored spans, and finishes with a quality gate run that you can repeat in CI. The steps behind it are under [Integrating by hand](#integrating-by-hand).

### Prompt for your coding agent

Copy everything in the block, replace the `<…>` placeholders (or leave them, and the agent asks), and paste it into your agent:

````text
Integrate Troubleshooting (https://github.com/agerimex/troubleshooting, self-hosted OpenTelemetry
trace viewer + quality gate) into this project. Rules: work on a new branch; never commit, push,
deploy or change shared config without asking me; ask before deleting anything.

1. READ FIRST, CHANGE NOTHING. Find out and report to me: language and version; HTTP router; DB
   layer (driver, ORM); any existing OpenTelemetry, tracing or logging setup (and config keys
   for it that shipped configs already set); whether request contexts reach the DB calls; how it
   is built, tested and deployed (Docker? offline/on-premise installs? updates possible?); which
   accounts or test data exist. Name 1-3 important operations to measure. Wait for my OK.

2. INSTRUMENT. Off unless the env var TROUBLESHOOTING_ADDR=<receiver host:port> is set. Never
   fatal, never blocking at startup; a dead receiver must not slow requests. Do NOT reuse an
   existing config key, even if its name fits: it may already be set at installs.
   - Go: github.com/agerimex/troubleshooting/log-sender: sender.NewTracer("<service>",
     sender.WithAddress(addr), sender.WithResourceAttributes({service.version,
     deployment.environment.name, troubleshooting.run=$TROUBLESHOOTING_RUN if set})), and
     sender.Shutdown(ctx) on exit. Router middleware (otelchi with WithChiRoutes, or otelhttp)
     so span names are route patterns, not raw paths with ids. DB: otelsql with
     TraceQueryWithoutArgs, db.system set per driver, and no rows_close/rows_affected spans
     (they double the gate's db_calls). Get log-sender with `go get ...log-sender@v0.1.1`.
   - Other languages: the standard OpenTelemetry SDK, OTLP over gRPC to <receiver host>:4317
     (set the protocol explicitly: some SDKs default to HTTP, which is not supported).
   - Throttle export errors: otel.SetErrorHandler that logs the first failure, then at most one
     line a minute with a count. Otherwise a down receiver writes a line every 5 s, forever.

3. NEVER SEND (audit every span, attribute KEY and value, including spans the project already
   creates itself): SQL arguments; query strings (tokens travel as ?token=; otelchi puts the raw
   RequestURI into http.target, so hand it the path only); user:password in URLs (RTSP/DB/stream
   URLs, also when used as an attribute key); Authorization and cookie headers; request/response
   bodies; names, emails or ids of people in span names or attributes. Redact in the app, before
   export.

4. VERIFY END TO END. Start Troubleshooting locally:
     git clone https://github.com/agerimex/troubleshooting.git && cd troubleshooting
     make all && make up    # UI http://localhost:8095, API 127.0.0.1:8094, gRPC 50055, OTLP 4317
   Run the app from a scratch copy with its own fresh test database (logins may invalidate other
   sessions' tokens), with TROUBLESHOOTING_ADDR=127.0.0.1:50055 TROUBLESHOOTING_RUN=<run id>.
   Send requests that carry a password you choose, a ?token=SECRET123, and anything with URL
   credentials. Wait 6 s, then:
     go install github.com/agerimex/troubleshooting/quality-check/cmd/troubleshooting-check@v0.1.1
     troubleshooting-check privacy --url http://127.0.0.1:8094 --service <service>   # must exit 0
   It finds SQL arguments, URL credentials (keys too), tokens in query strings, bearer tokens and
   password hashes. Your own values it can't know, so also check that the password and a
   returned token are nowhere (Q="docker exec logs-clickhouse-server clickhouse-client --query"):
     $Q "SELECT count() FROM otel_traces WHERE ServiceName='<service>' AND positionCaseInsensitive(concat(SpanName,' ',StatusMessage,' ',toString(SpanAttributes),' ',toString(ResourceAttributes),' ',toString(\`Events.Attributes\`),' ',toString(\`Links.Attributes\`)), '<needle>') > 0"
   Then kill the receiver mid-run and check that the app keeps answering at normal speed.

5. QUALITY GATE. Write a workload script (N requests per operation, default 50; one account
   nobody else uses; prints the UTC --from/--to window, starting before the first request).
   Measure first, then write quality.yaml
   (https://github.com/agerimex/troubleshooting/blob/main/docs/quality-gate.md): run.executions = N,
   latency p95 and db_calls per operation, limits with headroom for slower CI. Use a dataset big
   enough to expose N+1 queries (a list with 1 row hides them). Run:
     go install github.com/agerimex/troubleshooting/quality-check/cmd/troubleshooting-check@v0.1.1
     troubleshooting-check --url http://127.0.0.1:8094 --rules quality.yaml --run <run id> \
       --from <UTC> --to <UTC> --markdown summary.md --ui none   # 0 pass,1 fail,2 unresolved,3 error

6. GOTCHAS. Don't `go mod tidy` or upgrade deps into a newer Go version than the build images
   use. If Go says a package is missing that go.mod lists, run `go mod verify`: "dir has been
   modified" is a broken module cache entry; move that dir aside, don't change go.mod. Report
   anything you can't verify as unverified.

7. REPORT TO ME: files changed and why; what you verified, with the numbers (query results, gate
   output, outage behaviour); what's open; and the exact commands to reproduce. Don't commit
   until I say so.
````

### Integrating by hand

**1. What your application must never send.** A span leaves your application as soon as it is exported, and the receiver stores what it gets. Check every span before you switch tracing on: those of the libraries and those your own code creates (a startup span with a camera URL as attribute key is as much a leak as a recorded SQL argument):

* SQL arguments. Database instrumentation often records them (`db.sql.args.*`), and they are login names, password hashes and personal data. Record statements with placeholders only (for `go.nhat.io/otelsql`: `TraceQueryWithoutArgs()`, not `TraceQueryWithArgs()`).
* Query strings. Router middleware records the request URI (`http.target`), and WebSocket URLs often carry `?token=…`. Strip the query before the middleware sees it, or record the path only.
* User and password in URLs (`rtsp://user:pass@camera/…`, database DSNs), in attribute **values and keys**.
* `Authorization` headers and other headers, cookies, request and response bodies.
* Names or ids of people in span names. Use route patterns (`/api/v1/users/{id}`), not raw paths.

log-sender and the receiver redact the most common of these by default (below), but only the application knows its own data. Then verify against the stored spans: `troubleshooting-check privacy --service <service>` must exit 0, and no row may contain a password or a token you just used ([docs/privacy.md](docs/privacy.md)).

**2. Send spans, off by default.** Go applications use log-sender (spans with child counts, plus logs that carry their span):

```
go get github.com/agerimex/troubleshooting/log-sender@v0.1.1
```

```go
// Off unless TROUBLESHOOTING_ADDR names a receiver; never fatal.
func initTroubleshooting() {
	addr := os.Getenv("TROUBLESHOOTING_ADDR") // e.g. 127.0.0.1:50055
	if addr == "" {
		return
	}
	attrs := map[string]string{
		sender.AttrServiceVersion: version,
		sender.AttrEnvironment:    "development",
		sender.AttrSamplingPolicy: "unsampled",
	}
	if run := os.Getenv("TROUBLESHOOTING_RUN"); run != "" {
		attrs["troubleshooting.run"] = run // tags a test run for the quality gate
	}
	if _, err := sender.NewTracer("orders-api", sender.WithAddress(addr), sender.WithResourceAttributes(attrs)); err != nil {
		log.Printf("troubleshooting: traces are not sent: %v", err)
	}
}
// main: initTroubleshooting() before the database is opened; sender.Shutdown(ctx) on exit flushes the buffer.
```

Add router middleware (`otelchi.Middleware("orders-api", otelchi.WithChiRoutes(mux))` for chi, `otelhttp` for `net/http`) and database instrumentation (`otelsql`, without `TraceRowsClose` and `TraceRowsAffected`: their spans would count as extra database calls in the quality gate). Pass the request's context down to the database calls, or they don't belong to the request. The token for a receiver that requires one comes from `TROUBLESHOOTING_TOKEN`. Use a new environment variable for the switch rather than an existing config key that shipped configs already set. Don't let `go mod tidy` raise your `go` version above the one your build images use.

If the receiver is unreachable, the application keeps running and spans are dropped. Stop the receiver once while the application serves requests and check that it keeps answering at normal speed: that is the proof that tracing never blocks it. Each failed export is reported through OpenTelemetry's error handler, by default one log line every 5 s; install a throttled handler (`otel.SetErrorHandler`) for installations where the receiver can be away for long.

**Resources of each call (optional).** `sender.WithProcessResources()` adds the machine and the process to every span (host name, process id, CPU count, the machine's memory and the process's memory limit, its container's when it has one) and, to every entry call (an incoming request or a background job: a span with no parent in the process), what the process used while it ran: CPU time, memory allocated, heap, RSS and container memory at its end. Go can't attribute CPU or allocations to one request, so these are the whole process's figures for the time of the call; `troubleshooting.call.overlapping_calls` says how many other entry calls ran meanwhile (0: the figures are the call's own). Memory figures need Linux; elsewhere only CPU time is recorded. The execution viewer shows them for the selected span, under the process it ran in. Troubleshooting's own API uses it and also records, per ClickHouse query, what ClickHouse measured (CPU time from its `query_log`, peak memory, rows and bytes read).

Other languages send OTLP/gRPC to port 4317 (see below). OTLP carries no child counts, so their `quality.yaml` must not set `run.child_counts: required`, or every database-call check stays unresolved.

**3. Check a test run.** Write `quality.yaml` for one to three operations that matter. Run them first with generous limits and set the real limits from what you measured (see [In your CI](#in-your-ci) and [docs/quality-gate.md](docs/quality-gate.md)). The test data must be able to show the problem: a list with one entry hides a query per entry. Start the application with `TROUBLESHOOTING_ADDR` and `TROUBLESHOOTING_RUN`, make a fixed number of requests (as many as `run.executions` declares), wait about 6 s for the last batch, and run `troubleshooting-check` with a window that starts before the first request. An unresolved result (exit 2) usually means the window or the number of requests doesn't match `run.executions`: the gate counts what it can't see as missing data, never as a pass.

Sending logs
------
Logs go to the same receiver address. Each record carries the trace id and span id of the span active when it was written, so the report can show it with that span.

```
exporter, err := sender.NewLogExporter("orders-api", sender.WithAddress("localhost:50055"))
...
w := sender.NewLogWriter(exporter, time.Second) // sends in the background every second
defer w.Close(context.Background())             // final flush
logger := zerolog.New(io.MultiWriter(os.Stdout, w)).Hook(sender.TraceHook{})

logger.Error().Ctx(ctx).Msg("Storage returned 403 AccessDenied") // ctx holds the current span
```

* `TraceHook` adds `trace_id`, `span_id` (only when `ctx` has a span), a `record_id` and a precise time; other fields become attributes.
* Delivery is confirmed: the receiver answers only after the records are stored. `LogWriter` keeps unconfirmed records and retries them with the same `record_id`, so a retry is stored once in reports. Its buffer holds 10 000 records; when the receiver is down longer, the oldest are dropped and counted (`Dropped()`); `Err()` is the last delivery error.
* Without zerolog: `exporter.Send(ctx, []sender.LogRecord{sender.NewLogRecord(ctx, "ERROR", "...")})` returns nil only after the receiver stored them; on an error, send the same records again.
* Older senders that only set a message and a time still work; their records have no correlation and no severity.

Send traces and logs from any language (OTLP)
------
The receiver also accepts the standard OpenTelemetry protocol (OTLP over gRPC) on port 4317, so any application instrumented with an OpenTelemetry SDK (Go, Java, Python, Node.js, .NET, …) or an OpenTelemetry Collector can send traces and logs without log-sender. Logs that carry the trace and span ids of the active span (OpenTelemetry SDKs add them) appear under that span in Log Analysis and the execution report. Usually environment variables are enough:

```
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
OTEL_EXPORTER_OTLP_PROTOCOL=grpc
# or per signal: OTEL_EXPORTER_OTLP_TRACES_ENDPOINT / OTEL_EXPORTER_OTLP_LOGS_ENDPOINT,
# OTEL_EXPORTER_OTLP_TRACES_PROTOCOL=grpc / OTEL_EXPORTER_OTLP_LOGS_PROTOCOL=grpc
OTEL_SERVICE_NAME=orders-api
# if the receiver requires a token:
OTEL_EXPORTER_OTLP_HEADERS="authorization=Bearer <token>"
```

Traces and logs over gRPC are supported (not OTLP/HTTP on port 4318, not metrics). Some SDKs, e.g. Node.js, default to HTTP, so set the protocol explicitly. With Serilog, the OpenTelemetry sink sends logs the same way:

```csharp
Log.Logger = new LoggerConfiguration()
    .WriteTo.OpenTelemetry(o =>
    {
        o.Endpoint = "http://127.0.0.1:4317";
        o.Protocol = OtlpProtocol.Grpc;
        o.ResourceAttributes = new Dictionary<string, object> { ["service.name"] = "orders-api" };
    })
    .CreateLogger();
```

How an OTLP log record is stored, one rule per field:

* **Time:** the record's time, or its observed time when the time is 0; a record with neither is rejected.
* **Trace and span ids:** as lower-case hex; empty or all-zero ids (a record written outside a span) are stored as not recorded; any other wrong length is rejected.
* **Service:** the resource's `service.name`, the same value its spans get.
* **Severity:** the level name normalized like log-sender's (`Information` → `INFO`, `Verbose` → `TRACE`, `Warning` → `WARN`, `Critical` → `FATAL`); without a name, from the severity number by the OTLP ranges (1–4 TRACE … 21–24 FATAL).
* **Message and attributes:** a string body as is, other values as text (maps and arrays as JSON); the record's attributes as strings. Resource attributes other than `service.name` are not added.
* **Record id:** the `log.record.uid` attribute if present, otherwise a hash of the record (service, time, ids, severity, body and attributes), so an exporter's retry of the same batch is read once. Two truly identical records with the same nanosecond timestamp are therefore read as one.

Rejected records are reported as a partial success (`rejected_log_records`) while the valid records of the same request are stored; a failed insert answers `Unavailable`, which exporters retry. Logs are redacted like log-sender's (`docs/privacy.md`).

### Example: a Windows .NET 8 desktop or CLI app

A process that starts, does a few operations and exits has three traps a web service doesn't: spans that never leave the batch buffer, one long trace instead of one per operation, and no incoming request to tag. The steps below avoid all three. Commands are PowerShell.

**Ports.** Send to `127.0.0.1`, not `localhost`: `localhost` may resolve to `::1` first, and the test stack publishes all its ports on IPv4 loopback only. The dev stack publishes its ports on loopback too, unless `TROUBLESHOOTING_BIND` opens the receiver and UI (see [Security](#security)). The receiver speaks plaintext gRPC, so the endpoint URL starts with `http://` — that is OTLP over gRPC without TLS, not OTLP/HTTP, which the receiver does not support. Plaintext is for a local stack on the same machine only; never send it across a network. Use the three ports of one stack together:

| Stack | OTLP/gRPC | API | UI |
| --- | --- | --- | --- |
| dev (`make up`) | `4317` | `8094` | `8095` |
| isolated test stack (`docker-compose.test.yml`) | `24317` | `28094` | `28095` |

**Instrument.** Create the project and add the packages:

```powershell
dotnet new console -n media-tool -f net8.0
cd media-tool
dotnet add package OpenTelemetry
dotnet add package OpenTelemetry.Exporter.OpenTelemetryProtocol
```

Replace `Program.cs` with this (the whole file). Tracing stays off unless an environment variable names the receiver:

```csharp
using System.Diagnostics;
using OpenTelemetry;
using OpenTelemetry.Exporter;
using OpenTelemetry.Resources;
using OpenTelemetry.Trace;

// One fixed name. AddSource must name exactly this, or StartActivity returns null and nothing is sent.
const string sourceName = "Contoso.MediaTool";
using var source = new ActivitySource(sourceName);

TracerProvider? tracing = null;
var endpoint = Environment.GetEnvironmentVariable("TROUBLESHOOTING_OTLP"); // e.g. http://127.0.0.1:24317
if (!string.IsNullOrEmpty(endpoint))
{
    var attrs = new List<KeyValuePair<string, object>> { new("deployment.environment.name", "test") };
    var run = Environment.GetEnvironmentVariable("TROUBLESHOOTING_RUN");
    if (!string.IsNullOrEmpty(run)) attrs.Add(new("troubleshooting.run", run)); // tags the test run
    tracing = Sdk.CreateTracerProviderBuilder()
        .SetResourceBuilder(ResourceBuilder.CreateDefault()
            .AddService("media-tool", serviceVersion: "1.4.0")
            .AddAttributes(attrs))
        .AddSource(sourceName)
        .AddOtlpExporter(o =>
        {
            o.Endpoint = new Uri(endpoint);       // http:// = plaintext gRPC
            o.Protocol = OtlpExportProtocol.Grpc; // traces and logs; don't point metric exporters here
        })
        .Build();
}
try
{
    foreach (var file in args)
    {
        // One root per operation; work inside it are children.
        using var root = source.StartActivity("convert-file");
        root?.SetTag("media.size_bytes", new FileInfo(file).Length); // sizes and counts, never names or contents
        using (source.StartActivity("decode")) { /* ... */ }
        using (source.StartActivity("encode")) { /* ... */ }
    }
}
finally
{
    // The batch processor exports every 5 s; a process that exits sooner loses its spans without this.
    tracing?.ForceFlush(5000);
    tracing?.Dispose();
}
```

* **One root per operation.** Don't wrap the whole process (or a desktop app's main window) in one span: every operation would be a child of it, Log Analysis would show one trace, and the quality gate would count one execution. Each operation's root must be disposed before the next one starts; in a UI event handler that runs inside another activity, set `Activity.Current = null` before starting the root. For a detached async root (an operation started from a UI handler that keeps running after the handler returns), save `Activity.Current`, set it to `null`, start the root, and restore the saved value right after starting it, so the handler's own code and later operations don't end up under the detached root.
* **Short process exit.** `ForceFlush` + `Dispose` in `finally` covers a normal return and exceptions; `Environment.Exit` skips `finally`, so flush before calling it. Ctrl+C: flush in `Console.CancelKeyPress`.
* **Keep data out.** Never record file paths (they contain `C:\Users\<name>`), file names, file contents or recognized text, window titles, command-line arguments or anything typed by the user. Sizes, counts, durations and result codes are enough.

**Run it** against the stack you check:

```powershell
$env:TROUBLESHOOTING_OTLP = "http://127.0.0.1:24317"
$run = "local-$(Get-Date -Format yyyyMMdd-HHmmss)"
$env:TROUBLESHOOTING_RUN  = $run
dotnet run -- .\samples\CANARY-7f3a.wav   # each argument is one convert-file; 20+ per operation for a latency check, see below
Remove-Item Env:TROUBLESHOOTING_OTLP, Env:TROUBLESHOOTING_RUN
```

**Check it in the UI.** Open Log Analysis (`http://127.0.0.1:28095`), set the window, filter the service and tick "Root spans only": one row per operation (`convert-file`), and expanding a row shows `decode` and `encode` under it. No rows usually means a mismatched `AddSource` name, a `localhost` endpoint, an OTLP/HTTP exporter (port 4318 or protocol `http/protobuf`), or a process that exited without flushing.

**Check it through the API**, the same thing without the UI:

```powershell
$to   = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds() * 1000000
$from = $to - 3600L * 1000000000
$body = @{ time_from = "$from"; time_to = "$to"; scope = "root"; service_name = "media-tool"; rows_per_page = 100 } | ConvertTo-Json
(Invoke-RestMethod -Method Post -Uri http://127.0.0.1:28094/api/v1/view-spans -ContentType application/json -Body $body).data.Spans |
  Select-Object name, duration, childSpanCount, traceId    # duration in µs; one row per operation, childSpanCount > 0
```

**Canary check.** Put a unique value (`CANARY-7f3a` above) where personal data would travel — a file name, a path, text the app processes — and check that it is stored nowhere, after checking that the run's spans did arrive (otherwise 0 proves nothing):

```powershell
troubleshooting-check privacy --url http://127.0.0.1:28094 --service media-tool   # must exit 0
$q = "SELECT count() FROM otel_traces WHERE ServiceName='media-tool' AND positionCaseInsensitive(concat(SpanName,' ',StatusMessage,' ',toString(SpanAttributes),' ',toString(ResourceAttributes),' ',toString(``Events.Attributes``)), 'CANARY-7f3a') > 0"
docker exec troubleshooting-test-clickhouse clickhouse-client --query $q            # must print 0 (dev stack: logs-clickhouse-server)
```

**Quality gate.** In `quality.yaml` the entry is the root: `service: media-tool`, `operation: convert-file`. A latency check needs at least 20 executions (its default minimum), so run each checked operation at least 20 times and declare that number as `run.executions`. `troubleshooting-check --url http://127.0.0.1:28094 --rules quality.yaml --run $run --ui none` exits 0 passed, 1 failed (a limit was broken: fix the code or the limit), 2 unresolved and 3 error. Exit 2 is not a failure but missing evidence: fewer executions than declared (spans not flushed, the run tag missing, too few runs), no database call for a `db_calls` check, or executions without a valid duration. Read the reason in the output before changing limits.

**Plain `404 page not found` from an endpoint this README describes** (e.g. `/api/v1/privacy/scan` or `/api/v1/quality/evaluate`) usually means the API container runs an old, cached `docker-log-analysis:1` image. The compose files only use local images and never rebuild them. Rebuild the images from your checkout with `make all`, then recreate the stack with `make up` (test stack: `make test-stack-up`). Compose replaces only the containers whose image changed, and the ClickHouse volume is kept. To check which image a container runs, compare `docker inspect --format '{{.Image}}' <container>` with `docker image inspect --format '{{.Id}}' docker-log-analysis:1`.

Configuration
------
All settings are environment variables; docker-compose reads them from the shell or a `.env` file next to `docker-compose.yml`. Everything works without them.

| Variable | Used by | Default |
| --- | --- | --- |
| `CLICKHOUSE_ADDR` | receiver, analysis | `clickhouse-server:9000` (`localhost:19000` when running a service outside Docker) |
| `CLICKHOUSE_DATABASE`, `CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD` | ClickHouse, receiver, analysis | `default`, `default`, empty |
| `LISTEN_ADDR` | receiver, analysis | `:50055`, `:8094` |
| `OTLP_LISTEN_ADDR` | receiver (OTLP/gRPC) | `:4317` |
| `TRACE_RECEIVER_ADDR` | analysis (its own traces) | `log-receiver-compose:50055` |
| `CORS_ALLOWED_ORIGINS` | analysis | `http://localhost:5173,http://127.0.0.1:5173` (Vite dev server) |
| `APP_ENV` | analysis | empty; `development` pretty-prints JSON |
| `TROUBLESHOOTING_RUN` | analysis: tags its own spans with `troubleshooting.run` (the quality gate of its own CI) | empty = untagged |
| `TROUBLESHOOTING_TOKEN` | receiver (both ports), log-sender | empty = no token check |
| `TROUBLESHOOTING_BIND` | docker-compose: the address the receiver (50055, 4317) and the UI (8095) are published on; `0.0.0.0` opens them to the network (then set `TROUBLESHOOTING_TOKEN` and `UI_AUTH`) | `127.0.0.1` |
| `TROUBLESHOOTING_REDACT` | receiver: `off` stores spans and logs as sent; otherwise SQL arguments (unless the sender opted in), URL credentials and query strings are removed before they are stored (`docs/privacy.md`) | on |
| `UI_AUTH`, `UI_USER`, `UI_PASSWORD_HASH` | UI (Caddy) | `off` |
| `QUERY_TIMEOUT` | analysis: ClickHouse `max_execution_time` per query (a request runs at most two) | `10s` |
| `QUERY_MAX_ROWS_TO_READ` | analysis: ClickHouse `max_rows_to_read` per query | `50000000` |
| `QUERY_MAX_MEMORY_BYTES` | analysis: ClickHouse `max_memory_usage` per query | unset: an equal share of the server's memory limit per possible concurrent query (`CLICKHOUSE_MAX_QUERIES` + 1 background), at most 1 GiB and at least 32 MiB |
| `CLICKHOUSE_MAX_QUERIES` | analysis: how many queries interactive requests run at once (their connection pool); more wait, and a request that can't get a connection in time answers 503 "busy; retry". The statistics refresh has its own pool and a lower ClickHouse priority | `4` |
| `CLICKHOUSE_ASYNC_INSERT` | receiver: `off` writes one part per request; otherwise ClickHouse collects concurrent inserts into one part (`async_insert`) and still confirms each only after it is written | on |
| `MAX_SEARCH_WINDOW` | analysis: widest allowed `time_from`..`time_to` | `96h` (the 3-day retention plus a day) |
| `TRACE_RETENTION`, `LOG_RETENTION` | receiver: raw retention of spans and logs (Go duration or `7d`), applied to existing tables on startup; it must be longer than 1 h bucket + allowed lateness + 1 h rebuild margin, or the receiver refuses to start | `72h` |
| `STATS_RETENTION` | receiver: retention of statistics, coverage records and saved comparisons | `720h` (30 days) |
| `STATS_ALLOWED_LATENESS` | receiver and analysis (set both the same): how long after a bucket ends a record may still arrive and count in it | `24h` |
| `STATS_REFRESH_INTERVAL`, `STATS_REFRESH` | analysis: how often completed buckets are rebuilt; `off` disables the refresher | `1m`, on |
| `SAMPLING_POLICY_DEFAULT` | analysis: sampling policy of spans that declare none (`unsampled`, `head:0.2`); empty = unknown, which withholds change signals | empty |
| `TEST_CLOCK_CONTROL` | analysis: `1` enables the test clock and failure controls of the regression fixture; never on a real installation | off |

How these limits keep one ClickHouse server responsive, and what they don't cover, is in [Keeping ClickHouse available](#keeping-clickhouse-available).

The API reads the retention ClickHouse really applies from the table definitions (`GET /api/v1/retention`), so report warnings and statistics can't disagree with it. Expired data is removed by ClickHouse in the background, whole parts at a time; a longer retention can't restore what was already deleted.

A query that hits a limit, or a request that hits its deadline (2 × `QUERY_TIMEOUT` + 2 s), fails with HTTP 504 and a message naming the limit, and an unreachable ClickHouse gives HTTP 503 ("the database is unavailable"); the API never returns a partial result as if it were complete. A request the browser abandons cancels its queries.

Keeping ClickHouse available
------
Troubleshooting runs on one ClickHouse server. The design goal is that no single query, background job or burst of data can take that server down or make the UI wait behind it, and that an outage is reported as an outage, never as missing data. What it does, and what it doesn't:

**Every query is bounded.**
* Every span, log and report request names a half-open time window of at most `MAX_SEARCH_WINDOW` (96 h); a request without one is rejected (400) before any query runs. Raw tables are sorted by start time and partitioned by day, so ClickHouse reads only the days in the window.
* Every query carries a time limit (`QUERY_TIMEOUT`), a row limit (`QUERY_MAX_ROWS_TO_READ`) and a memory limit. Exceeding one is an explicit error (504 with the limit named), never a silently partial result.
* The memory limit is a share of the server's: the server's `max_server_memory_usage` divided by every query that may run at once (`CLICKHOUSE_MAX_QUERIES` interactive plus one background), at most 1 GiB. So even all queries at their limits together stay under the server's limit, and one heavy query can't take the memory the others need. A query that still meets the server's total limit is answered 503 "the database server is short of memory right now; retry", not "narrow the window".
* Pages of Log Analysis read the next rows in time order and stop; they never sort the whole window. When the browser cancels a request, its query is cancelled in ClickHouse too.

**People don't wait behind background work.**
* Statistics, release comparisons, problems and the sequence library read hourly aggregates (`stat_*` tables) that a background refresh builds one hour at a time, so screens don't scan raw spans for their numbers.
* Interactive requests and the refresh use separate connection pools (`CLICKHOUSE_MAX_QUERIES`, default 4, and 2), so the refresh can't take the connections requests need.
* Requests run at ClickHouse priority 1, the refresh at 10: while a request runs, ClickHouse pauses refresh queries instead of making the request wait.
* More concurrent requests than the pool wait for a connection; one that can't get it in time is answered 503 "every database connection was busy; retry".

**Writes stay cheap.**
* The receiver inserts with ClickHouse async inserts (`CLICKHOUSE_ASYNC_INSERT`): concurrent requests are written as one part instead of one part each, which saves the merges that cost memory and CPU under a steady stream. Each insert is still confirmed only after its rows are written.
* Expired data leaves as whole daily parts (`ttl_only_drop_parts`), which costs almost nothing; retention changes are applied only when they differ.
* `clickhouse/low-resources.xml` caps the server's memory (40 % of RAM) and its caches, and turns off internal logs and polling threads a small installation doesn't need. Background merge pools stay at their defaults: smaller ones make the server crash-loop.

**An outage is visible, not silent.**
* If ClickHouse can't be reached, the API answers 503 "the database is unavailable", and a report keeps the section that did load and marks the other unavailable, never empty.
* The receiver answers a failed log insert and a failed OTLP insert with `Unavailable`, which senders treat as retryable: log-sender's log writer keeps unconfirmed records (up to 10 000, the oldest dropped and counted) and sends them again, OTLP exporters retry. Spans sent through log-sender are not retried: its exporter reports the failure to the OpenTelemetry SDK, which drops that batch. A retried record is stored again under the same id and counted once.
* The receiver writes a heartbeat every 15 s; minutes without one are shown as collection gaps in statistics, so a gap is never read as zero traffic.

**Checked in CI.** `make fixture-overload` keeps the test ClickHouse busy with heavy low-priority queries while light Log Analysis requests must still answer within 3 × their unloaded p95 + 250 ms, and sends a burst of three times the connection pool, which must get results or "busy; retry", never another failure. The regression fixture also stops and restarts ClickHouse (explicit errors, then recovery) and recreates it with its volume.

**Not covered: replication.** All tables are plain `MergeTree` on one server. If that server or its disk fails, Troubleshooting is unavailable until it is back, and data written since the last backup is lost. High availability would need replicated tables (`ReplicatedMergeTree`), three ClickHouse Keeper nodes, at least two ClickHouse replicas behind the receiver and the API, and table creation on the cluster (the receiver creates tables today). Sharding (`Distributed` tables) is a further step for volumes one server can't hold. Until then, back up the ClickHouse volume if its history matters.

Security
------
By default every port of the stack is published on `127.0.0.1` only: the UI (8095), the receiver (50055, 4317), the API (8094) and ClickHouse. Applications on the same machine send to `127.0.0.1:4317`; containers join the `troubleshooting_network` and send to `log-receiver-compose:4317` (on Docker Desktop, `host.docker.internal:4317` works too). There is no authentication.

To receive spans from other machines or open the UI to them, set `TROUBLESHOOTING_BIND=0.0.0.0` (or one address of the machine). This publishes the receiver and the UI on it; ClickHouse and the API stay on loopback. Anyone who reaches those ports can then write spans and read them, so turn on:

* Basic auth for the UI and API: `UI_AUTH=basic`, `UI_USER=<name>`, `UI_PASSWORD_HASH=<hash>` where the hash comes from `docker run --rm caddy:2 caddy hash-password --plaintext '<password>'`. In `.env`, wrap the hash in single quotes because it contains `$`.
* A shared token for the receiver: `TROUBLESHOOTING_TOKEN=<random string>` for the receiver and every application that sends traces. The token travels unencrypted (no TLS), so it keeps out stray senders, not someone who can watch the network.
* A password for ClickHouse: `CLICKHOUSE_PASSWORD`.

Span API
------
`POST /api/v1/view-spans` returns `{"Spans": [...], "HasMore": bool}`; `POST /api/v1/count-spans` returns `{"Count": n}` for the same filter. Body (JSON):

| Field | Meaning |
| --- | --- |
| `time_from`, `time_to` | Required. Half-open window `[time_from, time_to)`, Unix nanoseconds as strings: a span starting exactly at `time_to` is not in it. Lists, counts and child counts only see spans that start inside it. |
| `scope` | `root` (root spans) or `all` (any depth). Default: `root`, or `all` with `trace_id`. |
| `trace_id` | 32 hex digits: only this trace. |
| `parent_id` | 16 hex digits: children of this span. Needs `trace_id`, because span ids are unique only within a trace. |
| `status`, `service_name`, `method_name` | `unset`/`error`/`ok`; case-insensitive substrings of the service and span name. |
| `min_duration_ms` | Spans that lasted at least this many milliseconds (fractions allowed; 0 or absent: any duration). A span without a valid duration never matches. |
| `service_names`, `span_names` | Exact picks (at most 500 each): a span matches if its service is one of `service_names`, or its `[service, span name]` pair is one of `span_names`, so a name picked under one service doesn't match it in another. Both empty: any. |
| `rows_per_page` | Default 25, at most 1000. `HasMore` says whether more rows follow (read with one extra row, not guessed). |
| `after_time`, `after_trace_id`, `after_span_id` | Cursor: the last row of the previous page; all three or none. Rows are ordered by (start time, trace id, span id). |

`POST /api/v1/span-names` with `{"time_from", "time_to"}` (the same window rules) lists the window's services and span names for picking: `{"Names": {"status", "names": [{"service", "name", "spans", "entry"}], "truncated", "plan", "notices"}}`. Whole hours with committed statistics are read from the hourly aggregates (`stat_calls`, entry operations from `stat_shapes`), the rest of the window (the current hour, partial edge hours) from raw spans, so a long window doesn't scan raw spans and no hour counts twice. `spans` is approximate (canonical calls from statistics, stored rows from raw spans); `entry` means the name started an execution (a root span); `status` is `partial` when some hours are no longer retained; at most 5000 names, sorted by service and name.

Rules every endpoint shares:

* A span is identified by (trace id, span id); its children by (trace id, parent span id).
* **Canonical record.** A span may be stored more than once (exporter retries, or a span sent twice). Reads pick one whole stored row per (trace id, span id) among its rows in the window, before any filter applies, so lists, trace views, child counts and totals agree and retries don't change them. For conflicting payloads the winner is, in order: error status; more events; more links; the larger hash of all other columns (independent of arrival order). Duplicates still exist in storage; only reads are deduplicated.
* Each span comes with `events` (`timeUnixNano`, `name`, `attributes`) and `links` (`traceId`, `spanId`, `traceState`, `attributes`), and `childSpanCount`: its children in the window.
* Paging is exact for data that doesn't change meanwhile; spans that arrive during paging may or may not appear (no snapshot). Refresh starts over.

Log and report API
------
`POST /api/v1/search-logs` returns `{"Logs": [...], "HasMore": bool}`. Body: `time_from`, `time_to` (required, `[from, to)`, Unix ns strings), optional `trace_id`, `service_name` and `message` (case-insensitive substrings, no wildcards), `severity` (exact, e.g. `ERROR`), `rows_per_page` (default 25, at most 1000) and the cursor `after_time` + `after_record_id`. A record is `timeUnixNano`, `recordId`, `traceId`, `spanId` (empty = not recorded), `severity`, `service`, `message`, `attributes`. Retries of one record (same `recordId`) are read once.

`POST /api/v1/trace-report` builds the report of one trace. Body: `trace_id`, `time_from`, `time_to` (required), `span_limit`, `log_limit` (default and maximum 1000), `sections` (`["spans"]` or `["logs"]` to retry one; default both). It answers `{"Report": {...}}` with HTTP 200 even when a source failed: each of `spans` and `logs` has `status` (`ok`, `unavailable`, `limit`, `error`), `error`, `records`, `shown`, `limit`, `truncated`, `total` (null if counting failed) and `totalStatus`; plus `warnings` (e.g. retention), `generatedAt` and `complete`.

Testing
------
```
make test             # Go unit tests of every module, UI type-check and unit tests
make ui-e2e           # browser checks of the UI against a mocked API (Playwright)
make test-stack-up    # isolated test stack (own project, ports and volume), then:
make fixture-load     # load a fresh regression fixture run
make fixture-check    # check it, incl. replay, database outage and container recreation
make test-stack-reset # delete the test stack and its volume (never the dev stack)
```
See [fixtures/regression/README.md](fixtures/regression/README.md) for the scenarios and expected answers. CI (`.github/workflows/ci.yml`) runs all of these.

Versions
------
Builds and CI use Go 1.27.0 (the modules still require only Go 1.21.5, so applications that import log-sender keep their own Go version), Node 20.18.0 (UI build), ClickHouse 26.9.13.15 and Caddy 2.8.4; the UI's dependencies are pinned by `log-ui/package-lock.json` (`npm ci`).

Statistics and release comparison
------
Call counts, error fractions, latency percentiles and error groups of an explicitly selected population, compared between two releases or windows, with examples that open the combined trace report. In the UI: the **Releases** page (release comparison and operation statistics).

**Population.** A recorded call is one canonical span of one service that started in a half-open window `[time_from, time_to)`. Operation (span name), span kind, environment, version, installation and workload segment are optional filters; a missing value is its own "not recorded" group and is never borrowed from another span. Without an operation the result has one row per (operation, span kind): SERVER, CONSUMER and CLIENT spans are never summed. Statistics read these attributes (resource or span attributes; on logs, record attributes): `service.version`, `deployment.environment.name`, `troubleshooting.installation`, `troubleshooting.workload_segment`, `troubleshooting.sampling_policy` (`unsampled` or `head:<ratio>`), and `operation.name` on logs. log-sender's `WithResourceAttributes` adds them to spans and logs.

**Metrics** (`metrics-v1`): calls, ERROR/OK/UNSET counts, recorded error fraction, rate per minute over the whole window, and p50/p95/p99 in milliseconds (ClickHouse t-digest, approximate) over valid durations. A span without a valid duration (no end, or end before start; stored as -1) counts as a call but not in percentiles. Percentiles need at least 20/200/1000 valid durations (a heuristic). A failed or expired query is unavailable, never 0.

**Error groups** (`errors-v1`): ERROR-status spans, exception events and ERROR/FATAL logs are separate source kinds and are never added up. Exceptions group by type and application frame; otherwise by the message with UUIDs, long hex ids and request/session/user/order/trace/span/correlation identifiers replaced (numbers such as 403 stay).

**Comparison** (`comparison-v1`): the user chooses baseline and candidate. A latency percentile is a regression or improvement signal when both sides meet its sample minimum and the change is at least 20 ms and 20 %; the error fraction when both sides have 1000 calls and it changes by at least 1 percentage point. Coverage gaps, unknown sampling, conflicting payloads or a population mismatch give "insufficient evidence" or "not comparable" instead. Each comparison is stored as a revision that can be reopened, regenerated (a new revision) and exported as HTML or JSON together with the combined reports the user included.

**Statistics storage.** The API rebuilds each completed UTC hour from canonical raw records and stores it as a new revision (`stat_calls`, `stat_errors`, committed by a row in `stat_revisions`); a failed rebuild leaves the previous revision. A record counts in its bucket only if the receiver got it before bucket end + `STATS_ALLOWED_LATENESS`; after that the bucket is final and never changes, and later arrivals are reported as excluded. Statistics outlive raw records (`STATS_RETENTION`). A query uses, per hour, either the stored revision or raw records, never both; bounds inside an hour need raw records, otherwise the API offers whole-hour bounds. Collection coverage comes from receiver heartbeats (every 15 s) and from declarations (`POST /api/v1/coverage`, e.g. for a known outage or backfilled history).

| Endpoint | Body → result |
| --- | --- |
| `POST /api/v1/stats/calls` | `{population, group_by_version, series}` → `{Stats}`: rows, time series, coverage, plan (stored or raw per hour), notices, cutoff |
| `POST /api/v1/stats/error-groups` | `{population}` → `{ErrorGroups}`, with a status per source (spans, logs) |
| `POST /api/v1/stats/examples` | `{population, kind: slow or error, threshold_ms, group, limit, cutoff}` → `{Examples}` (bounded; unavailable outside raw retention) |
| `POST /api/v1/stats/shapes` | `{time_from, time_to, service, operation, errors_only}` → `{Library}`: execution shapes (entry + distinct calls) with executions, failed share, p50/p95/p99 and representative executions; stored hourly, raw for hours without them (`docs/sequence-library.md`) |
| `POST /api/v1/compare` | `{baseline, candidate, regenerates}` → `{Analysis}` (stored) |
| `GET /api/v1/analyses`, `GET /api/v1/analyses/{id}` | saved comparisons, exactly as generated |
| `POST /api/v1/stats/rebuild` | `{time_from, time_to}` → outcome per bucket; rebuild or backfill now (idempotent) |
| `GET /api/v1/stats/status`, `GET /api/v1/retention` | refresher state and failures; effective retention |
| `POST /api/v1/coverage` | `{source, time_from, time_to, status: complete or gap, reason}` |

Population fields: `service` (required), `operation`, `span_kind`, `environment`, `version`, `installation`, `segment` (omitted = any, `""` = not recorded), `time_from`, `time_to`.

Problems, Releases and Settings
------
**Problems**: for one service and window it lists recurring error groups (headline: distinct affected calls, or matching records with "Call attribution unavailable" when the records name no span), slow calls (distinct calls above the slow-call duration) and release regressions (the p95 change of a stored comparison; no baseline is guessed from version text). A problem's **Investigate** tab has a bounded list of retained examples; the chosen one opens in Timeline & Sequence with its span and log selected. **Compare releases** compares releases of the problem's operation and checks a fix against the affected release and the original baseline. **Releases** holds the release comparison and the operation statistics. **Settings** holds the **detection rules** (three templates: error signals, slow calls, release regressions; every save is a revision, and a save based on an older revision is a conflict) and the effective **data retention** (read-only; set on the receiver). The Sequence view is described in `docs/sequence-investigation.md`; spans in `view-spans` and `trace-report` carry `spanKind`.

| Endpoint | Body → result |
| --- | --- |
| `POST /api/v1/problems` | `{service, environment, time_from, time_to, limit, cursor, rules_revision}` → `{Problems}`: status (available, empty, partial, unavailable, expired), sources, problems with their explanation, `next`, `waiting` |
| `GET /api/v1/problems/services?time_from&time_to` | services with recorded calls (a picker) |
| `GET /api/v1/detection-rules` | the latest revision, or the defaults as revision 0 |
| `POST /api/v1/detection-rules` | `{rules, expected_revision}` → `{Rules}` (the stored revision); 400 `{FieldErrors}`; 409 `{Rules}` with the current revision |

Feedback and help
------
Feature requests, bug reports and integration questions are welcome: [open an issue](https://github.com/agerimex/troubleshooting/issues). If you are trying Troubleshooting with your own service, say so in the issue, and I will help you set it up.
