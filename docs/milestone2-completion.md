# Milestone 2 completion record: statistics, error groups and release comparison

Acceptance scope: `features/milestone_2/*.feature` (46 scenarios), with
`features/milestone_2/METRIC_CONTRACT.md` (metrics-v1, errors-v1,
comparison-v1) and `REFERENCE_FIXTURE.md`, copied from
`Milestone_2_Feature_Files.zip` (11 October 2026). There are no Cucumber step
definitions: each scenario is checked by the tests named below, whose titles
carry the scenario tag.

Implementation: branch `feat/milestone-26-10`, on top of milestone 1,
uncommitted at the time of writing. **Fill in the commit after committing.**

## What changed

- **log-receiver**: schema moved to `internal/driver/schema.go`. Raw rows get
  `ReceivedTime`. New tables `stat_calls`, `stat_errors`, `stat_revisions`,
  `collection_heartbeats`, `coverage_declarations`, `analyses`. Retention is
  configured here (`TRACE_RETENTION`, `LOG_RETENTION`, `STATS_RETENTION`) and
  applied to existing tables; a raw retention too short for finalization is
  rejected at startup. A heartbeat every 15 s records collection coverage.
  OTLP spans without a valid duration are stored as -1 instead of 0.
- **log-sender**: `WithResourceAttributes` and the attribute key constants
  (version, environment, installation, segment, sampling policy, operation).
- **log-analysis**: statistics, error groups, examples, comparisons, stored
  analyses, the bucket refresher, coverage and effective retention (read from
  ClickHouse, no longer from `TRACE_RETENTION`/`LOG_RETENTION` of the API).
  Payload hashes sort attribute maps (`mapSort`): exact retries were stored
  with maps in a different key order and looked like conflicting payloads.
- **log-ui**: **Statistics** and **Compare releases** entries;
  `PopulationForm`, `ExamplesPanel`, comparison export (HTML/JSON) with the
  selected combined reports embedded; the report shows the comparison it was
  opened from and can be included in the export; invalid durations are shown
  as "invalid", never 0.
- **fixture**: version 3. `m2.go`, `m2checks.go`, `m2retention.go`; the test
  stack runs the fixture retention policy (48 h raw, 30 days statistics) and
  `TEST_CLOCK_CONTROL=1`; `make fixture-check` adds `-retention`.

## How statistics are computed

Bounded recomputation, as the contract allows: every completed UTC hour is
recomputed as a whole from canonical raw records and stored as a new
revision, committed by its `stat_revisions` row. Readers use exactly one
committed revision per hour (final wins), or raw records for hours without
one and for partial hours, never both. A record counts in a bucket if the
receiver stored it before bucket end + allowed lateness; after that deadline
the next rebuild commits the bucket as final, and it is never rebuilt again.
Comparisons are stored as JSON exactly as shown; exports never recompute.

Since 13 October 2026 only records whose time another row in the hour shares
are grouped per identity to pick the canonical row; a record alone at its
time is used as it is. Retries and replays keep a record's time, so the
results are unchanged (the milestone 2 checks pass, and rebuilding hours
computed the old way reported "unchanged"). Before, the hour's grouping held
every record in memory and failed at the 1 GiB query limit from about 1 M
spans an hour; an hour of 4.3 M spans now takes ~6 s and ~200 MB. Copies of
one identity stored with different times are no longer compared, as in the
span and log lists. Refresh queries get at least one minute.

## Results (local, 2026-10-11)

- Regression fixture on a clean test volume (`test-stack-reset` → `up` →
  `load` → `check` with replay, outage, recreation and the retention phase):
  **181 checks run, 0 failed** (run `be51eb0c`, fixture version 3).
- Before the `mapSort` fix the same run failed 5 checks (replays looked like
  conflicting payloads; RET-005's replay produced a new revision), which the
  fix removed.
- Go unit tests pass in every module (new: comparison rules, group labels,
  planner, coverage, normalization, placeholders, retention parsing).
- vitest: 39 pass. Playwright (mocked API): 45 pass (14 for milestone 2).
- Real-stack walkthrough of the UI (test stack, scratch script): saved
  regression comparison shows +60.0 ms / +60.00 % and +4.00 pp / +400.00 %
  as regression signals; regenerate creates a revision that names the
  earlier one; a StorageError example opens the trace with the API parent,
  storage span and the 403 log; the HTML export embeds it without scripts.

## Scenario map

Fixture = `fixtures/regression` on the real stack; UI = `log-ui/e2e/comparison.e2e.ts`;
Go = `log-analysis/internal/data/compare_test.go` unless noted; vitest =
`log-ui/src/services/__tests__/comparison.spec.ts`.

| Scenario | Checked by |
| --- | --- |
| CAL-001 SERVER calls, no dependencies | Fixture M2-CAL-001 |
| CAL-002 versions, environments, unknown | Fixture M2-CAL-002 |
| CAL-003 window and rate | Fixture M2-CAL-003 |
| CAL-004 replay, colliding span ids | Fixture M2-CAL-004 (every phase, after double replay) |
| CAL-005 zero vs unavailable | UI M2-CAL-005; vitest |
| CAL-006 sampling and gaps | Fixture M2-CAL-006; Go `TestInsufficientEvidence` |
| CAL-007 conflicting records | Fixture M2-CAL-007; Go `TestInsufficientEvidence` |
| CAL-008 time series with a gap | Fixture M2-CAL-008; UI M2-CAL-008 |
| LAT-001 unit conversion | Fixture M2-REF-001 (100 000 µs → 100 ms) |
| LAT-002 slow tail | Fixture M2-LAT-002 |
| LAT-003 unequal buckets | Fixture M2-LAT-003 (merged stored states) |
| LAT-004 sample thresholds | Go `TestPercentileSampleThresholds`; UI; vitest |
| LAT-005 invalid durations | Fixture M2-LAT-005; receiver `otlp` unit; vitest (`durationMsText`) |
| LAT-006 selected span, not children | Fixture M2-REF-001 (upload 100 ms with its storage child); see limitations |
| ERR-001 exception grouping | Fixture M2-ERR-001 |
| ERR-002 message grouping, codes | Fixture M2-ERR-002; Go `TestNormalizeMessage` |
| ERR-003 log errors vs failed calls | Fixture M2-ERR-003 |
| ERR-004 unlinked error | Fixture M2-ERR-004 |
| ERR-005 labels, undetermined novelty | Fixture M2-ERR-005; Go `TestGroupLabels`; UI |
| ERR-006 dedup, source kinds | Fixture M2-ERR-006 (also after replays) |
| CMP-001 regression signals | Fixture M2-REF-001; Go; UI; vitest |
| CMP-002 fix against both baselines | Fixture M2-REF-002; Go |
| CMP-003 zero baseline | Go `TestZeroBaseline`; UI; vitest |
| CMP-004 different window lengths | Fixture M2-CMP-004; Go |
| CMP-005 mixed results | Go `TestMixedResults` |
| CMP-006 mismatches | Fixture M2-CMP-006 (environment); Go `TestMismatchIsIncomparable` (rule-version scoping); UI |
| CMP-007 insufficient evidence | Go `TestInsufficientEvidence` (all five limitations and conflicts); UI |
| CMP-008 no candidate calls | Fixture M2-CMP-008; Go; UI |
| INV-001 slow example → report | Fixture M2-INV-001; UI |
| INV-002 log error example | Fixture M2-INV-002; UI |
| INV-003 bounded examples | Fixture M2-INV-003; UI |
| INV-004 later evidence vs revision | Fixture M2-INV-004; UI M2-REP-002 / M2-INV-004 |
| RET-001 statistics outlive raw | Fixture M2-RET-001; UI |
| RET-002 retention change | Fixture M2-RET-002 |
| RET-003 expired statistics | Fixture M2-RET-003; Go `TestPlanWindow` |
| RET-004 backfill twice | Fixture M2-RET-004 (see limitations) |
| RET-005 late call, provisional | Fixture M2-RET-005 |
| RET-006 final bucket, restart | Fixture M2-RET-006 |
| RET-007 historical + recent | Fixture M2-RET-007; Go `TestPlanWindow` |
| RET-008 failed refresh | Fixture M2-RET-008 |
| RET-009 whole-bucket bounds | Fixture M2-RET-009; Go `TestPlanWindow`; UI |
| REP-001 export HTML/JSON | UI M2-INV-001 (both formats, embedded report); vitest |
| REP-002 stable reviewed report | Fixture M2-REP-002; UI; vitest |
| REP-003 unavailable log statistics | UI; vitest; Go `TestGroupLabels` |
| REF-001 known regression | Fixture M2-REF-001, M2-REF-001E; walkthrough |
| REF-002 fix verification | Fixture M2-REF-002 |

## Deviations and limitations

- **Controllable clock.** Finalization and expiry are tested by moving the
  API's clock (`TEST_CLOCK_CONTROL`), as the feature README allows; arrival
  times stay real. Nothing waited a real 24 h or 30 days, and physical TTL
  deletion is not observed (expiry is checked logically).
- **Coverage** comes from receiver heartbeats and manual declarations.
  Heartbeats show the receiver was running and could write; they cannot see
  records a sender dropped before reaching it. Hours from before the
  heartbeats existed are "unknown" until declared, so comparisons over them
  give "insufficient evidence".
- **Sampling** is read from `troubleshooting.sampling_policy` (or
  `SAMPLING_POLICY_DEFAULT`). Without it, sampling is unknown and no change
  signal is given; this is deliberate but means existing instrumentation needs
  the attribute or the default before comparisons classify anything.
- **Attribute keys**: installation, segment and sampling use
  `troubleshooting.*` keys, log operations `operation.name`; OpenTelemetry has
  no standard keys for them.
- **Exception events** count at their own time only within 1 h after their
  span started (`eventLookback`). Events without a usable timestamp are not
  counted separately as "unassignable".
- **Error groups in a comparison** cover every operation of the service (each
  group shows its operation), so logs without a recorded operation are not
  hidden by an operation filter.
- **Examples** are read from raw records with the analysis cutoff; for a
  provisional bucket they can include a record that its stored revision did
  not have yet.
- **RET-004** is the same rebuild endpoint run over four days: there is no
  separate "statistics disabled" state. It shows idempotence and the
  unrecoverable range, not a migration of a pre-milestone installation.
- **REP-003** with only the log source down is checked in unit and UI tests:
  both sources live in one ClickHouse on the stack.
- **LAT-006** has one storage child per upload, not two overlapping ones; the
  calculation never looks at children.
- **Selected reports** for an export are kept in the browser until exported
  (not stored on the server).
- **Scale**: the refresher recomputes each provisional hour every minute
  (skipped when its content hash is unchanged). That is fine for the fixture
  and a small installation; it has not been measured at production volume.
  t-digest accuracy is checked only on the fixture's plateau and stepped
  distributions.

## Open items

- Commit, push and the first CI run (the workflow runs `make fixture-check`,
  which now includes the retention phase and recreates containers).
- Practical validation with another engineer (`features/milestone_2/README.md`).
- Instrument a real application with `WithResourceAttributes` and a sampling
  policy, and declare coverage for its history.
