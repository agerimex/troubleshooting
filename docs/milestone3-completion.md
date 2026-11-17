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

Implementation: branch `feat/quality-review-stage-2`, uncommitted at the time
of writing (2026-11-17). **Fill in the commit after committing.**

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
