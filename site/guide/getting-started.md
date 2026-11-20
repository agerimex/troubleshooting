---
description: Install Troubleshooting with Docker, send spans and correlated logs from Go or any OpenTelemetry SDK, and find your way around the six pages - from investigating one request to checking runtime requirements in CI.
---

# Getting started

Troubleshooting helps you find out why a request failed or slowed down. It shows what one request did across your services, its database calls and its log records together, as a table, a timeline or a sequence of calls. It groups recurring problems and compares releases to show what changed. Investigating needs no rules or CI setup. If your team already knows what must stay true, the [quality gate](./quality) checks those requirements against the traces of pull-request and release test runs.

- **What happened in this request?** [Log Analysis](./log-analysis) and [Timeline & Sequence](./timeline-sequence). A worked example: [Investigate a slow request](./investigate-a-slow-request).
- **What keeps going wrong?** [Problems](./problems).
- **Did the new release make it better or worse?** [Releases](./releases).
- **Does this test run keep our known requirements?** [Quality gate](./quality), in pull requests and release test runs.

## How data gets in

```
instrumented Go app ── log-sender (spans + logs) ──gRPC :50055──▶ log-receiver ──▶ ClickHouse
any OTel SDK / Collector ───────── OTLP/gRPC :4317 ────────────────▲                   ▲
browser ── UI :8095 ── /api/* ──▶ log-analysis :8094 (REST) ─────────────────────────┘
```

- **Go services** use the `log-sender` library. It sends spans and log records over gRPC to the receiver on port 50055. Each log record carries the trace and span it was written in, so logs attach to the right span.
- **Any OpenTelemetry SDK or Collector** can send traces over OTLP/gRPC to port 4317.
- The receiver stores everything in ClickHouse. Raw spans and logs are kept 3 days, statistics 30 days.

## Install

You need Docker and `make`.

1. Clone [the repository](https://github.com/agerimex/troubleshooting) and run `make all`, then `make up`. The first ClickHouse start on an empty volume can take several minutes on Docker Desktop.
2. Open `http://localhost:8095` in your browser.
3. Send data. In a Go service, call `NewTracer("my-service", WithAddress("localhost:50055"))` from `log-sender` and attach the zerolog `NewLogWriter` with `TraceHook`, so every log line carries its trace and span. Any other service points its OpenTelemetry OTLP exporter at `localhost:4317`.
4. Add `WithResourceAttributes` with `service.version` and `deployment.environment.name`. Releases and Problems compare by these values.

## Navigation

The header has six pages, in this order:

| Page | Use it to |
| --- | --- |
| Log Analysis | Search spans and see their logs inline (the default page) |
| Timeline & Sequence | Browse execution shapes and open one execution as a timeline or sequence diagram |
| Problems | See recurring error groups, slow calls and release regressions of one service |
| Releases | Compare two releases, and view statistics of one population |
| Quality | Check a test run against rules from your repository, or open a CI result |
| Settings | Edit problem detection rules, view retention |

A page keeps its state when you switch away and back. Any page can open one execution. The browser address then names it (`#/executions?trace=…&from=…&to=…&span=…&view=…`), so you can copy the link and send it to a colleague. Links carry identifiers only, never log text.

::: info About the screenshots
The screenshots in this handbook were taken on 2026-11-18 from the regression fixture data set (run `dc108d54`) on the current `main`. Its service names carry the run prefix, for example `fxdc108d54-recording-worker`. Times show in the browser's local time (UTC+1).
:::
