# Problem workspace: an added page

> Since 12 October 2026 the UI follows the product blueprint (`docs/product-blueprint.md`): this page is the **Problems** page (its examples open in Timeline & Sequence, its rules are in Settings, Graph was replaced by Sequence). The record below describes it as first added.

Source: the `troubleshooting-ui-agent/` package (UI_SPEC, DATA_CONTRACT,
DELIVERY_AND_QA, three feature files with 38 scenarios, the reference HTML
and its fixture), 11 October 2026. It was added as one **new page**,
"Problems (new)", next to the existing span search, log search, statistics
and comparison pages, which are unchanged. The goal is to try it out and keep
what proves useful; it doesn't replace the milestone 1/2 entry points, and
the package's proposal to drop the global log search is not applied.

Implementation: branch `feat/milestone-26-10`, on top of milestones 1 and 2,
uncommitted at the time of writing.

## What it does

- **Problems**: for one service (required) and window, the problems the
  current detection rules find:
  - *error groups* from the prepared error statistics (errors-v1, one source
    kind each). Headline: distinct affected calls, i.e. distinct recorded
    `(TraceId, SpanId)`. A group whose records name no span is listed as
    "record-based" with "Call attribution unavailable" and never meets a call
    threshold.
  - *slow calls*: distinct canonical calls above the rule's duration per
    operation, span kind and environment.
  - *release regressions*: the p95 change of a stored comparison
    (`analyses`) that is eligible under comparison-v1 and meets both rule
    thresholds. Without such a comparison the list says "Waiting for
    comparable release data"; no baseline is inferred from version text.
- **Problem detail**: header, counts, "Why was this flagged?" (rule
  revision, observed values, threshold, grouping, scope, coverage), and two
  tabs that stay mounted:
  - *Investigate*: up to 20 retained examples (existing `stats/examples`).
    Each opens one combined report (existing `trace-report`) drawn as a
    **graph** (one node per span, recorded parent → child edges only,
    detached nodes for missing parents, sibling overlaps from recorded
    times, render limit 60 with "Show all") or a **timeline** (one axis
    from trace start, invalid durations shown without an end), next to a
    **log inspector**: task lanes by the `task.id` log attribute ("Task not
    recorded" otherwise), a log list, or "All trace logs" with each log's
    association. Switching views uses the loaded report; exports (HTML and
    JSON) contain the whole report.
  - *Compare releases*: baseline and candidate releases of the problem's
    operation (versions from `stats/calls`), compared with the existing
    `compare`; a regression problem opens its stored comparison. "Check
    fixed release" compares a chosen release with the affected one, then
    with the original baseline.
- **Detection rules**: three templates with the package's defaults (error
  signals 1 affected call; slow calls > 500 ms, at least 5; regressions p95
  +20 % and +20 ms). Every save is a revision in `detection_rules` (no TTL);
  a save names the revision it edited, and another save in between is a 409
  conflict that keeps the user's edits until they load the newer revision.
  Disabled templates keep their values unvalidated.

## Changes

- **log-receiver** (`schema.go`): `stat_errors` gains `Calls`
  (`uniqExactIf` state of `TraceId:SpanId`) and `HasCalls`; new table
  `detection_rules`. Both are added to existing installations.
- **log-analysis**: `groupedErrors` and the refresher record calls; error
  groups return `affectedCalls` (null when part of the window was summarized
  before this change). New `problems.go`, `rules.go`, handlers in
  `cmd/rest_api/problems.go`, routes `problems`, `problems/services`,
  `detection-rules` (GET, POST).
- **log-ui**: `components/problems/*` (ProblemsView, ProblemDetail,
  TraceGraph, TraceTimeline, LogInspector, ProblemCompare, DetectionRules),
  `services/problems.ts`, `api/problems.api.ts`, `logs-api/problems.impl.ts`,
  `types/problems.ts`; App entry "Problems (new)".
- **fixture** (`pw.go`): the package's reference traces (failed upload next
  to a parallel thumbnail, the same call with OK spans and an ERROR log, 12
  logs with task lanes) and small cases, sent with the milestone 2 data;
  checks PW-RUL-001, PW-GRP-002, PW-RUL-007 in every phase.

## Results

RESULTS

## Scenario map

UI = `log-ui/e2e/problems.e2e.ts` (mocked API); unit = `log-ui/src/services/__tests__/problems.spec.ts`;
Go = `log-analysis/internal/data/problems_test.go`; fixture = `fixtures/regression/pw.go` (real stack).

| Scenario | Checked by |
| --- | --- |
| PW-GRP-001 grouped overview | UI; walkthrough |
| PW-GRP-002 3 logs, 1 call, replay | Fixture PW-GRP-002 (every phase, after replay); Go |
| PW-GRP-003 calls vs traces | Fixture PW-GRP-002 |
| PW-GRP-004 unattributed errors | Fixture; Go; UI |
| PW-GRP-005 source-specific counts | Fixture; Go `TestProblemIdentity` |
| PW-GRP-006 version not in identity | Go `TestProblemIdentity` |
| PW-GRP-007 bounded pages | UI (cursor and rules revision); server page size 50 |
| PW-GRP-008 availability states | Go `TestListStatus`; UI |
| PW-RUL-001 durable revision | Fixture (each phase reads what the previous phase stored, across restart and container recreation); UI |
| PW-RUL-002 validation, disabled values | Go; unit; UI; fixture (400) |
| PW-RUL-003 failed save | UI |
| PW-RUL-004 conflict | Fixture (409); UI |
| PW-RUL-005 history not rewritten | Not implemented (see deviations) |
| PW-RUL-006 idempotent evaluation | Not implemented (see deviations) |
| PW-RUL-007 waiting for comparable releases | Fixture; Go |
| PW-CMP-001 baseline vs degraded | Fixture M2-REF-001 (values) and PW-RUL-007; UI; walkthrough |
| PW-CMP-002 fix vs both baselines | Fixture M2-REF-002; UI (request order); walkthrough |
| PW-CMP-003 withheld conclusions | Existing comparison (Go `TestInsufficientEvidence`, M2 UI) |
| PW-CMP-004 mixed conclusions | Existing comparison (Go `TestMixedResults`); each metric keeps its label |
| PW-CMP-005 zero baseline | Go `TestRegressionProblems` |
| PW-CMP-006 merged distributions | Fixture M2-LAT-003 |
| PW-CMP-007 example release stays | The example's report keeps its own version; candidate examples come from the existing drilldown |
| PW-CMP-008 export without recalculation | Existing comparison export (M2 UI) |
| PW-INV-001 complete context | UI; walkthrough |
| PW-INV-002 120 ms overlap | Unit; UI; walkthrough |
| PW-INV-003 no invented join | Unit |
| PW-INV-004 selection across views | UI |
| PW-INV-005 task lanes | Unit; UI; walkthrough |
| PW-INV-006 all trace logs | Unit; UI |
| PW-INV-007 shared SpanId | Unit; existing M1 fixture checks |
| PW-INV-008 ERROR log, OK spans | Fixture; unit; UI; walkthrough |
| PW-INV-009 stale result | UI |
| PW-INV-010 expired example | Existing `stats/examples` (outside_retention, M2-RET-001) shown as a warning |
| PW-INV-011 failed log source | UI |
| PW-INV-012 logs without spans | UI |
| PW-INV-013 limits honest | Graph render limit (unit); truncated sections shown (component) |
| PW-INV-014 recorded log time | Logs ordered by recorded time and record id (existing `sortedLogs`); every stored log has a time |
| PW-INV-015 frozen export | UI (JSON and HTML, inert text) |

## Deviations and limitations

- **Evaluated on request, not stored.** Problems are computed when the list
  loads, with the latest rule revision, over the selected window. There are
  no stored detections, so PW-RUL-005 (an old detection keeps its rule) and
  PW-RUL-006 (idempotent evaluation key) don't apply yet; the explanation
  names the revision used. The package's rolling 15-minute evaluation every
  minute is not implemented: thresholds apply to the selected window.
- **Call counts need new statistics.** Hours summarized before `Calls`
  existed report affected calls as unavailable (the group is listed as
  record-based) until they are rebuilt; final hours are never rebuilt.
- **Slow calls read raw spans** (bounded by service, window and the query
  limits) instead of a prepared summary; outside raw retention they are
  unavailable. Raw logs are never scanned for the list.
- **One rule set per installation**, not per service or environment.
  Concurrency is guarded inside one API process.
- **No routes**: the app has no router, so the problem, tab and selections
  live in component state (kept while switching pages, lost on reload).
- **Theme**: the app's dark theme with the reference layout, not the
  reference's light palette.
- **Task lanes** use the application attribute `task.id`; nothing sends it
  yet except the fixture.

## Open

- Try it with real services; decide which parts to keep.
- If kept: stored detections with an evaluation cadence, per-service rules,
  a prepared slow-call summary, and URL state.
