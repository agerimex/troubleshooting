---
description: Compare two releases by call counts, error fraction, p50/p95/p99 latency and error groups, and open the example calls behind each number.
---

# Releases

Releases answers “what changed between two releases, and did the fix help?”. It has two tabs: **Compare releases** and **Operation statistics**.

## Compare releases: choose two populations

A population is a service, a window and optional dimensions: operation, span kind, environment, version, installation and segment. Each dimension is *any*, *is* (a value) or *not recorded*. Fill the **Baseline** and the **Candidate**, then click **Compare candidate against baseline**.

- Windows use UTC hour buckets. A bound inside an hour is computed from raw records while they are retained.
- **Copy baseline filters to candidate** and **Swap** save typing. Version text has no order: you decide which side is the baseline.
- **Saved comparisons** reopens any comparison stored earlier, exactly as it was generated.

![Comparison form](/screenshots/compare-form.png)
*Baseline: `recording-worker` version 1.0.0, 10:00–12:00. Candidate: version 1.1.0, 12:00–14:00.*

## The comparison

The result is a stored analysis with its own revision id and rule versions (`comparison-v1`, `metrics-v1`, `errors-v1`):

1. **Populations**: window, status, coverage and where the values came from (stored or provisional buckets, raw records), plus the cutoff time.
2. **Per operation and span kind**: recorded calls, call rate, error fraction, p50, p95 and p99, with absolute and relative change and a result.
3. **Error groups** of both sides with counts, change and a label such as “Recurring in both scopes”.

| Result | Meaning |
| --- | --- |
| Regression signal | The change crosses the comparison rule's thresholds |
| Within threshold | A change, but below the thresholds |
| Descriptive only | Shown for context (call volume); never a signal on its own |

Signals are heuristics that point to what to investigate, not proof that a deployment caused the change. **Regenerate (new revision)** builds the analysis again; **Export HTML** and **Export JSON** save exactly the shown revision.

![Comparison result](/screenshots/compare-result.png)
*Version 1.1.0 against 1.0.0: `storage.PutObject` p95 +66.7%, `upload_recording` error fraction 1% → 5%.*

## Examples behind a number

**Slow candidate examples** under a metric table lists observed calls at or above the candidate's p95, slowest first (10 shown). **Candidate examples** and **Baseline examples** do the same for an error group. **Open report** opens one example as an execution report, which keeps a note of the comparison it came from.

![Slow candidate examples](/screenshots/compare-examples.png)
*Slow candidate calls of `storage.PutObject` at or above 150 ms.*

In an opened example, **Include in comparison export** adds its report to the comparison's HTML/JSON export. **Back to Releases** returns to the comparison.

![Example report from a comparison](/screenshots/compare-example-report.png)
*A slow example opened from the comparison, with “Include in comparison export”.*

## Operation statistics

The second tab shows one population without a comparison: one row per operation and span kind (or per version with **one row per version**), with calls, rate, ERROR/OK/UNSET counts, error fraction, percentiles and valid durations. Below it are calls per hour (UTC) and the population's error groups, each with examples. Percentiles need at least 20 (p50), 200 (p95) and 1000 (p99) valid durations.

![Operation statistics](/screenshots/stats.png)
*Statistics of `recording-worker`, 10:00–16:00, with hourly buckets and error groups.*
