# Sequence library

The owner's request (13 October 2026): Timeline & Sequence opened with a
trace id field and a search form, which was too complicated. Instead it
should show a table, ten rows per page, of the **distinct sequences** —
each row one unique sequence diagram — with how many calls ran in the
period and their percentiles, filters to find a sequence, and the option to
open the sequence of a shape (for example its p95 execution) rather than a
call found by hand. The aggregation runs in ClickHouse.

Decisions (asked and answered):

- **Same sequence** = the same entry operation and the same **set of
  distinct calls**, a call being caller (service, operation) → callee
  (service, operation, span kind) along the recorded parent links. Repeats
  and order don't split a shape (3 or 5 queries are one shape); a different
  set of calls does (an early failure that skips calls is its own shape).
- **Computed hourly** and stored, like the release statistics.
- **Percentiles** p50, p95, p99 (the release comparison's set and minimums:
  20 / 200 / 1000 valid durations).

Uncommitted, on `feat/milestone-26-10`.

## How it works

- **Contract** `shapes-v1` (`log-analysis/internal/data/shapes.go`). An
  execution is a trace whose root span starts in the window; its duration
  is the root span's. Spans are the canonical eligible ones the other
  statistics use (`statSpans`), read up to 15 minutes after the hour
  (`shapeLookahead`); a call starting later isn't part of the shape. A child
  whose parent wasn't recorded is a call from an unknown caller.
- **ClickHouse**: one `GROUP BY TraceId` collects each trace's spans as
  compact tuples (span id, parent id, hash of service+operation, kind);
  array functions turn them into the sorted distinct call hashes and a
  `ShapeId`; a second `GROUP BY` per hour and shape counts executions and
  failed ones (any span with ERROR status) and keeps the root durations as a
  t-digest state, a sample of 64 executions and the slowest one.
- **Storage**: `stat_shapes` (created by the receiver, statistics
  retention). The refresher writes it in the same revision as `stat_calls`
  and `stat_errors`, committed by the same `stat_revisions` row; the
  definitions now include `shapes-v1`, so every provisional hour is rebuilt
  once. A **final** hour committed before shapes existed keeps its numbers
  and only gets its shapes (outcome `extended`), while its raw spans are
  retained; older hours have none and the library says so.
- **Reading** (`POST /api/v1/stats/shapes`, body `time_from`, `time_to`,
  `service`, `operation`, `errors_only`): per hour the stored shapes, or
  shapes computed from raw spans for hours without them (the current hour,
  partial edges). Stored and raw states merge in one query; the p50/p95/p99
  example is the kept execution whose duration is closest to that
  percentile. At most 500 shapes, most executions first.
- **Refresh limits**: refresh queries get at least a minute and may spill a
  large grouping to disk (`max_bytes_before_external_group_by` = half the
  memory limit) instead of failing.

## The page

Timeline & Sequence opens on the library (`SequenceLibrary.vue`): window
(last 24 hours by default), a service contained in the shape, the entry
operation, "with failed executions". Columns: entry operation and services,
distinct calls, executions, failed share, p50, p95, p99, and buttons to open
the p50, p95, p99 or slowest execution as a **sequence** (a missing
percentile says how many executions it needs). Ten rows per page. "Calls ▾"
lists the shape's calls, read on demand from one of its executions (one
trace-anchored report). Opening a sequence shows where it came from
("the sequence library (POST /api/orders · p95 · 1115 ms)"); Back returns to
the library on the same page with its filters. Opening by trace id moved
into "Open one execution by trace id". The former execution list (one row
per root span) is gone.

## Results (13 October 2026, test stack, 33 M spans of load data)

- 24 hours of stored shapes: 102 shapes in ~0.2 s. 20 minutes from raw
  spans: ~2 s.
- Storing an hour's shapes: ~1–3 s for ~1.1 M spans; the busiest hour
  (4.3 M spans) 14 s and 634 MB, spilling to disk.
- The failure paths of the load test appear as their own shapes (bank
  timeouts: 35 k executions, all failed, ~3 s).
- Checks: Go unit tests (`shapes_test.go`), vitest (`shapes.spec.ts`),
  Playwright LIB-001…005 and the rewritten lookup tests
  (`navigation.e2e.ts`), regression fixture **SEQ-SHAPE-001** (the sequence
  trace is one execution of its own shape: 5 calls, 3 services, failed, raw
  and stored, before and after a replay).

## Limits

- Shapes don't include versions or environments; filter by service or
  operation.
- An execution whose calls start more than 15 minutes after its hour ends is
  shaped without them.
- The examples are a sample of 64 executions per shape and hour, so a p99
  example is the closest kept one, not necessarily the exact p99 execution.
- Hours summarized before shapes existed and whose raw spans expired have no
  shapes (stated on the page).
