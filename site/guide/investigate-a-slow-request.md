---
description: A worked investigation with real product screenshots - a session start got slower between two versions; compare the releases, open a slow request, and find the extra database work in its timeline and sequence. Then add a runtime check so it can't come back.
---

# Investigate a slow request

A session start got slower in the new version. This page follows the investigation step by step, with screenshots of the product. You don't need to install anything to follow it.

The data is the regression fixture of this repository ([`fixtures/regression/qg.go`](https://github.com/agerimex/troubleshooting/blob/main/fixtures/regression/qg.go)): a `start_session` operation of a service `qg-api`, 200 starts in the version `baseline` and 200 in the version `candidate`. Service names carry the fixture run's prefix (`fxb7d3b725-`).

## 1. Compare the releases

**Releases → Compare releases.** Baseline and candidate are the same service and operation, each with its version:

| | Service | Operation | Version |
| --- | --- | --- | --- |
| Baseline | `fxb7d3b725-qg-api` | `start_session` | `baseline` |
| Candidate | `fxb7d3b725-qg-api` | `start_session` | `candidate` |

![Release comparison of start_session: p95 420 ms in the baseline version and 910 ms in the candidate, marked as a regression signal](/screenshots/investigate-compare.png)

The p95 of `start_session` rose from **420 ms to 910 ms** (+117 %), and the p50 from 384 to 874 ms. Both are marked **Regression signal**. The number of calls didn't change: every version handled 200 starts, so the operation became slower, not busier. p99 stays "insufficient evidence": 200 calls aren't enough for a 99th percentile.

A comparison points at what to investigate. It doesn't prove the cause, so the next step is to look at individual requests.

## 2. Open a slow request

**Slow candidate examples ≥ 910 ms** lists recorded executions of the new version at or above its p95, slowest first.

![The slowest candidate calls of start_session, each with Open report](/screenshots/investigate-examples.png)

**Open report** opens one of them in Timeline & Sequence, with a link back to the comparison.

## 3. Read the timeline

![Timeline of one slow start_session: check_permissions with twenty permission_lookup spans, insert_session, and GET /users to the directory service](/screenshots/investigate-timeline.png)

The execution has 25 spans. Under `check_permissions` there are **twenty `permission_lookup` database calls**, one per permission, followed by one `insert_session` and a `GET /users` call. In the baseline version the same operation makes one lookup and one insert, and no other call.

In this fixture the durations are set by the data generator. The timeline shows what work the new version does, not where each of the 490 extra milliseconds went. In your own application the span durations show both.

## 4. See who called whom

**Sequence** draws the same execution as calls between services:

![Sequence view: permission_lookup repeated 1 of 20, 2 of 20 and so on, from the API to an uninstrumented PostgreSQL](/screenshots/investigate-sequence-repeats.png)

Each `permission_lookup` is an arrow from the API to PostgreSQL, numbered *repeat 1 of 20*, *repeat 2 of 20* and so on. PostgreSQL appears as "not instrumented": the calls are recorded by the API's client spans.

![End of the sequence: insert_session to PostgreSQL, then GET /users to the directory service](/screenshots/investigate-sequence-end.png)

At the end, the API also calls the **directory service** (`GET /users`). The baseline version never calls it.

**What changed:** the new version reads permissions one by one, a query per permission, and it added a dependency on the directory service. That is the cause to fix.

## 5. Keep it from coming back

Once you know what must stay true, write it down next to the code as a runtime check. For this operation:

```yaml
# quality.yaml
version: 1
entry: {service: qg-api, operation: start_session, span_kind: server}
checks:
  - {id: db-calls, type: db_calls, max_per_execution: 3}
  - {id: no-directory, type: forbidden_call, to: qg-directory}
```

Checked against the same test run, the gate fails: "up to 21 database calls per execution, limit 3" and a call to the directory service "in 200 of 200 executions", each with a link to an execution that broke the rule. The fixed version passes. In CI the result becomes a comment on the pull request; see [Quality gate](./quality).

For a real example, see [the gate's comment on pull request #30](https://github.com/agerimex/troubleshooting/pull/30#issuecomment-6992712426). It checks this repository's own API, with its own limits and measurements, not the fixture above.

## Try it on your own data

- **You have a trace ID:** Timeline & Sequence → *Open one execution by trace id*. See [Timeline & Sequence](./timeline-sequence).
- **An operation got slower:** [Log Analysis](./log-analysis) finds slow spans in a window; [Releases](./releases) compares two versions or windows.
- **Something keeps failing:** [Problems](./problems) groups recurring errors and slow calls.
- **Getting data in:** [Getting started](./getting-started).
