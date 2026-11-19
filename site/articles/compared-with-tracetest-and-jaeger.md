---
title: "How is Troubleshooting different from Tracetest or Jaeger?"
description: Troubleshooting is a runtime quality gate for pull requests. It checks every execution of your own test run against limits in your repository, such as database calls per request, while Tracetest asserts on the trace of a request it triggers and Jaeger stores and shows traces.
---

# How is Troubleshooting different from Tracetest or Jaeger?

All three work with OpenTelemetry traces. They answer different questions.

**Jaeger** (and Tempo, SigNoz and other tracing backends) stores traces and shows them. It answers "what did this request do?". It doesn't decide whether a pull request may be merged.

**Tracetest** runs trace-based tests. A test has a trigger, such as an HTTP or gRPC request it sends, and assertions on the spans of the trace that request produced ([Tracetest docs: tests](https://docs.tracetest.io/concepts/tests), [triggers](https://docs.tracetest.io/concepts/triggers)). It answers "did this request behave as specified?". Its open-source edition is Tracetest Core; the commercial cloud and enterprise products ended in 2024 ([announcement](https://tracetest.io/blog/end-of-life-announcement-for-tracetest-cloud)).

**Troubleshooting** is a quality gate. It doesn't send requests. Your existing tests are the workload: their spans carry a run id, and the gate checks **every execution** of that run against rules in your repository's `quality.yaml`. It answers "does this change keep its limits?":

| | Tracetest | Troubleshooting |
| --- | --- | --- |
| What runs the code | Tracetest's trigger (a request it sends) or a test tool it integrates with | Your own tests, unchanged; only their spans are tagged with a run id |
| What is checked | Assertions on the spans of one test's trace | Limits over all executions of an operation in the run: at most N database calls in **every** execution, a p95 latency, a service that must never be called, at most N failed executions |
| Too little data | – | "Not enough data" (exit code 2), never a pass: missing executions, spans whose parent wasn't recorded, lost child spans |
| Who can change the limits | The test definition | `rules-from: base` reads the rules from the pull request's base branch, so a pull request can't loosen its own limits |
| Why a limit exists | – | Requirements with an owner and a reason, and their history: when a requirement broke and which commit fixed it |
| When it fails | The failed assertion | The execution that broke the rule, as a timeline or a sequence of calls, plus a snapshot kept with the report |

The two can work together: Tracetest for detailed assertions about one request, Troubleshooting for limits that must hold across a whole test run, such as "no request makes more than three database calls".

What the table says about Tracetest comes from its documentation as of November 2026. If something here is out of date, please [open an issue](https://github.com/agerimex/troubleshooting/issues).

## See also

- [Quality gate](/guide/quality): the rules, the result and the GitHub Action.
- [Catch an N+1 query in a pull request](./catch-n-plus-one-in-a-pull-request): one failing change, followed through the gate.
