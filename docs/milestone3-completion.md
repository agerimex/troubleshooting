# Milestone 3 completion record: quality gate

Acceptance scope: `features/milestone_3/quality_gate.feature`; contract and
user documentation: `docs/quality-gate.md` (`quality-gate-v1`). As for
milestones 1 and 2 there are no Cucumber step definitions: each scenario is
checked by the tests named below.

Implementation: branch `feat/quality-review-stage-1`, committed as `94ef722`
(2026-11-17).

## Stage 1: what changed

- **log-analysis**: `POST /api/v1/quality/evaluate` (`cmd/rest_api/quality.go`)
  and `internal/data/quality.go` (validation, one bounded canonical span query
  per run, pure evaluator). No schema change, nothing stored.
- **quality-check** (new module): the `troubleshooting-check` CLI.
- **fixtures/regression**: `qg.go`, four reference runs at T+6h and checks
  QG-REF-001, QG-REF-002, QG-EVD-001, QG-VAL-001; the runs are part of the
  replay payload.
- Makefile `test-go` and CI run the new module's tests.

## Scenario map

| Scenario | Test | Layer |
|---|---|---|
| QG-REF-001 | `TestQualityReferenceCandidateFails`; fixture QG-REF-001 | Go unit; real ingestion (OTLP → receiver → ClickHouse → API) |
| QG-REF-002 | `TestQualityReferenceFixPasses`; fixture QG-REF-002 | Go unit; real ingestion |
| QG-EVD-001 too few / no executions | `TestQualityInsufficientEvidence`; fixture QG-EVD-001 | Go unit; real ingestion |
| QG-EVD-001 outside retention, query limit, too many spans | `TestQualityInsufficientEvidence` (a load problem makes every check insufficient with its reason); `loadRun` maps retention, `classify` and the span bound to those reasons | Go unit (the mapping itself is not exercised against ClickHouse) |
| QG-VAL-001 | `TestQualityValidate` (19 invalid cases); fixture QG-VAL-001 (HTTP 400); CLI `TestErrorsExitThree` | Go unit; real API; CLI |
| QG-LAT-001 | `TestQualityLatencyBoundary`, `TestNearestRank` | Go unit |
| QG-GATE-001 | `TestQualityGatePrecedence`; baseline never changes status in the REF tests | Go unit |
| QG-DEP-001 | `TestQualityForbiddenCall` | Go unit |
| QG-RUN-001 | `TestQualityInsufficientEvidence` (other run), `TestQualityWindow`; fixture `-replay` (QG-* pass after RF02 resent everything twice) | Go unit; real ingestion |
| QG-CLI-001 | `TestExitCodesFollowTheGate`, `TestRequestReportAndJSON`, `TestBaselineWindow`, `TestErrorsExitThree` | CLI with a fake API |

Orphaned spans (parent not recorded) are reported: `TestQualityOrphansReported`.

## Verification run (2026-11-17)

- `go test ./...` in log-analysis and quality-check, `go vet ./...` in
  fixtures/regression: pass.
- Real stack: test ClickHouse (docker-compose.test.yml) with the receiver and
  API built from this branch, run natively on the test ports; `go run . load`
  (run 9fc2b374), statistics rebuilt for the reused volume, then
  `go run . check -replay`: every check passed, QG-* before and after the
  replay.
- CLI against that API: candidate exit 1, fixed exit 0, few exit 2, output as
  in `docs/quality-gate.md`.

Not run: `make ui-e2e` (no UI change), `-outage`, `-recreate`, `-retention`
phases, the limited API.

## Open

- Stage 2 is recorded below. Not planned unless asked: imported requirement revisions with digests, run
  registry, stored evaluations, load and outage experiments, sync-call
  semantics.

# Stage 2: reports, Quality page, GitHub Action, own CI

Implementation: commit `932c1f9` (branch `feat/quality-review-stage-2`,
2026-11-17).

## What changed

- **log-analysis**: `rules_yaml` in `quality/evaluate` (`ParseQualityRules`,
  strict like the CLI; `gopkg.in/yaml.v3` is now a direct dependency);
  `TROUBLESHOOTING_RUN` tags the API's own spans with `troubleshooting.run`.
- **quality-check**: `--html` and `--markdown` (`reports.go`); `action.yml`,
  a composite GitHub Action.
- **log-ui**: the **Quality** page (`QualityView.vue`, `services/quality.ts`,
  `types/quality.ts`, `api/quality.api.ts`, `logs-api/quality.impl.ts`),
  between Releases and Settings; the blueprint test now expects six pages.
- **CI**: the `integration` job sets `TROUBLESHOOTING_RUN` (passed to the test
  stack's API by `docker-compose.test.yml`) and runs the action with
  `.github/quality.yaml` after the fixture.

## Scenario map

| Scenario | Test | Layer |
|---|---|---|
| QG-API-001 | `TestQualityRulesYAML` | Go unit |
| QG-OUT-001 | `TestReportsEscapeRecordedText`, `TestMarkdownNeverLinksScripts`, `TestReportFilesWritten`; UI `quality.spec.ts` (export) | CLI; Vitest |
| QG-UI-001 | `e2e/quality.e2e.ts` QG-UI-001; `quality.spec.ts` (labels, windows, evidence targets) | Playwright (mocked API); Vitest |
| QG-UI-002 | `e2e/quality.e2e.ts` QG-UI-002 | Playwright |
| QG-UI-003 | `e2e/quality.e2e.ts` QG-UI-003; `quality.spec.ts` (parseResult) | Playwright; Vitest |
| QG-UI-004 | `e2e/quality.e2e.ts` QG-UI-004 | Playwright |
| QG-CI-001 | the action's check and gate scripts, extracted from `action.yml` and run locally with `GITHUB_OUTPUT`/`GITHUB_STEP_SUMMARY` set: passed run, a deliberately failing rule (exit 1, one failing check in the summary), every `gate:fail-on` combination | manual run, 2026-11-17 (not yet in GitHub) |
| QG-CI-002 | CI job `integration`, step "Quality gate of the API's own requests"; locally: API with `TROUBLESHOOTING_RUN=local-1`, `go run . check`, then the action script: 6 checks passed (trace-report 12 queries over 23 requests, view-spans 5 over 64, stats/calls 3, compare 14, quality/evaluate 2) | real test stack; CI once pushed |

## Verification run (2026-11-17)

- `go test ./...` in log-analysis and quality-check; `npm run type-check`,
  `npx vitest run` (93 tests), `npx playwright test` (93 tests, including the
  360 px check with the Quality page): pass.
- Quality page screenshots at 1366 px and 375 px with the real candidate
  result: no horizontal scroll.
- Real stack as above. Not run: the action inside GitHub Actions (it runs on
  the first push of this branch), the fixture's outage/recreate/retention
  phases with the tagged API.

# Stage 3: trusted rules, provenance, PR comment, an application's CI

Implementation: commit `2c8b27b` on branch `feat/quality-review-stage-2`
(2026-11-17).

## What changed

- **log-analysis**: incomplete executions (`missingBelow`, `requireComplete`
  in `quality.go`): `db_calls` and `forbidden_call` are `insufficient`
  (`incomplete_executions`) instead of passing when spans below the entry may
  be missing.
- **quality-check**: `source` in the result (`--commit`, `--ref`, `--ci-run`,
  `--rules-ref`, rules sha256); evidence snapshots (`snapshot.go`,
  `--no-snapshot`); `troubleshooting-check comment` (`comment.go`); the action
  gets `rules-from`, `commit`, `ref`, `comment`, `github-token`; new
  `quality-check/stack/action.yml`.
- **examples/quality-gate**: an example application (new module, in the Go
  test matrix and `make test-go`).
- **log-ui**: the source line on the Quality page and in its HTML export.
- **CI**: the `integration` job uses `rules-from: base` and may comment
  (`pull-requests: write`); new job `quality-example`.

## Scenario map

| Scenario | Test | Layer |
|---|---|---|
| QG-INC-001 | `TestQualityIncompleteExecutions` | Go unit |
| QG-SRC-001 | `TestSourceRecorded`; UI `quality.spec.ts` (source), `e2e/quality.e2e.ts` QG-UI-003 | CLI; Vitest; Playwright |
| QG-CI-003 | the action's check script, extracted from `action.yml` and run against a local git origin: base rules used with a note when changed, no note for unchanged CRLF files, introduction without base rules, push, unknown ref = error | manual run, 2026-11-17 (GitHub on push) |
| QG-CI-004 | `TestCommentPostsThenUpdates`, `TestCommentSkips`, `TestCommentBodyFits` (fake GitHub API) | CLI |
| QG-CI-005 | CI job `quality-example`; locally: the stack script from `quality-check/stack/action.yml` (ClickHouse, receiver, API), the example's tests over OTLP, the CLI: good run passed (2 database calls, 41 executions), regression failed (9 calls, `legacy-pricing` in 40 of 41) | real services; CI once pushed |
| QG-OUT-002 | `TestEvidenceSnapshot`; a real `report.html` of the regression run | CLI; manual |

## Verification run (2026-11-17)

- `go test ./...` in protos, log-sender, log-receiver, log-analysis,
  quality-check and examples/quality-gate; `go vet` in fixtures/regression:
  pass. `npx vitest run` (94 tests), `npx playwright test` (93 tests),
  `npm run type-check`, `npm run build-only`: pass.
- The regression fixture's QG checks (QG-REF-001/002, QG-EVD-001,
  QG-VAL-001) against the new API on the test stack: pass.
- Not run: anything inside GitHub Actions (the stack action, the comment, the
  `quality-example` job and `rules-from: base` run on the first push).
- First run in GitHub (`main` after the merge, CI run 44079936899): the
  `quality-example` job passed (good run passed, the planted regression
  failed as required) and the API's own gate passed; the pull request run
  failed only on the flaky milestone 2 fixture, so the PR comment and
  `rules-from: base` on a pull request are still to be seen.

# After stage 3: reports without dead links (2026-11-17)

That first run showed that evidence links from a Troubleshooting started
inside the job point to a server that is gone after the job (and to the API
port, which serves no UI). `--ui none` (action input `ui: none`, the stack
action's `ui` output) leaves the links out: the text names the trace and
span, the Markdown says the spans are in report.html, and the HTML report
keeps its snapshots. Both CI jobs use it. The HTML report shows its windows
as UTC times instead of nanoseconds and keeps the value columns on one line.
Tests: `TestNoEvidenceLinks`, `TestWindowText`, the updated
`TestReportsEscapeRecordedText` strings. The site article
`site/articles/catch-n-plus-one-in-a-pull-request.md` uses that run's
`result.json`, rendered with these changes.

# Review fixes (2026-11-17)

Merged as `37d337d` (#14). A review of everything since `164cfec` found, and this fixes:

- **Siblings counted as missing spans.** A span under the same unrecorded
  parent as the entry (an uninstrumented gateway's other call) made a
  complete execution incomplete. `missingBelow` now skips the entry's
  siblings.
- **Executions past the spans read.** Spans are read up to the window's end
  plus 5 minutes; an execution running past that could pass with its last
  calls unread. It is now incomplete (`PastRead`), with its own detail and
  evidence.
- **Fields of another check type** (e.g. `max_ms` on `db_calls`) were
  accepted and never applied; they are now errors (`foreignFields`).
- **CLI and API rules limits differ.** The CLI now rejects a second YAML
  document and files over 64 KiB, like `ParseQualityRules`.
- **Snapshot at a query limit** read as an empty trace; it is now
  "unavailable: query limit".
- **Comments with a personal access token** were never found again, so every
  run added one; ours is now the token's own user's (`GET /user`), or the
  bot's for `GITHUB_TOKEN`.
- **The Markdown named report.html** when only `--json` was written; it now
  names the HTML report actually written, or nothing.
- **Speed and duplication:** run and baseline are read concurrently, evidence
  snapshots up to 4 at a time; one `postJSON` helper in the CLI,
  `RootParentSpanID` instead of a second constant, one Markdown URL escaper.

Tests: `TestQualityIncompleteExecutions` (sibling, past the read end),
`TestQualityForeignFields`, `TestQualityValidate` (three new cases),
`TestRulesOneDocument`, `TestSnapshotAtQueryLimit`,
`TestCommentWithPersonalToken`, `TestNoEvidenceLinks` (no HTML report).

# Stage 4: verdicts that stand on enough evidence; saved evidence in the UI

Implementation: branch `feat/quality-stage-4` after `37d337d`, merged as
`9d25d40` (#15, 2026-11-17). It answers the web agent's review of `main`
(missing telemetry turning a failure into a pass, CI identity, saved evidence
the UI couldn't show) and fixes the flaky milestone 2 fixture.

## What changed

- **Flaky fixture.** The API's refresher runs once at startup, which in CI is
  during the fixture load; an hour it reached before its records arrived kept
  a revision without them (a whole hour is read from its committed revision)
  until the next refresh, so the milestone 2 checks saw 0 calls in about half
  the runs. `load` now rebuilds `T-3h` up to the current hour once the data is
  visible (`rebuildLoaded`). Reproduced deterministically by starting the API
  before the load with a 10-minute interval: the old fixture had 16 failing
  lines, the new one none.
- **Lost executions.** `run.executions` in `quality.yaml` declares how many
  executions of each checked entry the tests make; it is every check's
  default minimum (latency at least 20), and fewer is `missing_executions`.
- **No database calls** in a whole run is `no_database_calls` unless the
  limit is 0.
- **Invalid durations** make a latency pass `invalid_durations` (all invalid:
  "none of the N executions has a valid duration"); a measured failure stands.
- **Provenance.** The action records the checked-out commit
  (`git rev-parse HEAD`: a pull request's merge commit) as `commit` and the
  head as `pr_head` ("commit …, merging head …"); the comment's stale check
  uses the head. CLI `--pr-head`.
- **Run ids.** Every CI example uses
  `ci-${{ github.run_id }}-${{ github.run_attempt }}`; a pull request number
  repeats on every push.
- **Saved evidence in the UI.** A result file's evidence carries its snapshot
  to the viewer (`ExecutionTarget.snapshot`); when the live report has no
  spans of the trace, the viewer shows `snapshotReport` with a "Saved snapshot
  from the CI result" banner, no completeness verdict, logs "not kept", and
  Timeline and Sequence. The Quality page shows "saved snapshot: N spans".
- The example declares `run.executions: 41`.

## Scenario map

| Scenario | Test | Layer |
|---|---|---|
| QG-SUF-001 | `TestQualityEvidenceSufficiency` (declared executions); real services: the example with `executions: 50` is unresolved, "executions were lost" | Go unit; manual |
| QG-SUF-002 | `TestQualityEvidenceSufficiency` (no database calls, limit 0) | Go unit |
| QG-SUF-003 | `TestQualityEvidenceSufficiency` (18 of 20 invalid, a measured failure, all invalid) | Go unit |
| QG-SRC-002 | `TestPRHeadRecorded`, UI `quality.spec.ts` (merging head); the action's wiring parsed from `action.yml`; real services: "commit …, merging head …" | CLI; Vitest; manual |
| QG-UI-005 | `e2e/quality.e2e.ts` QG-UI-005; `quality.spec.ts` (saved snapshots) | Playwright; Vitest |

## Verification run (2026-11-17)

- `go test ./...` in log-analysis, quality-check, examples/quality-gate;
  `go vet` in fixtures/regression: pass. `npx vitest run` (97 tests),
  `npx playwright test` (94 tests), `npm run type-check`, `npm run build`: pass.
- The full fixture on a fresh test volume with the new receiver and API: no
  failures (QG checks included).
- The example through the new CLI on real services: good run passed (41
  declared executions), the regression failed, 50 declared was unresolved.
- Not run in GitHub yet; the action's `git rev-parse HEAD` runs on the first
  push.

## Not done yet

- A branch that was never recorded at all can't be detected from spans; the
  declarations narrow it, the result never claims more than "not observed".
- Website: the quality gate became the main story (#16, `40bf01d`).

# Stage 5: requirements and their history

Implementation: branch `feat/quality-stage-5` after `40bf01d`, uncommitted at
the time of writing (2026-11-17). The web agent's step 3: named requirements
linked to checks, with an owner and a reason, and a saved history of results.

## What changed

- **Requirements in the rules file.** `requirements:` (id, title, owner, why,
  checks) and `project:` in `quality.yaml`; Git stays the authoring path. A
  requirement's status comes from its own checks: violated, insufficient, met,
  or not evaluated without checks (never met). Every result has `project` and
  `requirements`; the CLI's text, Markdown and HTML reports list them.
- **Recording.** `record: true` stores the evaluation (result, exact rules text
  and sha256, `source`, evidence snapshots from the spans already read) in
  `quality_evaluations` (receiver schema, no TTL); the id is derived from the
  inputs, so a retry records once. CLI `--record` (prints the link), action
  input `record` (auto: push events). The CLI now sends the rules file as text
  (`rules_yaml`) and skips snapshots the API already made.
- **History API.** `quality/projects`, `quality/evaluations` (paged by
  recorded time), `quality/evaluations/{id}`, `quality/requirements` (the
  catalogue: the latest evaluation's requirements with up to 30 history points
  and since when their status holds).
- **Quality page.** Tabs Requirements, History, Check a run; the catalogue
  with status, owner, reason, check statuses, streak text and history cells
  (rules changes marked); the history list; an opened evaluation with its
  rules and a link `#/quality?evaluation=<id>`. The result view is one
  component (`quality/QualityResult.vue`) for checked, file and recorded
  results.
- **Rules files.** The example, the example application and this repository's
  own `.github/quality.yaml` declare requirements; a Go test validates all
  three with the API.
- Found on the way: a `{name:String}` query parameter can't carry newlines
  (ClickHouse parses it as escaped text), so the record is a native batch
  insert. The older stored JSON (analyses, detection rules) goes through such
  parameters too; no stored row is damaged, but a body with an escaped quote
  would be (backlog).

## Scenario map

| Scenario | Test | Layer |
|---|---|---|
| QG-REQ-001 | `TestQualityRequirementStatuses`; fixture QG-HIST-001 (statuses of three runs) | Go unit; fixture |
| QG-REQ-002 | `TestQualityRequirementValidation`, `TestQualityRepositoryRulesFiles` | Go unit |
| QG-HIST-001 | fixture QG-HIST-001; `TestQualityCatalogue`, `TestQualityRecordSnapshots` | fixture; Go unit |
| QG-HIST-002 | fixture QG-HIST-001 (retry, and every later `check`); `TestQualityEvaluationID` | fixture; Go unit |
| QG-HIST-003 | `TestRequirementsAndRecord`, `TestRecordErrorWarns`; the action's `record` input | CLI; manual |
| QG-UI-006 | `e2e/qualityHistory.e2e.ts` QG-UI-006; `qualityHistory.spec.ts` | Playwright; Vitest |
| QG-UI-007 | `e2e/qualityHistory.e2e.ts` QG-UI-007; `qualityHistory.spec.ts` (routes) | Playwright; Vitest |
| QG-UI-008 | `e2e/qualityHistory.e2e.ts` QG-UI-008 | Playwright |

## Verification run (2026-11-17)

- `go test ./...` in log-analysis, log-receiver, quality-check; `go vet` in
  fixtures/regression: pass. `npx vitest run` (102 tests), `npx playwright
  test` (97 tests), `npm run type-check`: pass.
- The fixture on the test ClickHouse with this branch's receiver and API
  (native): 66 checks pass, QG-HIST-001 included, and again on two more
  `check` runs (recording stays idempotent).
- The CLI with `--record` against the same API: recorded, link printed; the
  link opens the evaluation in the UI, its evidence opens in Timeline &
  Sequence and Back returns.

## Not done yet

- Requirement revisions with digests and a diff between them, kinds and
  support relations, catalogue filters, deleting records from the UI.
- Validation on one external application (the web agent's step 4).

# Stage 6: verdicts the data can carry

Implementation: branch `feat/quality-stage-6` after `ede9d2e`, uncommitted at
the time of writing (2026-11-17). It answers the web agent's review of
`ede9d2e`: lost child spans could still turn a failure into a pass, a retry
could answer a new verdict under a recorded id, and a snapshot could drop the
offending span.

## What changed

- **Lost child spans.** The quality query reads `ChildSpanCount` (recorded by
  log-sender; 0 for OTLP). A span that started more children in its own
  process than were recorded below it (same service) makes its execution
  incomplete, so `db_calls` and `forbidden_call` can't pass on it.
- **`min_per_execution`** on `db_calls`: an execution with fewer calls is
  `missing_database_calls`, not a pass.
- **Assumptions.** Every result has `assumptions`: verdicts hold for the
  recorded spans (instrumented, not sampled, exported), which losses are
  caught, and whether this data can show lost child spans. Shown in the CLI's
  text, Markdown and HTML reports and on the Quality page; the website and
  handbook say "lost executions can't pass", not "lost spans", and "which
  commit first passed its checks again", not "which commit fixed it".
- **One id, one verdict.** A retried record answers the stored evaluation
  (`recorded_before`), not the fresh result; `reevaluate` (CLI
  `--reevaluate`) records a re-evaluation under a new id (`reevaluated`).
  Recording goes through a small store interface, so this is unit-tested.
- **Snapshots keep the counterexample.** `SnapshotSelection` (API) and its
  copy in the CLI keep the evidence span and its ancestors, a `db_calls`
  check's database calls, then the spans below the evidence span, then the
  rest, at most 200; the CLI reads up to 1 000 spans to choose from.
- Docs: a stale sentence said a run without database calls passes; it has been
  `no_database_calls` since stage 4.

## Scenario map

| Scenario | Test | Layer |
|---|---|---|
| QG-SUF-004 | `TestQualityLostChildSpans` (the review's reproduction; remote children; without child counts) | Go unit |
| QG-SUF-005 | `TestQualityMinPerExecution` | Go unit |
| QG-SUF-006 | `TestQualityLostChildSpans` (assumptions), `TestRecordedBeforeAndAssumptions`, `qualityHistory.spec.ts`, `e2e/qualityHistory.e2e.ts` QG-UI-006 | Go unit; CLI; Vitest; Playwright |
| QG-HIST-004 | `TestQualityRetryReturnsTheRecord` (the review's reproduction), `TestRecordedBeforeAndAssumptions`; fixture QG-HIST-001 (retry) | Go unit; CLI; fixture |
| QG-EVD-002 | `TestQualitySnapshotKeepsCounterexample`, `TestSnapshotSelection`, CLI `TestSnapshotKeepsCounterexample` | Go unit; CLI |

# Stage 7: one record per id, honest streaks, guardrails

Implementation: uncommitted after `0011a2d` at the time of writing
(2026-11-18). It answers the web agent's review of `0011a2d`, four source
findings, none reproduced by the reviewer against a running instance.

## What changed

- **Concurrent records.** `recordQuality` looked up an id and then inserted,
  so two simultaneous requests could both miss and store two verdicts under
  one id (the table has no uniqueness). Now an API instance records one id at
  a time (`recordLocks`), the insert carries the id as
  `insert_deduplication_token`, and the receiver sets
  `non_replicated_deduplication_window = 10000` on `quality_evaluations`
  (also on existing tables), so ClickHouse drops a second insert of an id
  from another instance. Each request reads the record back after its insert
  and answers with what is stored. Reads (`lookup`, `LoadQualityEvaluation`,
  the history list) take the earliest row of an id, for duplicates recorded
  before.
- **Streaks and criteria.** Each requirement result has `criteria_sha256`
  (its checks' definitions without ids, targets and the advisory flag, plus
  `run.executions`), carried in the history summaries. A streak ends where the
  criteria change (`criteria_changed`), so two passes against different
  limits are two streaks; older records without criteria fall back to the
  rules file's sha256. The UI says "Met since …, when its checks changed; met
  under the earlier checks." and outlines cells where the criteria changed,
  not on every edit of the rules file.
- **Guardrails.** `guardrails:` on a requirement lists checks that support it
  without assessing it. They decide the gate as before, are listed with the
  requirement (`guardrail_summary`), and never change its status. The example
  rules, the fixture and the docs make `db-calls` REQ-START's guardrail: 400 ms
  starts with four database calls no longer mark "starts within 800 ms" as
  violated. The example application's REQ-ORDER-READ ("one query per table")
  is now assessed by its db-calls check, with the latency check as guardrail.
- **Wording.** REQ-DIRECTORY is "Starting a session must not call the
  directory": `forbidden_call` sees a call, not whether the caller waited.

## Verification

- Go unit tests (log-analysis, log-receiver, quality-check), Vitest (104),
  Playwright quality pages (8).
- Two API instances against the test ClickHouse, 16 simultaneous record
  requests split between them, eight rounds: one row and one answer every
  time. The same race against two instances of `0011a2d`: 8, 15 and 10 rows
  for 16 requests, each with its own recorded time.
- Fixture on a reused test volume: 69 of 70 checks pass, QG-HIST-001 (with
  the guardrail and criteria) and QG-HIST-005 included; M2-CAL-008 fails
  because the volume's hours were already final (known for reused volumes).

## Scenario map

| Scenario | Test | Layer |
|---|---|---|
| QG-HIST-005 | `TestQualityConcurrentRecordsAreOneRecord` (one instance and two), `TestQualityRecordReadBackFails`; fixture QG-HIST-005 | Go unit; fixture; manual two-instance race |
| QG-HIST-006 | `TestQualityStreakEndsWhenCriteriaChange`; `qualityHistory.spec.ts`; `e2e/qualityHistory.e2e.ts` QG-UI-006 | Go unit; Vitest; Playwright |
| QG-REQ-003 | `TestQualityGuardrails`; CLI `TestRequirementsAndRecord`; fixture QG-HIST-001 (candidate's REQ-START) | Go unit; CLI; fixture |

## Still open

- Requirement-support relations beyond guardrails, controlled workload and
  environment provenance, comparable load experiments and outage verification
  (the reviewer's larger vision); a pilot with one external application is the
  next step the review recommends.

# Stage 8: coverage per execution, unconfirmed records

Implementation: uncommitted after `b7c2e87` at the time of writing
(2026-11-18). It answers the web agent's review of `b7c2e87`.

## What changed

- **Child-span coverage per execution** (high). One span with a positive
  child count selected "Lost child spans are detected too" for the whole run,
  so a run mixing a log-sender execution with 199 OTLP executions that lost
  spans passed with that assurance (the reviewer's reproduction). Now each
  execution is `Counted` only when its entry and every span with recorded
  same-service children report a count, the result has `child_counts`
  (executions, counted, all/partial/none), and the assumption says "detected
  in N of M executions only" for a partial run. `run.child_counts: required`
  makes `db_calls` and `forbidden_call` passes with uncounted executions
  `unverified_child_counts` (unresolved); it is part of the requirements'
  criteria (left out of the hash when not set, so recorded criteria stay
  equal).
- **Unconfirmed records** (medium). When the insert went through but the read
  back failed, the CLI printed "Recorded as evaluation". The API now sets
  `record_unconfirmed` and keeps the id; the CLI prints "Not confirmed: …"
  with the link (also for an API that sends only `record_error` with an id),
  and the Quality page shows the same for a result file.

## Scenario map

| Scenario | Test | Layer |
|---|---|---|
| QG-SUF-007 | `TestQualityMixedChildCoverage` (the review's reproduction, the policy, a parent without a count, criteria) | Go unit |
| QG-HIST-007 | `TestQualityRecordReadBackFails`; CLI `TestUnconfirmedRecord`, `TestChildCountsRule`; `e2e/quality.e2e.ts` QG-UI-003 | Go unit; CLI; Playwright |

## Still open

As the review lists: controlled-run comparability (runner registration and
finalization, expected entry identities, scenario, environment, dataset,
instrumentation and workload revisions), sync-only call rules and scoped
allowlists, independent import and promotion of requirement revisions, load
and outage verification. Next acceptance test: one real pull request in a
second repository.

# Stage 9: quick wins after the first external pilot

The first integration of an outside Go application showed four things worth
fixing at once (2026-11-18).

## What changed

- **The run's own window.** A request may leave out the window; the API
  takes the run's (first to last entry span within 24 h) and the baseline's,
  and says so (`window_from_run`). The CLI's default and the action's
  `since` default are now that window: a window counted back from "now"
  changed on every retry, so a recorded evaluation recorded again, and a
  window that started late made a run unresolved. An older API that needs a
  window gets the last `--since` from the CLI. This repository's own gate (`.github/quality.yaml`, at most 3 queries per `quality/evaluate`) failed the first version of this change: it looked up the run's and the baseline's window with a query each, 4 calls in all, and the new statement details named both lookups. They are one query now.
- **Explainable database calls.** otelsql's rows spans carry `db.system`
  and were counted as calls (a query counted twice); they aren't now, and the
  details say so. The worst execution's calls are listed by statement.
- **log-sender as an outside module.** CI builds an application that
  imports log-sender with only log-sender replaced, so protos comes from the
  version log-sender requires (the bug fixed in #24: it compiled only inside
  this repository).
- **Wording.** A guardrail is defined where it first appears in a result;
  the docs say up front that a pass means "not observed in this run". The
  README's integration guide (#25) starts from what an application must
  never send.

## Scenario map

| Scenario | Test | Layer |
|---|---|---|
| QG-WIN-001 | `TestQualityWindowFromRun`, `TestQualityRunRangeQuery`; CLI `TestWindowFromRun`, `TestWindowFromRunOldAPI`, `TestWindowFromRunReport`; fixture `qgWindowFromRun` | Go unit; CLI; fixture |
| QG-EVD-006 | `TestQualityDBCallsStatements` | Go unit |
| QG-DOC-001 | CLI tests of the guardrail line; docs | CLI; review |
| (log-sender) | CI job `log-sender-outside` | CI |

# Stage 10: privacy by default

An outside application's integration leaked SQL arguments, a login token in
WebSocket query strings and camera credentials in an attribute key; nothing
in Troubleshooting stopped or found them (2026-11-18). Record: `docs/privacy.md`.

## What changed

- **log-sender redacts by default** (`redact.go`): SQL arguments, URL
  credentials in keys and values, query strings of request URLs, keys named
  by `WithRedactKeys`; `troubleshooting.redacted` says what was removed.
  `WithSQLArgs` opts in, logs it once and marks the resource. The API's own
  tracing opts in. This changes what earlier versions sent.
- **The receiver applies the same rules** to every sender before storing
  (`TROUBLESHOOTING_REDACT=off` to store as sent).
- **`troubleshooting-check privacy`** and `POST /api/v1/privacy/scan`: kinds
  of leak with counts and an example, never the value; exit 1 on findings.

## Scenario map

| Scenario | Test | Layer |
|---|---|---|
| PRV-001 | log-sender `TestRedactionByDefault`, `TestRedactionWithSQLArgs`, `TestRedactionOfLogRecords`, `TestRedactText`; end to end against the test stack (a span and a log record through log-sender: stored cleaned, scan exits 0) | Go unit; manual |
| PRV-002 | receiver `TestReceiverRedaction` | Go unit |
| PRV-003 | `TestPrivacyRequest`, `TestPrivacyPatterns`; CLI `TestPrivacyCommand`; against the test stack (probe rows with all five kinds: found, the opted-in span not) | Go unit; CLI; manual |

## Not done

- A purge command (plan item G): data stored before these layers needs
  `ALTER TABLE … DELETE`.
- The UI doesn't yet show `troubleshooting.redacted` specially (it appears
  as an attribute).
