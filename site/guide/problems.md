---
description: Recurring error groups, slow calls and release regressions of one service, each with why it was flagged and example calls to open.
---

# Problems

Problems lists the recurring symptoms of one service in a window: error groups, slow calls and release regressions. They are evaluated on request with the current [detection rules](./settings#detection-rules), not stored.

## The problem list

Choose a **Service** (required), optionally an **Environment**, and the window (the last 24 hours by default), then **Show problems**. Each row shows its type, title, operation, when it was last seen, and its measure on the right:

| Type | Opens when | Measure shown |
| --- | --- | --- |
| Error | An error group reaches the minimum number of affected calls. ERROR spans, exception events and ERROR logs are grouped and counted separately | Affected calls, matching records, traces |
| Slow calls | Enough calls of an operation exceed the duration rule. Needs raw records, so only inside raw retention | Slow calls |
| Release regression | A stored comparison shows a p95 increase above both thresholds | p95 before → after, calls evaluated |

Errors whose records name no span are listed as “Call attribution unavailable” and never meet the threshold.

![Problem list](/screenshots/problems-list.png)
*Eight problem groups of `fx7e3d4841-recording-worker`: three error groups of the same failure (exception, ERROR span, ERROR log), two release regressions, and smaller groups.*

## Problem detail

**Investigate** opens a problem: its counts, the versions it was seen in, first and last occurrence, and **Why was this flagged?** — the rule, what was observed, the threshold, grouping, scope and collection coverage.

Under **Investigate**, pick one of the retained example calls (up to 20 are kept) and open it with **Open timeline** or **Open sequence**. The example opens as its own report with the occurrence's span and log selected. Examples without a trace can still be inspected.

![Problem detail with why flagged](/screenshots/problems-detail.png)
*The error group “StorageError at storage.Upload”: 70 affected calls in versions 1.0.0, 1.1.0 and 1.1.1.*

![Problem example opened as timeline](/screenshots/problems-example.png)
*Its first example opened as a timeline. “Back to Problems” returns to the detail.*

## Release regressions and “Check fixed release”

A regression's **Compare releases** tab shows the stored comparison it was detected from: baseline and candidate versions, every metric with its change and result. Choose another version under **Fixed release** and click **Check fixed release** to see whether a later release brought the latency back.

![Release regression with comparison](/screenshots/problems-regression.png)
*“storage.PutObject became slower in version 1.1.0”: p95 90 ms → 150 ms, +66.7%.*
