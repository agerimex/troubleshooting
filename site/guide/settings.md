---
description: Edit the problem detection rules for errors, slow calls and release regressions, and read the effective data retention.
---

# Settings

Settings has two tabs: **Detection**, where you edit what counts as a problem, and **Data retention**, which is read-only.

## Detection rules

Three rules, each with its own checkbox:

| Rule | Settings | Default |
| --- | --- | --- |
| Error signals | Open after N affected calls in the window; group by operation and error signature (`errors-v1`, kept across releases) | 1 call |
| Slow calls | Duration above N ms per call; open after N slow calls in the window | 500 ms, 5 calls |
| Release regressions | p95 increase of at least N % **and** at least N ms; only generated comparisons with an eligible p95 count | 20 %, 20 ms |

The header shows the revision in force and when it was saved. Changing a value shows “Unsaved changes” with **Save rules** and **Discard changes**. A save applies from the next evaluation and creates a new revision; every revision is kept. If someone else saved in the meantime, the save is refused and you reload their revision first.

![Detection rules](/screenshots/settings-rules.png)
*The detection rules at revision 36.*

![Detection rules with unsaved change](/screenshots/settings-rules-edit.png)
*Slow-call duration changed to 120 ms, not yet saved.*

## Data retention

The retention tab shows the effective values read from the database: raw spans, raw logs, statistics, the statistics bucket (1 h) and the allowed lateness (1 day: late records still update a bucket for this long, then it is final). Retention is set on the receiver with `TRACE_RETENTION`, `LOG_RETENTION` and `STATS_RETENTION` and can't be changed from this page. Comparisons and problem counts stay available after their raw records expire, but their examples can no longer be opened.

![Data retention](/screenshots/settings-retention.png)
*Retention of the test stack (2 days raw, 30 days statistics). The default installation keeps raw records 3 days.*
