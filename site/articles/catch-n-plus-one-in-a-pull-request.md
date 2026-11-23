---
title: "Catch an N+1 query in a pull request, from the test run's traces"
description: A change passes every functional test and still adds a query per item and a call to a legacy service. Walk through how the quality gate fails the pull request and shows the execution behind it, with real output from CI.
---

# Catch an N+1 query in a pull request, from the test run's traces

A change can pass every functional test and still make the service worse. It reads prices one by one instead of in one query. It calls a legacy service that was supposed to be gone. The response is the same, so the tests stay green, and the reviewer has to notice it in the diff, if they notice it at all. That's true for a change written by a person and even more for one written by a coding agent.

The test run already contains the answer. Its traces record every database call and every call to another service. This article follows one such change through Troubleshooting's [quality gate](../guide/quality), from the change to the evidence. Everything below is real output from one run of this repository's CI ([run 44079936899](https://github.com/agerimex/troubleshooting/actions/runs/44079936899)), where the example service [`examples/quality-gate`](https://github.com/agerimex/troubleshooting/tree/main/examples/quality-gate) goes through the gate on every push. The command output is shortened (evidence lines left out); the comment and the report are rendered from that run's `result.json`, without links, as the action now writes them for a Troubleshooting started inside the job.

## 1. The change

`GET /orders/{id}` reads an order's items and then their prices, in one query:

```go
defer s.query(ctx, "SELECT prices WHERE item_id IN (...)")()
```

The change reads each price on its own, and asks the `legacy-pricing` service for discounts:

```go
for _, id := range ids {
	end := s.query(ctx, "SELECT prices WHERE item_id = ?")
	out[id] = price(id)
	end()
}
// GET /discounts on legacy-pricing
```

The order and its total come out the same. The tests still pass: 40 orders with 8 items each, and an unknown order that answers 404.

## 2. The rules, kept next to the code

The service's limits are written down in `quality.yaml`, in its repository:

```yaml
version: 1
entry:
  service: example-orders
  operation: GET /orders/{id}
  span_kind: server
checks:
  - id: orders-p95
    type: latency
    max_ms: 250
    target_ms: 100
  - id: orders-db-calls
    type: db_calls
    max_per_execution: 3
    target_per_execution: 2
  - id: no-legacy-pricing
    type: forbidden_call
    to: legacy-pricing
```

An **execution** is one `GET /orders/{id}` server span with everything below it in its trace. `db_calls` counts the client spans with `db.system` in each execution, and the limit holds for **every** execution, not on average. `forbidden_call` fails if any execution calls `legacy-pricing`. SQL text is never parsed: the names in the result are the spans' own names.

## 3. The CI job

The job starts a private Troubleshooting, runs the tests with spans going to it, and checks the run. The application needs no change for this: the OpenTelemetry SDK reads the endpoint and the run id from the standard environment variables.

```yaml
- id: troubleshooting
  uses: agerimex/troubleshooting/quality-check/stack@quality-check/v0.1.1
- run: go test ./...
  env:
    OTEL_EXPORTER_OTLP_ENDPOINT: ${{ steps.troubleshooting.outputs.otlp-endpoint }}
    OTEL_EXPORTER_OTLP_INSECURE: "true"
    OTEL_RESOURCE_ATTRIBUTES: troubleshooting.run=ci-${{ github.run_id }}-${{ github.run_attempt }}
- uses: agerimex/troubleshooting/quality-check@quality-check/v0.1.1
  with:
    url: ${{ steps.troubleshooting.outputs.url }}
    ui: ${{ steps.troubleshooting.outputs.ui }}
    run: ci-${{ github.run_id }}-${{ github.run_attempt }}
    rules: quality.yaml
    rules-from: base
```

`rules-from: base` takes `quality.yaml` from the pull request's base branch, so the change can't raise its own limit to 9 and pass.

## 4. The result

The gate fails, with two failed checks and the reason for each:

```
Quality gate: FAILED
run example-44079936899-1-regression, baseline example-44079936899-1, 3 checks
commit 4b85f7689e2b (main) · rules examples/quality-gate/quality.yaml, sha256 d80ff3781a3d

PASS    orders-p95  example-orders / GET /orders/{id} (server)
        p95 11.9 ms, limit 250 ms, 41 executions
        baseline: 2.2 ms (41 executions)

FAIL    orders-db-calls  example-orders / GET /orders/{id} (server)
        up to 9 database calls per execution, limit 3, 41 executions, 40 over the limit
        baseline: 2 (41 executions)
        target 2 per execution not met yet (advisory)
        SELECT prices WHERE item_id = ? × 8
        SELECT order_items × 1
        evidence: execution with the most database calls: 9

FAIL    no-legacy-pricing  example-orders / GET /orders/{id} (server)
        call to legacy-pricing in 40 of 41 executions
        baseline: 0 (41 executions)
        example-orders → legacy-pricing in 40 executions
        evidence: example-orders → legacy-pricing (GET /discounts)
```

Each line can be checked against the change:

- **9 database calls**: one `SELECT order_items` and eight `SELECT prices WHERE item_id = ?`, one per item. Before the change it was 2.
- **40 of 41 executions**: the 404 request answers before it reads anything, so it makes no database call and doesn't call `legacy-pricing`. It counts as an execution, but it doesn't break either limit.
- **The p95 passes**: 11.9 ms is well inside 250 ms. A latency limit alone would have let this change through, because the test database is fast. The extra work shows up in the call counts long before it shows up in latency.

## 5. The pull request comment

The same result goes to the job summary and, on a pull request, into one comment. The comment is updated on every run instead of adding a new one. This is the Markdown the action writes:

::: info Comment on the pull request
**❌ Quality gate: Failed**

Run `example-44079936899-1-regression` against baseline `example-44079936899-1`, 3 checks. commit 4b85f7689e2b (main) · rules examples/quality-gate/quality.yaml, sha256 d80ff3781a3d. [CI run](https://github.com/agerimex/troubleshooting/actions/runs/44079936899/attempts/1)

| | Check | Value | Limit | Baseline | Executions |
|---|---|---|---|---|---|
| ✅ | **orders-p95**<br>example-orders / GET /orders/{id} (server) | 11.9 ms | 250 ms (target 100 ms, met) | 2.2 ms | 41 |
| ❌ | **orders-db-calls**<br>example-orders / GET /orders/{id} (server) | 9 | 3 (target 2, open) | 2 | 41 |
| ❌ | **no-legacy-pricing**<br>example-orders / GET /orders/{id} (server) | 40 | 0 | 0 | 41 |

**orders-db-calls** FAIL: up to 9 database calls per execution, limit 3, 41 executions, 40 over the limit
- target 2 per execution not met yet (advisory)
- SELECT prices WHERE item\_id = ? × 8
- SELECT order\_items × 1
- Evidence: execution with the most database calls: 9, trace `09aa7dedc4bb0a64d39881e733a91f6d`; its spans are in report.html

**no-legacy-pricing** FAIL: call to legacy-pricing in 40 of 41 executions
- example-orders → legacy-pricing in 40 executions
- Evidence: example-orders → legacy-pricing (GET /discounts), trace `09aa7dedc4bb0a64d39881e733a91f6d`; its spans are in report.html

<sub>quality-gate-v1 · exact nearest-rank percentile of entry span durations · evaluated 2026-11-17T12:08:25Z. Statuses describe this run only; a pass is "not observed in this run", not a guarantee.</sub>
:::

The comment names exactly what was tested: the commit, the rules file and its SHA-256, and the CI run.

## 6. The evidence

The Troubleshooting started inside the job is gone when the job ends, so the comment has no links to it. Instead, `report.html` in the job's artifact keeps a snapshot of each evidence execution. This is the one behind `orders-db-calls`:

[![HTML report of the failed gate: orders-db-calls with the snapshot of the execution with 9 database calls](/articles/quality-report-regression.webp)](/articles/quality-report-regression.webp)
*The `orders-db-calls` row of the report from the CI run (the other two checks hidden here). The snapshot shows the server span, one `SELECT order_items`, eight `SELECT prices WHERE item_id = ?` one after another, and the call to `legacy-pricing` at the end.*

The snapshot shows the execution as it ran: the price queries one after another, each 1.1 ms, and `GET /discounts` at the end. A reviewer doesn't need to reproduce anything to see it.

With a Troubleshooting that keeps running, for example the one your team already uses, the evidence is a link instead. It opens the execution in **Timeline & Sequence** with the span selected, and **Open a result file** on the [Quality page](../guide/quality#the-quality-page) shows the same result in the UI.

## 7. Fix and rerun

The fix restores the single query and removes the call. The output below comes from the same tests on the code without the change, which is what the fix brings back:

```
Quality gate: PASSED
run example-44079936899-1, 3 checks

PASS    orders-p95  example-orders / GET /orders/{id} (server)
        p95 2.2 ms, limit 250 ms, 41 executions

PASS    orders-db-calls  example-orders / GET /orders/{id} (server)
        up to 2 database calls per execution, limit 3, 41 executions
        SELECT order_items × 1
        SELECT prices WHERE item_id IN (...) × 1

PASS    no-legacy-pricing  example-orders / GET /orders/{id} (server)
        no call to legacy-pricing in 41 executions
```

On a pull request, the same comment is updated to this result. The target of 2 database calls is now met too. Targets are advisory and never change the gate.

This repository's CI runs both versions on every push: the planted change must fail and the original must pass. If the gate ever let the change through, the build would fail.

## What it can't tell you

- **Only what the tests executed.** A code path the tests don't reach isn't measured. A pass means "not observed in this run", not "never happens".
- **Only what is instrumented.** A database call without a client span isn't counted. A missing span can't make a check pass, though: if a span below the entry names a parent that was never recorded, the execution is incomplete, and the call checks report "not enough data" instead of a pass.
- **Not a load test.** 41 requests from one test process say nothing about behaviour under load. The call counts are what carry over: 9 queries per request stay 9 queries per request in production.
- **The baseline is named by hand.** Here it is the run of the code before the change, in the same job. To compare against `main`, tag a run on `main` and pass its id as `baseline`. There is no automatic "latest main" yet.

## Try it

- The [quality gate handbook page](../guide/quality) has the rules, the action's inputs and how to read a result.
- [`examples/quality-gate`](https://github.com/agerimex/troubleshooting/tree/main/examples/quality-gate) is the service from this article, with its `quality.yaml` and tests.
- The contract, `quality-gate-v1`, is in [`docs/quality-gate.md`](https://github.com/agerimex/troubleshooting/blob/main/docs/quality-gate.md).
