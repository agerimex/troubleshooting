# Verification: data consistency and report speed (13 October 2026)

Test stack (native receiver and API, test ClickHouse capped at ~2.5 GB in a
4 GB WSL VM), with 33 M spans and 15.5 M logs of load data (`ld-*`, 24 h,
one busy hour) and the regression fixture (run `182a148e`). Branch
`feat/milestone-26-10`, uncommitted, with the paging, count, log search,
statistics memory and sequence library changes of the same day.

## Consistency: 23/23 checks against independent SQL

Every report was compared with plain SQL straight on ClickHouse
(`uniqExact`, `quantileExact`), not with the API's own queries. The load
data has no conflicting copies, so "distinct (TraceId, SpanId) among the
matching rows" is the truth; conflicts and replays are covered by the
fixture (below). Script: session scratchpad `verify/verify.mjs`.

| Area | Checked | Result |
| --- | --- | --- |
| Log Analysis totals | root / any depth in the busy hour (589 450 / 4 320 481), payments errors and login roots over 6 h | equal |
| Paging | every page of 5 028 rows (1000 per page) and of 3 303 rows (25 per page, 133 pages) | each row exactly once, in order, all matching the filter, sum = total |
| Child counts | 25 spans of a page; 5 expanded | equal to SQL; expansions return exactly that many children |
| Trace reports | 20 random traces | spans and logs equal |
| Log search | ERROR logs over all pages | every record once, equal |
| Statistics | calls, errors, p50/p95/p99 of two operations, stored hours and raw partial hours | counts equal; percentiles within 0.3 % of exact (t-digest) |
| Statistics per operation / version | all ld-payments operations over 24 h; 3.4.0 vs 3.5.0 | equal |
| Error groups | occurrences vs ERROR spans and ERROR logs (63 916 each) | equal |
| Late arrivals | the hour where 59 967 rows arrived after the 24 h lateness | excluded from counts and reported as a notice |
| Sequence library | executions per entry = traces with that root (4.19 M); failed executions = traces with a failed span (52 325); 24 examples are executions of their shape; percentiles vs exact; a stored hour vs the same hour from raw spans | equal / within 0.1 % (examples within 2.4 % of the percentile) / identical shapes and counts |

The regression fixture passes every check before and after a replay
(122 PASS lines, including SEQ-SHAPE-001 and the conflict, collision and
late-arrival scenarios).

## Speed (API, median of 5; first run in parentheses when slower)

| Report | 1 h | 6 h | 24 h |
| --- | --- | --- | --- |
| Log Analysis page (root spans) | 0.16 s | 0.21 s (page 41: 0.19 s) | 0.20 s |
| Filtered page (payments errors / rare login errors) | | | 0.22 s / 0.42 s |
| Total (root spans) | 0.73 s | 1.98 s | fails: >50 M rows to read |
| Children of a span / trace report | 0.1 s / 0.08 s | | |
| Log search (ERROR / message) | | 0.06 s (all logs) | 0.09 s / 0.32 s |
| Sequence library | 0.02 s | | 0.03 s (last 20 min from raw: 0.07 s) |
| Statistics (ld-gateway with series / by version) | | | 4.0 s / 3.9 s |
| Error groups (ld-payments) | | | 0.7 s |
| Compare 3.4.0 vs 3.5.0 | | | 8.6 s |
| Problems (ld-payments) | | 6.3 s | 3.4 s |

Browser (real data): app start 0.5 s; Log Analysis next page 0.36 s, total
of 6 h shown after 2.4 s; library 0.07 s; opening a p95 sequence 0.17 s;
back 0.05 s; a shape's calls 0.14 s. No console errors.

## Findings

1. **Error groups split on ids after a space.** errors-v1 keeps numbers (so
   403 and 500 stay apart) and removes ids only in forms like `order-123` or
   `order=123`. "…failed for order 3684958: timeout…" is one group per
   order: ~32 000 groups for ld-payments in 6 h. Counts are right; the lists
   (Problems, Releases error groups) become unusable for such messages.
   Fix: a contract change (errors-v2), e.g. also `order 123`, and numbers of
   five or more digits as `<num>`.
2. **The refresher keeps ClickHouse busy.** Every 5 minutes it recomputes
   the content hash of every hour whose raw data is retained (3.5 s on
   average here): 116 hashes, ~400 s of ClickHouse time per 30 minutes,
   even when nothing changed. Fix: skip an hour when no row arrived after its
   revision's receipt cutoff (one cheap `max(ReceivedTime)` per hour).
3. **Statistics, comparisons and problems are slow (3–9 s) because of the
   receipt notices.** Counting late and unknown-receipt records groups every
   raw span of the window per span (8 s, 21 M rows); the stored statistics
   themselves are fast. Fix: count those records per hour at refresh time
   and store them with the revision.
4. **Totals of large windows.** 2 s for 6 h; 24 h over 33 M spans exceeds
   the 50 M rows-to-read guard ("Total unavailable"; the list works).

## After the fixes (same day)

Implemented findings 1–3, plus a Problems fix found on the way; the 23
consistency checks and the regression fixture (with replay, 122 PASS) pass
again.

1. **Error groups**: errors-v1's configured identifiers now include an id
   after an identifier word (`order 3684958` → `order <id>`) and numbers of
   six or more digits (`<num>`); status codes, durations and counts stay
   (M2-ERR-002 passes). The set is versioned (`message-rules-2` in the
   statistics definitions), so every provisional hour was rebuilt once;
   final hours keep their old groups. ld-payments over 6 h: 31 967 → 25
   groups, occurrences still equal to the error spans and logs.
2. **Refresher**: before hashing an hour it reads a fingerprint (row count
   and latest receipt time of the rows the hash reads, ~0.1 s); an hour whose
   fingerprint hasn't changed since it was last found unchanged or rebuilt is
   not hashed. A quiet cycle hashed 1 of 25 hours (the one still receiving
   data) instead of all.
3. **Receipt notices**: only spans with a row received late or without a
   receipt time are grouped (exact: no other span can count).
4. **Problems slow calls** failed for 6 h windows (rows-to-read limit, made
   worse by the statistics memory fix): a preselected statistics query is
   now one grouping of the preselected spans, and slow calls and slow
   examples preselect by duration.

| Report | Before | After |
| --- | --- | --- |
| Statistics, ld-gateway 24 h with series | 4.0 s | 1.45 s |
| Statistics by version, 24 h | 3.9 s | 1.3 s |
| Statistics, one operation, 6 h | 8.8 s | 0.64 s |
| Compare 3.4.0 vs 3.5.0 | 8.6 s | 1.3 s |
| Error groups, ld-payments 24 h | 0.7 s | 0.27 s |
| Problems, ld-payments 6 h | 6.3 s, slow calls failed | 8.5 s, complete |
| Problems, ld-payments 24 h | 3.4 s, slow calls failed | 2.8 s, slow calls "timeout" (rows-to-read limit), errors and regressions complete |

Still open: Problems' slow calls over windows with many slow spans (here
~200 000 in 6 h during the incident) build full canonical records for each;
a lean slow-call query reading only the needed columns, or slow-call counts
stored per hour, would make them fast. Totals of 24 h windows still exceed
the rows-to-read guard.
