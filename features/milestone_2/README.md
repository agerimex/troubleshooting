# Milestone 2: statistics, error reports and release regression investigation

Prepared for Artem Parygin on 11 October 2026.

## Product outcome

An engineer can see how recorded call volume, errors and latency changed between two releases or time windows; inspect representative calls in the combined trace-and-log report; and compare a subsequent fix with both the affected release and the original baseline.

Today the Milestone 1 specification defines investigation of one selected trace. After Milestone 2 is implemented, a population-level finding provides the entry point to that same investigation. These files specify intended behavior; they do not claim that either milestone has been implemented.

## Keep the original focus

| Original feature | Required Milestone 2 outcome | Acceptance files |
| --- | --- | --- |
| Call statistics | Canonical observed call count, status counts, recorded error fraction, time-normalized rate and trends | call_statistics.feature |
| Percentiles | p50, p95 and p99, units, sample counts, valid duration population and mergeable distributions | latency_percentiles.feature |
| Error report | Recurring error groups, source-specific occurrences, first/last occurrence, unknown attribution and examples | error_groups.feature |
| Progress and regression between versions | Explicit baseline/candidate, absolute and relative changes, improvements, deterioration and inconclusive results | release_comparison.feature |
| Logs and traces in one report | Every available selected example opens the existing combined investigation with its context | investigation_drilldown.feature |
| Historical statistics | Configurable retention independent of raw evidence, backfill, freshness and replay-safe history | statistics_retention.feature |
| Shareable evidence | A reviewed comparison with selected combined reports in HTML or JSON | comparison_report.feature |
| Reproducible investigation and fix verification | Baseline, degraded release and fixed release with known expected results | reference_regression.feature |

The central workflow is: select an operation and comparable populations; inspect the differences; open a slow call or error example; investigate its spans and logs; export the evidence; repeat the comparison after a fix.

## Package contents

- Eight feature files under features/milestone_2.
- METRIC_CONTRACT.md defines populations, identities, units, formulas, grouping, comparison eligibility and retention semantics.
- REFERENCE_FIXTURE.md defines the controlled three-release example and expected results.

All feature files use English Gherkin. Stable tags beginning with M2- identify scenario definitions. The core tag highlights the first complete workflow; the remaining scenarios are still acceptance requirements for this milestone. Scenario outlines expand to multiple test cases.

These are acceptance specifications. Step definitions, an application sender, ingestion integration, migrations, queries and interface implementation are separate work. Parser validation does not establish product correctness.

## Relationship to Milestone 1

Use the revised Milestone_1_Feature_Files.zip from 11 October 2026 as the investigation contract. This package adds statistics and comparisons; it does not replace or narrow that contract.

- M2-CAL-004 and error replay behavior depend on M1-COR-001 and M1-REL-001/002 identities.
- M2-INV-001/002 continue M1-RPT-002/005/007: discover an example, retain supporting context, move between a span and its logs.
- M2-RET-001 and M2-REP-003 preserve the missing-evidence distinctions in M1-CMP-001/002/006.
- M2-REP-001/002 extend M1-EXP-001/003/004/005: export reviewed evidence, preserve scope and treat recorded text as data.
- Individual reports retain the M1-RPT-008 single-trace boundary. A comparison covers many recorded calls but does not reconstruct a multi-trace business workflow.

Error-group and call populations deliberately differ. A log group counts log events; a call metric counts qualifying canonical spans. The interface must identify that difference, including separate time-selection rules.

## Development order

Build a complete small workflow first: one selected operation, two fixture versions, call/error counts and percentiles, a comparison, and a contributing example that opens the combined report. Then complete grouped errors, fix verification, export, historical retention and the remaining quality cases. Keep the product path available while hardening it.

The order is an implementation suggestion, not permission to omit acceptance requirements. Estimate after checking the implemented Milestone 1 contracts and the current schema. No inherited 60-hour or 75-hour allocation is a validated estimate for this scope.

## Deliberate scope boundary

This stage includes call and error statistics, time buckets, latency distributions, version/window comparison, deterministic rule-based change signals, retained history and investigation evidence. It does not require AI.

Explicit customer outcomes, cross-trace business operation reconstruction, revenue calculations, human handoff analysis and AI investigation remain later extensions to the same report. Technical OK status and observed error fractions are not business success measurements. There is no automatic deployment verdict or remediation action in this milestone.

One installation serves one organization, following the existing roadmap. Authentication and transport requirements for shared use remain release prerequisites; this feature bundle does not introduce hosted multitenancy.

## Source alignment

- Troubleshooting_Roadmap_2026_2027.docx, 10 October 2026: release analysis core, Q1 call counts/error groups/percentiles/comparisons, and later business context and AI.
- Troubleshooting_Development_Plan_2026-10-10.docx: TS11 through TS14 supply the error, call, comparison and export intent.
- The later agreed technical-first sequence takes precedence over the earlier development plan's dependencies on business-event tickets TS08/TS09. Business events are not prerequisites for these analytics.
- The revised Milestone 1 features take precedence over the roadmap's older trace-only first milestone.

Current ClickHouse documentation checked on 11 October 2026 informs the engineering notes in METRIC_CONTRACT.md. This is a product specification, not a new repository review.

## Validate usefulness after implementation

Have another engineer investigate a representative incident without revealing the answer. Record whether the comparison identifies a relevant change, whether the engineer reaches the supporting logs and spans, which evidence is missing, and whether the next check is defensible. Compare active investigation effort with the current method where the tasks are comparable.

The controlled fixture verifies behavior. It does not prove general root-cause accuracy, market demand or time savings.
