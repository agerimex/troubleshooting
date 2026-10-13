# Product blueprint: the main UI

Source: `Troubleshooting_Product_Blueprint.zip` (troubleshooting-blueprint,
product-blueprint-v1.3, 12 October 2026). On the owner's request its pages
**replace** the UI's navigation and are now the main view. Branch
`feat/milestone-26-10` on top of `370762e` (main), uncommitted, together with
milestones 1–2, the problem workspace and the sequence page it builds on.

Implemented: the **Core A** slice (navigation, Log Analysis, Timeline &
Sequence) and the Core B/C placement of what already existed (Problems,
Releases, Settings). Contracts, Architecture and Extensions are later phases
and are not shown.

## Navigation

Five destinations, in this order: **Log Analysis** (default), **Timeline &
Sequence**, **Problems**, **Releases**, **Settings**. There is no Report or
Architecture tab. Pages stay mounted once visited, so returning to one keeps
its filters, page, expansion, selection and scroll position.

The URL hash names the page or the open execution:
`#/executions?trace=…&from=…&to=…&span=…&log=…&view=sequence`. It carries
identifiers only (no log text, no origin state). The legacy
`#/investigate` route and `view=timing` are accepted. An invalid link opens
the page with an explanation instead of guessing a trace. The browser's Back
returns to the previous page.

| Page | What it is | Built from |
| --- | --- | --- |
| Log Analysis | The existing span table (time, name, status, msg, service, duration, column choice, filters, cursor paging, total). New: expanding a span shows its child spans **and its own log rows inline**, in recorded time order; trace-level and unmatched logs are a separate group under the root span; row actions | `TracePage.vue` |
| Timeline & Sequence | The sequence library (distinct execution shapes with percentiles, opened as a sequence; `docs/sequence-library.md`, replaces the execution list on 13 October 2026), trace id lookup, and the focused viewer: Timeline (default) or Sequence, the log inspector, CLIENT/SERVER switch, events and links, completeness, per-section retry, regenerate, widen, investigation link, HTML/JSON export | `ExecutionsView.vue`, `ExecutionViewer.vue` |
| Problems | The problem list and detail; an example opens in Timeline & Sequence with its span and log selected; Compare releases and "Check fixed release" stay in the detail | `problems/ProblemsView.vue`, `ProblemDetail.vue` |
| Releases | Compare releases (with included reports and exports) and operation statistics | `ReleasesView.vue` (`CompareView`, `StatisticsView`) |
| Settings | Detection rules (versioned, optimistic concurrency) and data retention (read-only, from `GET retention`) | `SettingsView.vue` |

Row actions in Log Analysis: **Open timeline**, **Open sequence**, **Copy
investigation link**, and record details. The row's "⋯" button, right-click
and the keyboard (Enter on the button, arrows, Escape returns focus) open
the same list (`ActionMenu.vue`). If the clipboard is unavailable the link is
shown for selection, never reported as copied.

## Capability matrix

| Requirement | Existing before | Change | Remaining gap |
| --- | --- | --- | --- |
| Log Analysis first and preserved | Span search (`TracePage`) | Renamed, kept as is, default page | — |
| Logs inline in the same table | Logs only in the separate report | Expanding a span reads the trace's logs once per traversal (one `trace-report` by trace id and window) and attaches them by exact `(TraceId, SpanId)`. A loaded child has an arrow only if it has child spans or logs of its own, so the arrows show where the trace goes deeper; a list row (logs not yet read) keeps its arrow until expanded and loses it if nothing is under it | The report's log limit (1000) applies; a notice says when it was reached |
| Trace-level / unmatched logs in their own group | — | Group under the root span; reason "trace-level", "span not found", or "span not among the loaded spans" | Shown only under a root span (a child-only row has no group) |
| Filters select span metadata only | Yes (`view-spans`) | Labeled; no log search | — |
| Row actions (button, right-click, keyboard) | Report button only | `ActionMenu` | — |
| Return state | Pages were hidden while a report was open | Pages stay mounted; Back/return restores them | No "expired cursor" message: cursors don't expire |
| Narrow screens | — | Table reflows into fields below 640 px; filters stay | — |
| Timeline & Sequence page | Report view, problem investigation | One viewer for every origin | — |
| Exact lookup | Trace id field on the span search | On the page; independent of the list | — |
| Execution list | — | Entry (root) spans in a window, 50 per page, filters on the entry span's service, name and status, Previous/Next | Traces without a recorded root are not listed (open them by id); no catalogue generation, so rows ingested meanwhile can appear on later pages |
| Separate error indicators per row | — | Entry span status only, labeled; logs are not read for the list | No log-error count (would need prepared metadata) |
| Report identity, revision, link | Report generated on request | Identity = trace + window; revision = generation time; link regenerates | Revisions are not stored, so a link can't reopen the exact earlier revision; exports are the frozen copy |
| Timeline default, saved preference | Problem page chose per type | Explicit action > saved preference > Timeline | — |
| Origin with return | Back to search | "Back to …" with the origin label; origin is not part of the link | — |
| Settings: detection, retention | Rules inside the problem page | Settings → Detection; Data retention read from the server | Retention can't be changed from the UI (receiver env) |
| Design inputs, contracts, architecture findings | — | Not shown (later phases) | Later phases |
| Authorization of every id | Optional basic auth for the whole UI | Unchanged | No per-scope authorization |

## Scenario map

UI = Playwright with a mocked API (`log-ui/e2e/`), unit = vitest
(`log-ui/src/services/__tests__/navigation.spec.ts`).

| Scenario | Checked by |
| --- | --- |
| NAV-EXP-001, -002 | UI `navigation.e2e.ts`; unit (order) |
| NAV-EXP-003 | UI `navigation.e2e.ts` (log → Sequence), `report.e2e.ts` M1-RPT-005 (log → Timeline) |
| NAV-EXP-004 | UI (button, right-click, keyboard, Escape restores focus) |
| NAV-EXP-005, -015 | UI (filter, expansion and selection restored; no new query; browser Back) |
| NAV-EXP-006 | UI `problems.e2e.ts` PW-GRP-004; unit (no trace guessed); Log Analysis logs always come from a trace |
| NAV-EXP-007 | UI `problems.e2e.ts` PW-GRP-001 |
| NAV-EXP-008 | UI (link has ids only, opens in a fresh page without the origin); unit |
| NAV-EXP-009 | UI (`#/investigate`, `view=timing`); unit |
| NAV-EXP-010 | Existing query limits and RF13 (`tracePage.e2e.ts`); logs are read by trace id only |
| NAV-EXP-011, -012 | UI (inline logs, time order, group, no duration) |
| NAV-EXP-013, -014 | UI (OK filter keeps the ERROR log; no log query); child spans stay reachable through "Any depth" |
| NAV-EXP-016 | Children capped at 1000 with a notice (RF10); logs only for expanded spans |
| NAV-EXP-017 | UI (no page overflow at 360 px on every page); real-data screenshots at 360/320 |
| NAV-CAT-001…006, -009 | Replaced by the sequence library on the owner's request (`docs/sequence-library.md`): one row per execution shape instead of per trace; UI LIB-001…005 |
| NAV-CAT-007 | UI (lookup without the list) |
| NAV-CAT-008 | Report states (expired/unavailable sections, retention warning) |
| NAV-CAT-010 | UI LIB-003 (a failed library is an error, not an empty one) |
| RPT-DIR-001, -002, -003, -004, -012 | UI `navigation.e2e.ts` |
| RPT-DIR-005 | UI: problem (`problems.e2e.ts`), release comparison (`comparison.e2e.ts` M2-INV-001); architecture finding deferred |
| RPT-DIR-006 | UI (link opens without the origin); partial: regenerated, not a stored revision |
| RPT-DIR-007 | UI (same generation time across view and scope switches) |
| RPT-DIR-008 | UI `problems.e2e.ts` PW-INV-015, `report.e2e.ts` M1-EXP-* |
| RPT-DIR-009 | UI `report.e2e.ts` M1-EXP-003 |
| RPT-DIR-010 | Existing section states and retention warning; no stored report metadata |
| RPT-DIR-011 | Existing query limits (`limit` state, 504); no per-scope authorization |

The milestone 1 report scenarios (`report.e2e.ts`), problem workspace
(`problems.e2e.ts`) and sequence scenarios (`sequence.e2e.ts`) now run
against the shared viewer. Graph was replaced by Sequence (the blueprint's
COMPATIBILITY table), so PW checks of graph nodes now check timeline rows and
the sequence overlap. The global log search (M1-RPT-005) is revised as the
blueprint says: an ERROR log is found through Log Analysis rows or a problem
group's retained example.

## Results

- vitest: 83 pass (11 new in `navigation.spec.ts`).
- Playwright (mocked API): 86 pass (17 new in `navigation.e2e.ts`; the
  report, problems, sequence and comparison tests updated for the new
  navigation).
- Type-check clean; production build OK. No backend change.
- Real data (native receiver/API against the test ClickHouse, fixture run
  `f9823fd3`): Log Analysis showed the sequence trace's 12 logs inline under
  their spans; "Open sequence" on the storage ERROR log selected
  `upload_recording` with that log; return restored the table; a problem
  example opened in Timeline. No page overflow at 1024/736/360/320 px; no
  console errors.
- Fixed on the way: the Releases population form overflowed narrow screens
  (its rows sat side by side).
- **Not done**: the resource measurements and mixed read/write run the
  blueprint asks for, the pilot with engineers, the full fixture on a reset
  volume with new images.

## Deviations and leftovers

- No router: hash routes in `App.vue` (`services/navigation.ts`).
- The pages "Search logs", "Problems (new)" with Graph, and the old
  report page are gone from the UI. Their components (`LogSearch.vue`,
  `ReportView.vue`, `problems/TraceGraph.vue`) are unused and kept until the
  owner decides; the `search-logs` endpoint still exists.
- Log record details open in a dialog, not an inline details row.
- The Timeline marks a missing parent ("parent missing") without telling
  "not in window" from "not loaded"; the completeness section does.

## Rollback

UI only: no schema, API or data change. Reverting the `log-ui` files of this
step restores the previous navigation.
