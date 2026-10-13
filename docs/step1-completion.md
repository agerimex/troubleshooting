# Step 1 completion record

Milestone: reliable trace investigation and a reproducible regression fixture
(`step1.md`, Q4 2026). This records what was implemented and verified, and
what is still open. It is not a claim that the milestone is complete: stages F
and G and the CI run are outstanding (see "Open items").

## Baseline and implementation

- Baseline commit: `370762e` (`main`, "fix: bugfix 26 09 (#8)"); the plan's
  source-inspected baseline `dfc9e3d` is an ancestor. Intervening changes were
  reviewed: they are the branch work the plan describes as baseline.
- Implementation: branch `feat/milestone-26-10`, uncommitted at the time of
  writing. **Fill in the commit after committing.**

## Implemented rules

**Identity.** A span is (TraceId, SpanId); children are (TraceId,
ParentSpanId). The API rejects `parent_id` without `trace_id`; UI node keys
are `traceId:spanId`; the cursor is (UnixTime, TraceId, SpanId).

**Canonical record.** Every read (lists, trace view, child lists, child
counts, totals) goes through one subquery that keeps one whole stored row per
(TraceId, SpanId) among the span's rows in the searched window, chosen
before filters apply. Winner, in order: error status; more events; more
links; the larger `cityHash64` of all non-identity columns. The rule doesn't
depend on arrival or storage order (RF03 checks both orders). Physical
duplicates stay in storage; only reads are deduplicated.

> Since 13 October 2026 a page of the span list compares a span's rows only
> within the page's time range (from its first to its last candidate), so
> paging no longer reads the rest of the window (7–9 s → 0.2–0.35 s per page
> on 33 M spans). Retries and replays repeat a span with its start time, so
> the result is unchanged (RF02/RF03 pass); copies of one span stored with
> different start times are no longer compared on a page. The total works
> the same way: rows at a start time no other row shares are counted
> directly, only shared start times are ranked (6 h of root spans: 5.7 s →
> 1.2 s; selective filters 0.1–0.4 s). Child counts too: copies that name
> the same parent count without ranking, only disputed children are ranked
> (116 → 47 ms per page). A page of 25 root spans over 6 h of 33 M spans now
> takes about 0.2 s in all.
> Log search pages and log totals use the same rules (1–5 s → 55–176 ms
> per page; an unfiltered 6 h page no longer runs out of memory).

**Windows.** Every request has a `time_from`/`time_to` window (inclusive in
step 1; `[from, to)` since milestone 1, see `milestone1-completion.md`; at most
`MAX_SEARCH_WINDOW`, default 96 h). Lists, counts, child counts and
expansions use the same window; the UI resolves it once per traversal and
shows it. The old "parent start minus one minute" heuristic is gone. The
trace view can be widened by 1 h on each side; a span whose parent is absent
says "Parent not found within the searched window" (or "not among the loaded
spans" when the trace was truncated).

**Search.** `scope=all` searches every depth; the UI switches to it when a
filter is entered. A trace opens by id or from a row.

**Evidence.** The API returns `events` and `links`; the UI's evidence dialog
shows event times, names and attributes, exception type/message/stack trace,
and link ids with "Open linked trace". No stack trace is synthesized, and
"no events" is not presented as "no error".

**Explicit states.** `HasMore` (one extra row) drives truncation and paging;
a failed count shows "Total unavailable" and is never 0 or stale; load errors
stay at the list or the node, with Retry; empty and failed results look
different.

## Query limits

| Setting | Default | Test stack |
| --- | --- | --- |
| `max_execution_time` (`QUERY_TIMEOUT`) | 10 s | 10 s |
| `max_rows_to_read` (`QUERY_MAX_ROWS_TO_READ`) | 50 000 000 | 50 000 000; 10 on the "limited" API |
| `max_memory_usage` (`QUERY_MAX_MEMORY_BYTES`) | 1 GiB | 1 GiB |
| overflow modes | `throw` | `throw` |
| request deadline | 2 × timeout + 2 s = 22 s | 22 s |
| widest window (`MAX_SEARCH_WINDOW`) | 96 h | 96 h |
| HTTP server | read header 5 s, read 10 s, write deadline + 5 s, idle 60 s | same |

A limit or deadline gives HTTP 504 naming the limit; an unreachable database
gives HTTP 503. No partial result is returned.

## Fixture

- Version 1. Base-time policy: each `make fixture-load` is a fresh run (random
  4-byte run id, base = load time truncated to the second), stored in
  `fixtures/regression/.state/fixture.json`; replays rebuild byte-identical
  requests from it. Records are selected by run id (trace ids) and
  `fx<run id>` (service names).
- Scenarios and expected answers: `fixtures/regression/README.md`.

### Results (local, 2026-10-10)

Environment: Windows 11, Docker Desktop 28.1.1, ClickHouse 26.9.13.15, Go
1.21.13 (fixture) / golang:1.21.13 (images), Node 20.18.0 (UI image), Node
22.18 + Playwright 1.64.0 / Chromium (local UI checks).

- Fixture, clean volume (`test-stack-reset` → `up` → `load` → `check`):
  **44 checks run, 0 failed** (run `e96b2599`): 16 initial (RF01–RF10, RF12, RF16, the RF05 before/after pair, PARITY, IDS, LIMIT), the double replay, 13 after the replay, RF13 outage, RF15 recreation, 12 after the recreation. RF11 and RF14 are UI checks (below).
- Notes from that run: RF03 tie resolved to the same version in both arrival
  orders; RF16 v1 20 calls 100 000–195 000 µs (mean 147 500), v2 20 calls
  200 000–390 000 µs (mean 295 000); the limited API answered "query limit
  exceeded: the query would read more than 10 rows".
- Mutation check (the fixture must fail on known bugs): no canonicalization →
  caught (duplicate identity in a page); child counts keyed by span id only →
  RF01, RF09, RF10, RF12 fail; expansion ignoring the trace id → RF01, RF09,
  RF10, RF12 fail; search limited to root spans (old F08) → RF02, RF04, RF05,
  RF06, PARITY fail.
- UI: 12 Playwright checks (mocked API) pass: error search at any depth →
  trace → exception and link; no-events wording; RF11 total unavailable and
  never stale; RF10 truncation under the node; child-load error with Retry;
  RF13 explicit error and Retry; empty vs failed; RF14 stale response;
  RF05/RF06 missing parent and widening; full cursor and stable window;
  trace-id validation. 10 vitest unit tests pass.
- Real-stack walkthrough (scripted Playwright against the test stack UI):
  error search for the RF04 service found `UploadToStorage`; its trace showed
  `POST /recordings > upload pipeline > UploadToStorage`; the evidence dialog
  showed `AccessDeniedException: access denied: s3://recordings/…` and the
  stack trace.
- Go unit tests pass in protos, log-sender, log-receiver, log-analysis; the
  fixture module vets.

### Fixes found while verifying

- The paginator disabled "Next" when a page wasn't full even though
  `HasMore` was true (caught by a UI check).
- A request that hit its deadline was reported as a raw 500 "i/o timeout";
  now 504 naming the limit. An unreachable database is now 503.
- A new ClickHouse container spent 4–10 minutes hashing its own binary under
  Docker Desktop (also seen on the dev stack's first start).
  `skip_binary_checksum_checks` in `clickhouse/low-resources.xml` brought a
  fresh test stack to healthy in about 15 s. The first queries on a fresh
  container can still take 20–40 s to plan; the fixture's readiness probe
  waits for them.

## Known limitations

- The canonical record is chosen among a span's rows **inside the window**.
  Conflicting versions with different start times on either side of a window
  edge could resolve differently in different windows. Exact retries are
  unaffected.
- Children and child counts are limited to the window: an async child that
  starts after the window ends is not counted. The window is always shown.
- The trace view loads at most 1000 spans (it says so when there are more).
- Paging is exact only for data that doesn't change meanwhile (no snapshot).
- Retries still add physical rows (storage growth); there is no storage-level
  deduplication.
- Not benchmarked. Broad searches (any depth, no filter, wide window) on a big
  table can hit the row/time limits; they then fail with 504 rather than
  return partial results.
- `service_name`/`method_name` are `ILIKE` substrings; `%` and `_` in the
  input act as wildcards.
- Only the OTLP path stores `otel.scope.*` attributes (log-sender doesn't send
  the instrumentation scope); otherwise the two trace paths agree.
- A live query **timeout** was observed (cold container) but is not a
  repeatable fixture check; the limit path is checked with the row limit, and
  the timeout mapping with unit tests and a mocked UI check.
- Some transitive dev dependencies print npm engine warnings on Node 20.
- Logs (`LogMessage`) remain experimental (F04); the UI no longer calls
  `view-logs`, which is capped at 1000 rows.

## Open items

- Commit, push, and record the first GitHub Actions run of
  `.github/workflows/ci.yml` (written, not yet run).
- A clean-checkout build at the recorded commit (DoD 1) — run after committing.
- Stage F: user/problem validation conversations.
- Stage G: an independent engineer follows the instructions; record help
  needed and usability problems.
- DoD 9: investigation comparison (current method vs this tool), labelled
  synthetic if no real incident is available.
- Actual hours: not measured for this implementation session.
