# Sequence investigation: an added page

> Since 12 October 2026 the UI follows the product blueprint (`docs/product-blueprint.md`): the Sequence view is part of **Timeline & Sequence**, used by every page; the separate "Sequence (new)" page is gone. The record below describes it as first added.

Source: `Troubleshooting_Sequence_Agent_Package.zip` (troubleshooting-sequence-agent:
UI_SPEC, SEQUENCE_SEMANTICS `sequence-semantics-v1`, DATA_AND_COST_CONTRACT,
COMPATIBILITY, 31 scenarios in three feature files, eight fixture scenarios,
reference model and preview), 12 October 2026.

The package proposes replacing the problem workspace's Graph with a Sequence.
To compare the two, it was added as a **second page**, "Sequence (new)",
instead: the same problem list, detection rules and release comparison as
"Problems (new)", but each example opens as **Sequence / Timing** (with the
same log inspector) instead of Graph / Timeline. "Problems (new)" is
unchanged. Uncommitted at the time of writing.

## What it shows

- **Sequence**: one column per recorded service (at most 6; the rest share an
  "other services" column), one row per mark in time order (at most 80 per
  page, Earlier/Later, "Locate selection"). Marks are operation starts, span
  completions ("Ended", never a response), call arrows and ERROR/FATAL log
  markers on the span that recorded them. Under 430 px it becomes a labeled
  list ("recording-api → object-storage · call PutObject").
- **Pairing**: a CLIENT span with exactly one directly parented SERVER span of
  another service is one arrow; both spans stay selectable through a
  CLIENT / SERVER switch above the inspector, each with its own logs. Several
  candidate servers stay separate. A CLIENT without a captured server points
  to its recorded peer (`peer.service`, `server.address`, …) shown as "not
  instrumented", or to "unknown peer".
- **Honesty rules** (unit-tested): INTERNAL spans stay local operations;
  missing parents are stated, never replaced; an unknown end has no
  completion marker; a log outside its span's interval stays visible with a
  note; repeated names are "repeat n of m", never retries; sibling overlaps
  come from recorded intervals (the package's 122 ms case), but between
  services whose server spans fall outside their client's interval (clock
  doubt) no cross-service overlap is claimed; links are listed as
  associations and never fetched.
- **Internal operations** checkbox hides local operations (presentation
  only; the selected span and log markers stay).
- **Timing** is the existing timeline. The view starts as Sequence for error
  problems and Timing for slow calls and regressions, then keeps the user's
  last choice per problem type (browser storage).
- **Exports**: the existing report exports, plus `presentation` in JSON
  (renderer version, view state, participants, operations, events,
  limitations, log associations) and an ordered event list in HTML.

## Changes

- **log-analysis**: spans in `view-spans` and `trace-report` now carry
  `spanKind` (it was stored but not returned). No other backend change.
- **log-ui**: `services/sequence.ts` (pure model, `buildSequence`, paging,
  lanes, `exportSequenceHtml`), `components/problems/SequenceView.vue`;
  `ProblemsView`/`ProblemDetail` take `variant: 'graph' | 'sequence'`;
  `LogInspector` can focus a picked log and shows the span kind; App entry
  "Sequence (new)" (both problem pages mount on first visit).
- **fixture**: `seq.go` sends the package's base trace (three services, two
  CLIENT/SERVER pairs, 12 logs) with the milestone 2 data; check
  SEQ-REAL-001 reads it back through `trace-report`.

## Results

- vitest: 72 pass (17 new: pairing, ambiguity, local operations, unobserved
  peers, completion markers, OK spans with ERROR logs, outliving children,
  unknown ends, logs outside spans, repeats, determinism, the 122 ms overlap,
  links, missing parents, clock doubt, paging, hiding).
- Playwright (mocked API): 71 pass (11 new SEQ-* tests on the package's
  scenarios).
- Go tests pass; the fixture builds and vets.
- Real data (12 October 2026, native receiver and API against the test
  ClickHouse, run `f9823fd3`, statistics rebuilt): `fixture check` 62 run,
  0 failed, SEQ-REAL-001 passes; a browser walkthrough of the page showed
  3 participants, "4 operations / 6 spans", 2 call arrows, the 122 ms
  overlap, the server-side ERROR log selecting `PUT /recordings`, and the
  narrow list.
- **Not run yet**: the timing/profiling gates and the five-engineer usability
  comparison the package asks for.

## Scenario map

UI = `log-ui/e2e/sequence.e2e.ts`; unit = `log-ui/src/services/__tests__/sequence.spec.ts`.

| Scenario | Checked by |
| --- | --- |
| SEQ-GEN-001 pair | unit; UI |
| SEQ-GEN-002 ambiguous pair | unit |
| SEQ-GEN-003 local operations | unit |
| SEQ-GEN-004 completion, no response | unit; UI (no "200 OK", success or retry text) |
| SEQ-GEN-005 122 ms overlap, no join | unit; UI |
| SEQ-GEN-006 nesting is not overlap | unit (siblings only) |
| SEQ-GEN-007 links stay associations | unit |
| SEQ-GEN-008 child outlives parent | unit (timeline) |
| SEQ-GEN-009 unknown end, clocks, ties, log outside span | unit |
| SEQ-GEN-010 missing parent | unit; UI |
| SEQ-GEN-011 repeats, not retries | unit |
| SEQ-GEN-012 deterministic, source unchanged | unit |
| SEQ-UI-001 enter through an error group | UI |
| SEQ-UI-002 ERROR log on an OK span | UI |
| SEQ-UI-003 either endpoint | UI |
| SEQ-UI-004 switch without a request | UI |
| SEQ-UI-005 trace-level and orphan logs | UI |
| SEQ-UI-006 unlinked log not attached | Problems page (PW-GRP-004): such logs open no trace |
| SEQ-UI-007 task events | existing task lanes (by `task.id`) |
| SEQ-UI-008 source states | UI (unavailable); empty/partial as on the problems page |
| SEQ-UI-009 stale response | UI |
| SEQ-UI-010 fix verification | shared Compare releases tab (PW-CMP-002) |
| SEQ-UI-011 narrow screen | UI (360 px list, no page overflow) |
| SEQ-RES-001 render budget | unit; UI |
| SEQ-RES-002 server limit ≠ render limit | model limitation text; pager says render limit |
| SEQ-RES-003 expired raw evidence | existing examples state (outside retention) |
| SEQ-RES-004 no ingestion work | by construction: built in the browser on open |
| SEQ-RES-005 bounded detail queries | the existing trace-report (trace-anchored, query limits) |
| SEQ-RES-006/007 exports | UI |
| SEQ-RES-008 pairing doesn't change statistics | by construction: statistics are not touched |

## Deviations

- A separate page instead of replacing Graph, so both can be tried.
- Timestamp quality isn't recorded by the system; clock doubt is inferred
  only from server spans outside their client's interval.
- No deep links (the app has no router); selection state lives in the page.
- Light theme of the reference not ported; the app's dark theme is used.
- The "Task events" chips of the reference are the existing task lanes.
