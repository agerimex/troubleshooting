# Milestone 1 completion record: combined trace and log report

> Since 12 October 2026 the UI follows the product blueprint (`docs/product-blueprint.md`): the span search is **Log Analysis**, the report is the **Timeline & Sequence** viewer, and the log search page is no longer in the navigation. The mapping below names the components as first built; the tests were updated with them.

Acceptance scope: `features/milestone_1/*.feature` (35 scenarios, copied
from `Milestone_1_Feature_Files.zip`, revision 2026-10-11). This records what
was implemented, where each scenario is checked, and what is still open. The
feature files are not executed by Cucumber: there are no step definitions.
Each scenario is checked by the tests named below, whose titles carry the
scenario tag.

Implementation: branch `feat/milestone-26-10`, on top of the step 1 work,
uncommitted at the time of writing. **Fill in the commit after committing.**

## What changed

- **Protocol** (`protos/logs.proto`): `LogMessageRequest` gained optional
  `record_id`, `trace_id`, `span_id`, `severity`, `service_name`,
  `attributes`; a batch RPC `SendLogs(Logs)`. Old senders keep working.
- **log-sender**: `LogExporter.Send` (confirmed delivery), `NewLogRecord`
  (correlation from the active span), zerolog `LogWriter` (background flush,
  keeps unconfirmed records, bounded buffer) and `TraceHook`.
- **log-receiver**: new `log_records` table (TraceId, SpanId, RecordId,
  Severity, ServiceName, Body, Attributes; 3-day TTL). Every log RPC answers
  only after the insert (finding F04 fixed). Invalid ids are rejected; missing
  ids stay empty. The old `logs` table is no longer written.
- **log-analysis**: `search-logs` and `trace-report`; one canonical row per
  RecordId; per-section status, limits, totals and retention warnings.
  `GET /api/v1/view-logs` was removed (it read the old table, and the UI no
  longer called it).
- **log-ui**: entry by span search or log search; `ReportView` with scope,
  completeness, warnings, span tree, linked traces, correlated logs, span
  focus, per-section retry, regenerate, widen, and HTML/JSON export.

## Results (local, 2026-10-11)

- Regression fixture on a clean test volume (`test-stack-reset` → `up` →
  `load` → `check` with replay, outage and recreation): **87 checks run,
  0 failed** (run `d2f03d0a`, fixture version 2).
- Mutation check: deduplicating log reads removed → 11 checks fail; trace
  filter on logs removed → 11 checks fail.
- Playwright (mocked API): 31 pass (12 span search, 19 report).
- vitest: 25 pass (10 span tree, 15 report logic and export).
- Go unit tests pass in protos, log-sender, log-receiver, log-analysis; the
  fixture vets. `vite build` succeeds.

## Scenario map

Fixture = `fixtures/regression` (real stack); UI = `log-ui/e2e/report.e2e.ts`
unless noted; unit = Go `*_test.go` or `log-ui/src/services/__tests__`.

| Scenario | Checked by |
| --- | --- |
| RPT-001 combine evidence | Fixture M1-RPT-001; UI M1-RPT-001/003/004 |
| RPT-002 failed child → report | Fixture M1-RPT-002; UI `tracePage.e2e.ts` (error search → report) |
| RPT-003 error log, all spans OK | Fixture M1-RPT-005 (all-OK trace with an ERROR log); UI M1-RPT-001/003/004 (span status vs log severity) |
| RPT-004 absent metadata | Fixture M1-RPT-001 (no version); UI M1-RPT-001/003/004; unit `serviceVersion` |
| RPT-005 error log → report | Fixture M1-RPT-005; UI M1-RPT-005 |
| RPT-006 log without TraceId | Fixture M1-RPT-005 (found, not in the report); UI M1-RPT-006 |
| RPT-007 span focus | Fixture M1-RPT-001 (order and span ids); UI M1-RPT-007; unit |
| RPT-008 single-trace boundary | Fixture M1-RPT-008; UI M1-RPT-008 / M1-COR-005; unit |
| COR-001 same SpanId | Fixture M1-COR-001; unit |
| COR-002 across services | Fixture M1-RPT-001 |
| COR-003 no / unknown span id | Fixture M1-COR-003; UI M1-RPT-007, M1-COR-006; unit |
| COR-004 no inference from SpanId/time | Fixture M1-RPT-005; receiver unit (`LogRecordFromProtoSpanWithoutTrace`) |
| COR-005 span events ≠ logs | UI M1-RPT-008 / M1-COR-005; unit (export); fixture M1-REF-001 |
| COR-006 logs without spans | Fixture M1-COR-006; UI M1-COR-006 |
| REF-001 reference investigation | Fixture M1-REF-001 (API path); UI M1-RPT-005 and export tests (UI path) |
| REF-002 container recreation | Fixture RF15, then every M1 check |
| REF-003 failed log delivery | Fixture RF13 (outage); log-sender unit; receiver batch unit |
| REL-001 replay | Fixture RF02 replay, then M1-REL-001 and every M1 check |
| REL-002 identical events | Fixture M1-COR-003 |
| REL-003 late evidence | Fixture M1-REL-003 (before and after) |
| REL-004 stale report | UI M1-REL-004 |
| REL-005 retry a source | UI M1-CMP-001 / M1-REL-005; unit `mergeSections` |
| CMP-001 one source fails | UI (both sources); Go unit; fixture RF13 (both sections unavailable, HTTP 200) |
| CMP-002 empty vs unavailable | UI M1-CMP-002; unit |
| CMP-003 limits | Fixture M1-CMP-003; Go unit; UI M1-CMP-003 |
| CMP-004 window, not parent duration | Fixture M1-CMP-004; UI M1-CMP-004/005 |
| CMP-005 missing parent, widen | Fixture M1-CMP-005; UI M1-CMP-004/005 |
| CMP-006 retention | Fixture M1-CMP-006 (warning); Go unit; UI M1-CMP-006 |
| CMP-007 timeout | Go unit; UI M1-CMP-007 |
| CMP-008 failed total | UI M1-CMP-008; Go unit |
| EXP-001 export HTML/JSON | UI M1-EXP-001; unit |
| EXP-002 incomplete export | UI M1-EXP-002; unit |
| EXP-003 export ≠ new query | UI M1-EXP-003 |
| EXP-004 HTML text is inert | UI M1-EXP-004 (opens the file, no dialog fires); unit |
| EXP-005 focused view export | UI M1-EXP-001 / M1-EXP-005 |

## Deviations and limitations

- **Windows are `[from, to)`**, as the feature README says, for spans, logs
  and reports (changed from step 1's inclusive windows on 2026-10-11). An
  empty window (`to <= from`) is rejected with HTTP 400. The UI's To time
  becomes the first nanosecond after the selected millisecond, so everything
  shown with that time is still included. Checked by RF12 (spans) and M1-WIN
  (logs) on the stack and `TestWindowIsHalfOpen`.
- **No Cucumber step definitions.** The tags map to tests (table above).
- **CMP-006**: the fixture checks the warning; it doesn't wait for real TTL
  expiry. Since milestone 2 the warning uses the retention read from the
  table definitions in ClickHouse (set on the receiver), so it can't drift
  from the real TTLs.
- **CMP-007**: a live timeout isn't reproduced on the stack; the timeout path
  is checked with unit and mocked-UI tests (as in step 1).
- **CMP-001 with one source down**: both sources live in one ClickHouse, so
  the real stack can only show both unavailable at once. The one-failed case
  is checked with unit and UI tests.
- **Log retries** store duplicate rows; reads deduplicate by RecordId. A
  sender that sends no record id (old senders) gets a random one per request,
  so its retries are not recognized.
- **Logs go through the custom `LogMessage`/`SendLogs` path**, not OTLP logs
  (the feature README allows one supported log path).
- The report loads at most 1000 spans and 1000 logs, and says when it is
  truncated.
- The report's "Version" is `service.version` from resource or span
  attributes.

## Open items

- Commit, push, first CI run (`.github/workflows/ci.yml` runs all of the above).
- The practical validation in `features/README.md`: another engineer runs a
  representative or replayed incident without being given the answer.
- A real application instrumented with `LogWriter` (only the fixture and unit
  tests send correlated logs so far).
