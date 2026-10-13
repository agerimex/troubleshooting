# Regression fixture

A small, deterministic dataset and a checker for the trace-investigation
workflow (milestone plan `step1.md`, section 4) and the combined trace and
log report (`features/milestone_1`). It is a correctness fixture,
not a throughput benchmark.

## Usage

From the repository root, with Docker and Go 1.21+:

```
make images            # build the three service images
make test-stack-up     # isolated stack: own project, ports, network and volume
make fixture-load      # a fresh run: new base time and ids
make fixture-check     # all checks; writes results/report.md
make test-stack-reset  # removes the test stack and its volume only
```

The test stack (`docker-compose.test.yml`, project `troubleshooting-test`) uses
ports 29000 (ClickHouse), 24317 (OTLP), 20055 (log-sender), 28094 (API),
28096 (API with a 10-row read limit) and 28095 (UI), all on 127.0.0.1, and the
volume `troubleshooting-test-clickhouse-data`. Nothing here touches the
development stack in `docker-compose.yml` or its data.

`fixture-check` runs, in order:

1. RF05 before the late parent arrives (first check of a run only), then
   delivers that parent;
2. every scenario check, the log-sender/OTLP parity check, the invalid-id
   check and the query-limit check;
3. `-replay`: resends every request of the run twice, then all checks again;
4. `-outage`: stops the test ClickHouse, expects explicit errors, starts it,
   retries;
5. `-recreate`: recreates the ClickHouse container with its volume and runs
   all checks again;
6. `-retention`: the milestone 2 retention phase (moves the API's test clock,
   restarts the API, recreates the receiver with other retention). It runs
   last and resets the clock.

Each flag can be left out (`cd fixtures/regression && go run . check -h`).
The browser checks of the UI (RF11, RF14 and the UI side of the others) live
in `log-ui/e2e` and run with `make ui-e2e`; they use a mocked API.

## Run policy

- `load` starts a **fresh run**: a random 4-byte run id and a base time
  (now, truncated to the second). Both go into `.state/fixture.json`.
- Every trace id starts with the run id, every service name with
  `fx<run id>`, so checks select exactly the run's records; the API's own
  telemetry and earlier runs don't affect the expected answers.
- Scenario spans sit around `base - 10 min`; the default search window is
  `[base - 30 min, base + 5 min]`. RF06's parent is at `base - 50 min`.
- Replays rebuild the requests from the run id and base time, so a replay
  resends identical ids, timestamps, durations and attributes. A new `load`
  never reuses the ids of an earlier run.
- Spans are written by the receiver before it answers, so readiness is a
  bounded poll of the API (default 3 minutes), not a sleep.
- Tables keep 3 days (TTL): check a run within that time, or load a new one.
- Expected answers are written out in `checks.go`; none is computed with the
  API's SQL or filter code.

## Scenarios and expected answers

| ID | Data | Expected |
| --- | --- | --- |
| RF01 | Traces A and B, both with parent span id `…a0`; A has children `a.child1`, `a.child2`, B has `b.child1` | Expanding (A, `…a0`) returns A's two children, (B, `…a0`) B's one; child counts 2 and 1 |
| RF02 | The whole run resent twice | The run's logical total equals the number of distinct (trace, span) pairs generated; no identity appears twice in any page or across pages; every other check still passes |
| RF03 | One identity sent as an OK and an error version, in both orders (2 traces); another as versions `c`/`d` that differ only in attributes and duration, in both orders | The error version, whole (status, message, attribute, event), in list, trace and counts; status=ok finds 0. For `c`/`d` both traces resolve to the same version, with that version's own duration |
| RF04 | OK root `POST /recordings` → OK `upload pipeline` → failed `UploadToStorage` with an exception event | An error search at any depth finds exactly `UploadToStorage`; a root-only error search finds nothing; the trace has all three in a correct parent chain and the exact exception attributes |
| RF05 | Child `rf05.child` whose parent arrives after the first check | First: the child is searchable and its trace has only the child. After delivery: the trace has both and the parent has 1 child |
| RF06 | Parent at `base - 50 min`, child at `base - 10 min` | Default window: child only. Widened by 1 h on each side: both |
| RF07 | A failed span with an exception event (type, message, stack trace, `retry.count`) and a link to RF04's root | The event's time (to the nanosecond), name and attributes, and the link's trace id, span id and attributes come back exactly |
| RF08 | 8 roots: 7 at the same nanosecond, 5 of them with span id `…e8` in different traces, and one 1 ns later | Paging with 1, 2, 3 and 7 rows per page returns each of the 8 exactly once, ordered by (time, trace id, span id); count 8 |
| RF09 | Four levels | Each expansion returns exactly the next level; child counts 1, 1, 1, 0 |
| RF10 | One parent with 1000 children (the cap), one with 1001 | A 1000-row page: HasMore false for 1000, true for 1001; counts 1000 and 1001 |
| RF11 | (UI) list succeeds, count fails | Rows stay, "Total unavailable", never 0 or the previous total |
| RF12 | A child 2 min before its parent (clock skew), one 5 min after the parent ended (async) | Windows are `[from, to)`. `[skewed, async + 1 ns)`: both; `[skewed + 1 ns, async + 1 ns)`: async only; `[skewed, async)`: skewed only (a span at exactly `to` is out); `[parent start, parent end)`: none, and child count 0 |
| RF13 | Test ClickHouse stopped, then started | Search and count fail with an explicit 5xx error and no rows; after the restart the RF04 check passes. (UI side: error notice and Retry, in `log-ui/e2e`) |
| RF14 | (UI) slow response for an old filter | It cannot replace the newer result |
| RF15 | ClickHouse container recreated with its volume | All scenario checks pass again on the new container |
| RF16 | `TranscodeRecording`, 20 calls of `service.version` 1.0.0 (100–195 ms in 5 ms steps) and 20 of 2.0.0 (exactly twice as long) | Both populations of 20 with exactly those durations (µs) and versions; the report lists count, range and mean per version |
| PARITY | The same two spans (error status, exception event, link, typed attributes) sent through log-sender (`SendSpans`) and the OTel SDK's OTLP exporter | Same parent structure, status, message, duration (µs), attributes, events and links on both paths. `otel.scope.*` attributes are ignored: only the OTLP path stores the instrumentation scope |
| IDS | One valid span and three with a zero trace id, a zero span id and a 3-byte parent | 3 rejected (OTLP partial success), the valid one stored |
| LIMIT | Any-depth search over the run on the API with `QUERY_MAX_ROWS_TO_READ=10` | HTTP 504 naming the limit, no rows |

RF16 prepares data for the Q1 2027 comparison; it computes no percentiles.

## Milestone 1: combined trace and log report

Scenarios of `features/milestone_1` (fixture version 2). Spans go through
OTLP; log records go through log-sender's `LogExporter` (`SendLogs`, port
20055), the supported log path. Each record id is `fx<run>-<alias>`, with the
aliases of the feature files, so replays repeat them. Records sit around
`base - 8 min`, inside the default window. Each check runs in every phase
(initial, after replay, after recreation).

| ID | Data | Expected |
| --- | --- | --- |
| M1-RPT-001 | recording-A: OK `start_recording` (recording-api, `service.version` 1.4.0) → failed `upload_recording` (recording-worker, no version); logs `log-accepted` (request), `log-queued` (no span id), `log-denied` (upload, ERROR, "Storage returned 403 AccessDenied"), `log-upload-start`, `log-retry` (upload, WARN), sent out of time order | The report has exactly both spans (status, duration, version or none) and the 5 logs in recorded time order with their recorded span ids and severities, from both services; complete, totals 2 and 5 |
| M1-RPT-002 | (same) | An error span search of recording-worker at any depth finds `upload_recording`; its report has the OK root and INFO, WARN and ERROR logs |
| M1-RPT-005 | recording-ok: every span OK; `ok-unpublished` (ERROR, "Recording could not be published") and `ok-unlinked` (same text and time, no TraceId, upload's span id) | An ERROR search of recording-worker for "could not be published" finds both, without a TraceId; the report of `ok-unpublished`'s trace has `ok-accepted`, `ok-retry`, `ok-unpublished`, never `ok-unlinked` (RPT-003, RPT-006, COR-004) |
| M1-RPT-008 | The upload links to background-B, which shares `business.operation=recording-42` and has its own log | The link points at background-B's root; background-B's report has only its own span and log |
| M1-COR-001 | Two traces, each with span id `…e8`, one log each | Each report has only its own log |
| M1-COR-003 | Logs with no span id, with an absent span id `…ee`, and two identical events (same time, severity, text) with separate ids | All four, with their recorded span ids (REL-002) |
| M1-COR-006 | Two logs of a trace without spans | Spans: successful and empty; both logs |
| M1-CMP-003 | Traces with 3 and 4 spans and logs; report limits 3 | 3 shown, limit 3: complete at 3; truncated with total 4 at 4 |
| M1-CMP-004 | A parent (0–100 ms), an async child at 500 ms, a log at 300 ms | All three in the report |
| M1-CMP-005 | RF06's trace | Report in the default window: child only; widened by 1 h: both |
| M1-CMP-006 | A report window 80 h ago | A log retention warning |
| M1-REF-001 | (recording-A) | An ERROR search for "AccessDenied" finds `log-denied`; its report has the request and failed upload and the detail, which is in no span attribute or event |
| M1-REL-001 | All logs of the run, also after the double replay | Every confirmed record exactly once, across pages of 7 |
| M1-REL-003 | A child and a log referencing its parent, which arrives after the first report | First report: child only, log keeps its span id; after delivery: parent attached |
| M1-REF-003 | (with `-outage`) A log sent while ClickHouse is stopped | The sender gets an error, not a confirmation; the report is HTTP 200 with both sections unavailable (CMP-001). After the restart the same record is retried until confirmed and sent once more: the report has it once |
| M1-REF-002 | (with `-recreate`) | Every M1 check passes again on the new container |

The UI side of these scenarios (focus, labels, export, stale responses,
retry) is checked in `log-ui/e2e/report.e2e.ts`; see
`docs/milestone1-completion.md` for the scenario-by-scenario map.

## Milestone 2: statistics and release comparison

`m2.go` generates the reference regression of
`features/milestone_2/REFERENCE_FIXTURE.md` and small isolated cases, each
under its own service `fx<run>-<case>`. T is the whole UTC hour eight hours
before the base time, so none of it falls into the RF or M1 windows. Releases
of `recording-worker` / `upload_recording` (CONSUMER, environment `fixture`,
installation `reference-local`, segment `upload-small`, `unsampled`):
1.0.0 in `[T, T+1h)`, 1.1.0 in `[T+2h, T+3h)`, 1.1.1 in `[T+4h, T+5h)`, 1000
uploads each with the API parent, a storage CLIENT span and the logs of the
reference. Most small cases are in `[T+5h, T+6h)`. Load declares complete
collection coverage for `[T-3h, T+6h)` except a declared gap `[T-1h, T)`.
Expected values are literals from the feature files; percentiles use the
1 ms fixture tolerance.

| ID | Data | Expected |
| --- | --- | --- |
| M2-REF-001 (CMP-001) | 1.1.0 against 1.0.0 | 1000 calls each; p50/p95/p99 100 → 160 ms (+60 ms, +60 %); error fraction 1 → 5 % (+4 pp, +400 %); both regression signals; exception, span-status and log groups 10 → 50, recurring |
| M2-REF-001E | (same) | StorageError at storage.Upload has 50 occurrences in 50 traces; an example opens a report with the API parent, the upload, storage.PutObject and the 403 log, which no span records |
| M2-REF-002 (CMP-002) | 1.1.1 against 1.1.0 and 1.0.0 | 105 ms; −55 ms (−34.375 %) and −4 pp (−80 %) improvement signals; against 1.0.0 +5 ms and 0 pp within thresholds; the saved regression comparison is unchanged |
| M2-CAL-001 | 5 SERVER calls (2 ERROR, 2 OK, 1 UNSET, one with a parent) and 12 CLIENT spans | 5 calls, 40 %; rows per operation/kind, never summed |
| M2-CAL-002 | production 1.0.0 ×2, 1.1.0 ×3, no version ×1 (its trace has a 9.9.9 span of another service); test 1.0.0 ×4 | 2, 3 and 1 not recorded |
| M2-CAL-003 | Calls at S, S+60 s, S+120 s | `[S, S+120s)`: 2 calls, 1 per minute |
| M2-CAL-004 | Two traces with one span id | 2 calls, also after replays |
| M2-CAL-006 | 200 calls of a `head:0.2` source in the gap hour | 200 (not extrapolated), coverage gap, sampling notice, bucket not a confirmed value |
| M2-CAL-007 | One identity sent with 100 ms OK and 200 ms ERROR | 1 call, 1 conflict, notice |
| M2-CAL-008 | 100 calls in T-3h, none in T-2h, T-1h a gap | Series 100, 0, gap |
| M2-LAT-002 | 900 × 100, 80 × 300, 20 × 1000 ms | p50 100, p95 300, p99 1000 |
| M2-LAT-003 | 990 × 100 ms in T+4h, 10 × 1000 ms in T+5h, both stored | p95 100 from merged states |
| M2-LAT-005 | 1000 × 100 ms, one 0 ms, one ending before start, one without end | 1003 calls, 1001 valid, 2 excluded, 100 ms |
| M2-ERR-001 | StorageError in storage.Upload twice (other request ids and lines), once in auth.Authorize | Two exception groups: 2 and 1 |
| M2-ERR-002 | ERROR logs "Request request-A/B failed with status 403", "… request-C … 500" | Message groups 2 and 1; examples keep the original text |
| M2-ERR-003 | One OK call with 3 ERROR logs of one group | 3 occurrences, 1 trace; 0 of 1 calls failed |
| M2-ERR-004 | An ERROR log without trace, span or version next to 1.1.0 calls | Unlinked, version not recorded, not in 1.1.0 |
| M2-ERR-005 | Groups A 10 → 3, B 0 → 2, C 4 → 0 between versions b and c | recurring, newly observed, not observed |
| M2-ERR-006 | Two identical ERROR logs, one ERROR span with one exception | 2, 1 and 1, also after replays |
| M2-CMP-004 | 2000 calls in 1 h, 4000 in 2 h | +2000 calls, 33.33/min each, window notice |
| M2-CMP-006 | Candidate environment `test` | Mismatch; incomparable; statistics shown |
| M2-CMP-008 | publish_recording only in the baseline | Candidate 0 calls, metrics unavailable, note |
| M2-INV-001 | Slow 1.1.0 example | Report with the whole trace and INFO logs of both services |
| M2-INV-002 | m2-okerr log group example | Report: OK span, 3 ERROR + INFO logs |
| M2-INV-003 | 40 of 1000 calls take 900 ms | 5 of 40 shown, truncated; 1000 calls |
| M2-INV-004 | A log sent after the regression comparison | New report has it; saved comparison unchanged |
| M2-REP-002 | Reviewed comparison with 1000 candidate calls; 10 more arrive | Stored analysis keeps 1000 and comparison-v1; regenerated revision has 1010 |
| PW-RUL-001 (with RUL-002/004) | detection rules | a save of the read revision stores revision+1 (slow 700 ms, read back); the same expected revision again is 409; a negative duration is 400; each phase starts from the 500 ms the previous phase stored, so the rules survive restart and recreation |
| PW-GRP-002 (with GRP-003/004/005, INV-008) | pw-worker in [T+5h30m, T+6h): 3 ERROR logs of one span; 2 failing spans of one trace with an ERROR log each; an ERROR log without TraceId; the reference failed and log-only traces; 6 calls of 800–850 ms | 3 occurrences, 1 call; 2 calls, 1 trace; listed unqualified with "Call attribution unavailable"; AccessDenied 2 calls in 2 traces (one with all spans OK), separate from the ERROR-status uploads; 6 slow calls |
| PW-RUL-007 | recording-worker [T, T+6h) and pw-worker | the stored 1.0.0 → 1.1.0 comparison is a regression (p95 100 → 160 ms); the fix 1.1.1 is not; pw-worker without comparisons waits for comparable release data |
| SEQ-REAL-001 | seq-api, seq-storage, seq-thumbs at T+5h40m: the sequence package's base trace (publish SERVER, upload INTERNAL, PutObject CLIENT → PUT /recordings SERVER, GenerateThumbnail CLIENT → generate_thumbnail SERVER) and 12 logs | the report returns all six span kinds with their services, the server-side ERROR log on its own span and the trace-level log |

Retention phase (`-retention`), in order: RET-005 (a late call in a
provisional bucket: 100 → 101, its replay leaves it unchanged), RET-008 (a
forced commit failure keeps the previous revision and is reported; the retry
gives 101), M2-FINAL (clock +25 h: every fixture bucket final), RET-007
(stored and raw hours never counted twice: 150), RET-006 (API restarted; an
unseen and a replayed call for the final 1.0.0 bucket: still 1000, 1
excluded with a notice), RET-001 (clock +4 days: statistics and groups stay,
examples outside retention), RET-009 (`[T+10m, T+70m)` refused with
`[T, T+2h)` offered), RET-003 (clock +31 days: unavailable, no signal),
RET-004 (backfill twice: nothing revised, old hours unrecoverable), RET-002
(receiver recreated with 7/3/90 days twice, then restored: effective retention
shown, no duplicate statistics).

The UI side (formatting, examples, export, regenerate, unavailable states) is
checked in `log-ui/e2e/comparison.e2e.ts`; see `docs/milestone2-completion.md`.
