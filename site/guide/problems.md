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
*Nine problem groups of `fxdc108d54-pw-worker`, 09:00–10:00: slow `render_preview` calls, error groups per source kind (ERROR logs, ERROR spans), and “Cache volume is full”, whose logs name no span, so its calls can't be attributed. No release regression is listed: no stored comparison of this service overlaps the window.*

## Problem detail

**Investigate** opens a problem: its counts, the versions it was seen in, first and last occurrence, and **Why was this flagged?** — the rule, what was observed, the threshold, grouping, scope and collection coverage.

Under **Investigate**, pick one of the retained example calls (up to 20 are kept) and open it with **Open timeline** or **Open sequence**. The example opens as its own report with the occurrence's span and log selected. Examples without a trace can still be inspected.

![Problem detail with why flagged](/screenshots/problems-detail.png)
*The error group “Storage returned 403 AccessDenied”: 2 affected calls in 2 traces, with “Why was this flagged?” open: the rule, the observed counts, the threshold, the grouping and the coverage.*

![Problem example opened as timeline](/screenshots/problems-example.png)
*An example opened as a timeline, with `upload_recording` selected and its logs on the task lanes `checksum` and `transfer`. “Back to Problems” returns to the detail.*

## Release regressions and “Check fixed release”

A regression's **Compare releases** tab shows the stored comparison it was detected from: baseline and candidate versions, every metric with its change and result. Choose another version under **Fixed release** and click **Check fixed release** to see whether a later release brought the latency back.

![Release regression with comparison](/screenshots/problems-regression.png)
*“upload_recording became slower in version 1.1.0” on `fxdc108d54-recording-worker`: p95 100 ms → 160 ms, +60%, from the stored comparison of 1.1.0 against 1.0.0.*
