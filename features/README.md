# Milestone 1: combined trace and log investigation report

These Gherkin acceptance specifications define the first product milestone: an engineer can inspect and export one report containing a selected trace, its correlated logs and the limits of the available evidence.

The investigation can start from a failed span or an error log found by time, service and message. Opening a report preserves the chosen time window and retrieves the selected trace's available context across services and log severities. A missing TraceId leaves a log inspectable, with an explicit correlation limitation.

Use these feature files as the acceptance scope for that outcome. The earlier trace-only step1.md is not a substitute for the combined-report scenarios.

## Files

- features/milestone_1/combined_report.feature: the usable report and entry workflow.
- features/milestone_1/evidence_correlation.feature: reliable associations and source labels.
- features/milestone_1/report_completeness.feature: search windows, retention, limits and failures.
- features/milestone_1/report_export.feature: self-contained HTML and JSON artifacts.
- features/milestone_1/replay_and_refresh.feature: logical identity, retries and refreshed evidence.
- features/milestone_1/reference_investigation.feature: supported ingestion, reproduction and persistence.

## Execution conventions

- The files are acceptance specifications, not evidence of implemented or passing product behavior. Cucumber-compatible step definitions, fixtures and application integration are still required.
- Tags beginning with M1- identify scenarios. The core tag highlights the end-to-end product path; all milestone_1 scenarios remain acceptance requirements.
- Human-readable trace, span and log identifiers are fixture aliases. Step definitions must map trace/span aliases to valid protocol identifiers. Reusing the same span alias in different traces intentionally means the same SpanId.
- Log record identity must survive a retry. Separate events retain separate identifiers, even when their text and timestamps match.
- Generate one base time for each isolated fixture run; reuse the exact payload on retry. All unspecified reference evidence is inside a declared UTC window. Apply windows as [from, to).
- In log tables, offset_ms is the recorded event timestamp relative to the fixture base time; table row order is not arrival order or chronological order. The value `not recorded` in the span column means no SpanId was recorded.
- Keep fixture data isolated from development data and the application's own telemetry. Wait for ingestion/readiness with bounded checks.
- The reference scenario must use a real supported ingestion path for each evidence type. One supported log path is sufficient; implementing OTLP logs is not a prerequisite.
- HTML and JSON are the chosen export formats for this specification. An export captures the already generated report; it is not a promise of database snapshot isolation during concurrent ingestion.
- Search filters identify an entry record; service, severity and message filters do not silently carry over to the generated report. Selecting a span focuses the visible logs without removing other generated evidence. The full trace context remains accessible, and exports retain the full generated report.
- Complete means the configured sources successfully returned all matching retained records for the declared window. It does not prove every real-world event was instrumented, sampled or delivered.
- Examples using a limit of 3 are deliberately small fixture settings, not a production recommendation.

## Scope boundary

Each report covers one selected TraceId within explicit time bounds. A business operation may span multiple traces. Shared business identifiers do not merge their records; span links are displayed as references to traces that have not been inspected. Source completeness within the declared scope does not establish the final business outcome or coverage of an entire workflow.

Multi-trace workflow reconstruction, percentile dashboards, release comparisons, retained statistical aggregates, business-workflow modeling, AI investigation and automatic remediation remain later work. No effort estimate or implementation status is implied by these files.

## Practical validation after implementation

The reference fixture demonstrates correct handling of known evidence; it does not establish faster investigation or prove a root cause. Evaluate the implemented workflow with a representative real or replayed incident and another engineer who has not been given the answer. Record whether they identify the failing operation, cite relevant evidence, recognize limitations and choose a defensible next action. Compare their active investigation effort with the existing workflow; do not claim time savings before measuring them. This exercise complements the acceptance scenarios and is not an automated pass/fail threshold.

## Revision 2026-10-11

- Added log-first discovery, including OK spans and a log without a TraceId.
- Defined span selection, chronological log context and return to the full trace view.
- Made the single-trace scope explicit in reports and exports; focused views retain all generated evidence on export.
- Updated the reference investigation to begin with log discovery.

Prepared 2026-10-11 from the agreed combined-report outcome. Gherkin syntax reference: https://cucumber.io/docs/gherkin/reference/
