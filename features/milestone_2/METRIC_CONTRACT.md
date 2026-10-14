# Milestone 2 metric and evidence contract

Contract identifiers: metrics-v1, errors-v1 and comparison-v1. These are proposed product contracts, not existing endpoint or schema names.

## 1. Populations and identity

A recorded call is one canonical span matching the explicitly selected service, operation, span kind, environment, version and UTC event-time window. Optional installation and workload-segment filters are retained when supplied. The window is half-open: [from, to). Call membership uses the selected span's recorded start time.

A span need not be a trace root to qualify. SERVER entry spans, CONSUMER operations and CLIENT dependencies are separate measurements; do not add all of them and label the sum as user requests. Duration belongs to the selected span, not the sum of overlapping child durations.

The span identity is (TraceId, SpanId). Log identity is a stable source identity plus event identifier that survives replay. An exception event is identified by its canonical span and stable recorded event position. Duplicate ingestion must not add occurrences. Different event identities remain distinct even when message and timestamp match.

Use each record's recorded resource metadata. In this milestone, absent version, environment or operation attribution stays in an explicit unknown group; do not infer it from timestamp proximity or the root span of another service. Error grouping of logs uses their own recorded attributes. A missing log operation is an unknown-operation group, not a reason to hide the log.

The canonical-record policy is inherited from the implemented Milestone 1 storage contract and is versioned in analysis metadata. Conflicting payloads for one identity are a data-quality problem: count at most one canonical identity, expose the conflict and withhold a qualified change signal for affected populations. Do not let later background merges silently erase that qualification.

## 2. Counts, fractions and rate

- Recorded calls N: all admitted canonical spans in the population.
- Recorded error calls E: those spans with recorded status ERROR.
- Recorded error fraction: E / N when N > 0; unavailable when N = 0.
- OK and UNSET are separate status counts. UNSET is not a recorded success.
- ERROR logs and exception events remain diagnostic evidence; they do not rewrite span status or the call denominator.
- Observed call rate: N divided by the full selected window duration, with the unit stated. A source gap makes coverage incomplete; do not silently shorten the denominator to active seconds.
- A successful complete query with no observations gives count 0 and rate 0. A failed query, expired interval or uncovered bucket is unavailable, not zero.

All metrics describe recorded evidence. Head/tail sampling, unknown sampling and collection gaps must be shown. This stage does not extrapolate counts to all traffic or estimate business completion from telemetry.

## 3. Durations and percentiles

Valid recorded durations are finite and nonnegative. Zero is valid; negative, missing or invalid duration values are excluded from percentiles and counted separately. They do not silently remove a recorded call from the call-count population. Display valid duration sample count beside percentiles.

Convert microseconds to milliseconds by dividing by 1000. Report p50, p95 and p99 over the same valid duration population and state the method. The initial method is ClickHouse t-digest quantiles with a recorded algorithm/implementation version. It is approximate; recomputation can vary with aggregation order.

For the deterministic plateau fixtures in these files, accept an absolute percentile error of at most 1 millisecond. This is a fixture acceptance tolerance, not a universal t-digest error guarantee. Validate representative real distributions separately against an exact reference before relying on a broader accuracy claim. Record any measured benchmark conditions.

Initial configurable sample heuristics:

| Metric | Required valid duration samples |
| --- | --- |
| p50 | 20 |
| p95 | 200 |
| p99 | 1000 |

Below the selected threshold show insufficient sample, not a normal headline percentile or an eligible change classification. N = 0 produces an unavailable percentile. Meeting a threshold does not prove precision or statistical significance.

Combine distributions or mergeable aggregate states to calculate a percentile over multiple buckets. Never average or weight already-computed scalar p95 values. An exported reviewed report stores its displayed values; exporting does not rerun an approximate calculation.

## 4. Error grouping

Errors-v1 keeps source kinds distinct: ERROR-status span, exception event, and ERROR/FATAL log. An event belongs to one group within its source kind. A status error, its exception and an ERROR log can describe the same underlying incident; their counts must not be added and labeled as distinct failures.

Group dimensions include recorded service, operation, environment, source kind and a versioned fingerprint. Service version is a comparison dimension, not part of the cross-release fingerprint identity.

Prefer recorded exception type and a normalized application frame. For the reference fixture the frame is the module/function; line numbers and request identifiers are excluded. Different application frames stay separate. Where structured exception details are absent, use an explicitly labeled normalized-message signature. Only configured dynamic identifiers are normalized; meaningful codes such as 403 and 500 remain distinct. Preserve original messages for raw-evidence inspection.

Log and exception occurrence windows use recorded event timestamps; an ERROR-status span uses its start time. Thus error-event counts need not equal call counts even in the same wall-clock window. If an event has no usable timestamp, report it as unassignable to time statistics; do not assign the ingestion time silently.

For each group show occurrences, known affected trace count, unlinked occurrence count, first/last occurrence in the selected window, release distribution and available examples. Distinct counts across multiple groups are not additive. Shared trace identity alone does not establish customer, business-case or incident identity.

Newly observed means absent from a sufficiently covered selected baseline and present in the candidate. Not observed in candidate does not mean permanently resolved. Unknown or incompatible baseline coverage makes novelty undetermined. All labels remain scoped to the selected observed populations.

## 5. Comparisons and change rules

A comparison selects two explicit populations. It can compare two versions within one window or separate windows, or the same version across windows. Version text has no chronological meaning; the user selects baseline and candidate.

Retain the service, operation, span kind, environment, optional installation/workload segment and sampling contract. Known differences beyond the selected version or time window make the affected comparison ineligible until reconciled. Unknown coverage is insufficient evidence. Matching declared context improves comparability; it does not prove that every confounding factor was controlled.

A metric definition mismatch blocks that metric. An error-fingerprint rule mismatch blocks group identity/novelty conclusions, not otherwise compatible call or latency comparisons. Show independently available statistics even when the conclusion is unavailable.

Absolute difference = candidate minus baseline.
Relative change = 100 * (candidate - baseline) / baseline when baseline is nonzero.
For an error fraction, absolute changes are percentage points; relative changes are percentages. A zero baseline has no defined relative change, including 0 to 0.

Comparison-v1 initial configurable signal rules:

| Measurement | Minimum evidence | Change threshold |
| --- | --- | --- |
| Selected latency percentile | Both duration populations meet that percentile's minimum | Absolute change at least 20 ms AND relative magnitude at least 20 percent |
| Recorded error fraction | At least 1000 calls in each population | Absolute change at least 1 percentage point |

Apply the latency rule symmetrically for increases and decreases relative to the selected baseline. When baseline latency is zero, use the absolute threshold alone and show relative change as unavailable. Error fractions use the absolute percentage-point threshold even with a zero baseline. Zero to zero is within threshold when there are sufficient calls.

Classify each metric separately: regression signal, improvement signal, within threshold, insufficient evidence or incomparable. More duration and more recorded errors are deterioration. Call-count changes alone are descriptive. Mixed metrics retain their individual results.

These thresholds are explainable product heuristics, not hypothesis tests, confidence intervals, causal findings or automatic release approval. Retain sample counts, thresholds and rule version with the report. A signal supports further investigation; reproduction or additional evidence is needed to attribute a cause.

## 6. Analysis revisions and drill-down

A generated analysis records population definitions, source coverage, calculation/grouping/rule versions, units, sample counts, values, analysis cutoff or committed aggregate generations, and generation time. Use consistent revisions for a metric's counts, denominator and distribution.

Logical analysis consistency does not require promising database-wide snapshot isolation. Implement a bounded materialized result or equivalent cutoff/generation scheme. Export captures that result exactly. Re-running it creates a separately identifiable revision.

Selected examples must match the statistical population and its supported cutoff, and identify TraceId plus SpanId or a stable log/event identity. A bounded list is labeled as examples. Slow examples use the displayed duration threshold; an approximate p95 need not correspond to one exact span.

Opening an example uses the Milestone 1 combined-report workflow. It retains the comparison's context but loads the selected trace's other available spans and logs, including other services and severities. The initial investigation window equals the selected population window; explicit widening is allowed and does not change the earlier comparison.

A newly generated detail report can include later-arriving context. Its own generation time and limitations remain visible. Do not represent fresh details as part of an older statistical snapshot. Offline comparison exports contain the exact selected embedded reports; unembedded live links are labeled accordingly.

## 7. Retention and aggregation correctness

Configure raw trace retention, raw log retention and statistical retention separately. Display effective settings and apply changes to existing installations through the supported migration/configuration procedure. No production retention default is prescribed here.

Acceptance fixtures use 48 hours for each raw source, 30 days for statistics, one-hour UTC buckets, a 24-hour allowed-lateness period after bucket end and a one-hour rebuild margin. They are test settings. Raw retention must exceed the bucket width plus allowed lateness plus the documented rebuild margin for each contributing source; reject incompatible configuration before it is used.

A provisional bucket may be refreshed from the complete admitted canonical population. Publish a replacement revision consistently, rather than adding a second copy of the bucket. This applies to counts, duration states, error groups and distinct-trace information. Failed refreshes leave the old revision intact and visibly stale.

For new data, statistical eligibility uses a recorded first-received time. A record for a bucket is eligible if first received before that bucket's end plus allowed lateness. At or after the deadline it is too late for that bucket. Finalize only after the deadline and successful processing of the admitted data. A wall-clock deadline alone does not establish successful collection.

Persist the finalization watermark and generation metadata across restarts. Normal ingestion must not modify a finalized bucket, even if raw copies have expired and an old record is replayed. Newly seen too-late records receive an explicit statistical-exclusion reason and a coverage notice; raw-ingestion acceptance is a separate result. This bounded contract avoids claiming eternal deduplication after identity evidence has disappeared.

The initial historical backfill uses a recorded snapshot of retained canonical records. State its actual coverage and any unknown legacy sampling/receipt information. It can initialize covered historical buckets. Re-running it is idempotent; replacement requires a complete source population for the target interval. Never rebuild a full historical bucket from only its surviving raw subset or overwrite known complete history with that subset.

Retained states preserve selected cohort dimensions, source/grouping versions, count/status values, mergeable duration distributions and required error-group statistics. Do not discard version or environment dimensions needed for later comparison. Use a disjoint plan for historical states and newer raw/provisional data so the same interval never contributes twice.

After raw expiry, detailed example availability can shrink while statistical values remain. If statistics also expire, the interval is unavailable, not a zero. Report coverage bounds and bucket granularity. An exact arbitrary sub-hour query requires adequate raw evidence; otherwise offer explicit whole-bucket bounds without silently widening or prorating.

Retention can be logically effective before physical deletion. Background cleanup is asynchronous. A retention increase cannot recover physically deleted records. Tests should use backdated fixture data or a controllable application clock and bounded cleanup checks, not multi-day sleeps.

## 8. ClickHouse implementation constraints

ClickHouse is the current storage choice. These are engineering constraints supporting the feature behavior, not a required table migration design:

- AggregatingMergeTree can store mergeable aggregate states queried with State/Merge combinators. It does not make duplicate source events harmless by itself. [CH1]
- Incremental materialized views process inserted blocks. Background deduplication in a raw table does not retroactively correct results already emitted by a view. Canonicalize before aggregation or use verified replacement/rebuild semantics. [CH2, CH3]
- T-digest is approximate and order-dependent. Freeze reviewed values; use documented tolerance when comparing independently computed results. [CH4]
- TTL cleanup occurs through background work; acceptance should distinguish configured logical coverage from immediate physical deletion. [CH5]

A materialized view over existing tables does not automatically constitute historical backfill. Choose and validate a path for already recorded data. The features permit a simpler bounded recomputation approach; they do not require a new streaming platform.

Primary references checked on 11 October 2026:

- [CH1 AggregatingMergeTree](https://clickhouse.com/docs/reference/engines/table-engines/mergetree-family/aggregatingmergetree)
- [CH2 Incremental materialized views](https://clickhouse.com/docs/concepts/features/materialized-views/incremental-materialized-view)
- [CH3 ReplacingMergeTree](https://github.com/ClickHouse/docs-reference/blob/master/docs/en/engines/table-engines/mergetree-family/replacingmergetree.md)
- [CH4 quantileTDigest](https://clickhouse.com/docs/reference/functions/aggregate-functions/quantileTDigest)
- [CH5 Manage data with TTL](https://clickhouse.com/docs/concepts/features/operations/delete/ttl)
