---
title: How to compare p95 latency between two releases from OpenTelemetry traces
description: A fair release comparison from traces needs matching populations, enough calls per percentile and the example calls behind the number. Here is how to do it with Troubleshooting.
---

# How to compare p95 latency between two releases from OpenTelemetry traces

“Did version 1.1.0 make uploads slower?” sounds like one number. In practice a dashboard comparing two p95 lines rarely settles it, for three reasons:

1. **The two sides aren't the same population.** Last Tuesday's traffic against today's, all environments mixed, one release still rolling out.
2. **The percentile rests on too few calls.** A p95 from 40 calls is two calls.
3. **A number is not evidence.** Even a clear +60% leaves the next question open: *which* calls were slow, and what did they do?

Here is how Troubleshooting handles each one.

## 1. Tag spans with the release

Comparisons select by resource attributes. Set `service.version` (and, if you have several, `deployment.environment.name`) on every service:

```go
tracer, err := sender.NewTracer("recording-worker",
    sender.WithAddress("localhost:50055"),
    sender.WithResourceAttributes(map[string]string{
        sender.AttrServiceVersion: "1.1.0",
        sender.AttrEnvironment:    "production",
    }),
)
```

With any other OpenTelemetry SDK, set the same two resource attributes and export over OTLP/gRPC to port 4317.

## 2. Define two populations, not two lines

On **Releases → Compare releases**, a *population* is a service, a time window and optional dimensions: operation, span kind, environment, version, installation and workload segment. Each dimension is *any*, *is* a value, or *not recorded*. Fill a **Baseline** (version 1.0.0, 10:00–12:00) and a **Candidate** (version 1.1.0, 12:00–14:00).

![Comparison form](/screenshots/compare-form.png)

Version text has no order, so you decide which side is the baseline. **Swap** and **Copy baseline filters to candidate** keep the two sides identical except for what you're testing.

## 3. Read the comparison per operation

The result lists every operation and span kind with recorded calls, call rate, error fraction, p50, p95 and p99, each with its absolute and relative change and a result:

| Result | Meaning |
| --- | --- |
| Regression signal | The change crosses the comparison rule's thresholds |
| Within threshold | A change, but below the thresholds |
| Descriptive only | Shown for context (call volume); never a signal on its own |

![Comparison result](/screenshots/compare-result.png)

A percentile is shown only when it rests on enough valid durations: 20 for p50, 200 for p95 and 1000 for p99. With fewer, you see “—”, not a misleading number.

Signals are heuristics: they point to what to investigate, not proof that the deployment caused the change. The analysis is stored with its revision and rule versions, so the same link shows the same result next week.

## 4. Open the calls behind the number

Under a metric, **Slow candidate examples** lists observed calls at or above the candidate's p95, slowest first. **Open report** opens one as a full execution: spans and logs on a timeline.

![Slow candidate examples](/screenshots/compare-examples.png)

This is the step most tools skip, and the one that tells you *why* `storage.PutObject` went from 90 ms to 150 ms.

## 5. Check the fix

A stored comparison whose p95 increase crosses the regression rule appears on the **Problems** page as a release regression. Its **Check fixed release** compares a later version against the same baseline, so you can see whether 1.1.1 brought the latency back.

→ Full reference: [Releases](../guide/releases) and [Problems](../guide/problems).
