---
description: Install Troubleshooting with Docker, send spans and correlated logs from Go or any OpenTelemetry SDK, and find your way around the five pages.
---

# Getting started

Troubleshooting shows what one request did across your services: its spans and its log records together, in a table, a timeline or a sequence diagram. On top of that it groups recurring problems and compares call statistics, latency percentiles and error groups between releases.

It is for developers and support engineers of Go services (or any OpenTelemetry-instrumented service) who need to answer three questions fast:

- **What happened in this execution?** [Log Analysis](./log-analysis) and [Timeline & Sequence](./timeline-sequence).
- **What keeps going wrong?** [Problems](./problems).
- **Did the new release make it better or worse?** [Releases](./releases).

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

The header has five pages, in this order:

| Page | Use it to |
| --- | --- |
| Log Analysis | Search spans and see their logs inline (the default page) |
| Timeline & Sequence | Browse execution shapes and open one execution as a timeline or sequence diagram |
| Problems | See recurring error groups, slow calls and release regressions of one service |
| Releases | Compare two releases, and view statistics of one population |
| Settings | Edit problem detection rules, view retention |

A page keeps its state when you switch away and back. Any page can open one execution. The browser address then names it (`#/executions?trace=…&from=…&to=…&span=…&view=…`), so you can copy the link and send it to a colleague. Links carry identifiers only, never log text.

::: info About the screenshots
The screenshots in this handbook were taken on 2026-11-12 from the regression fixture data set. Its service names carry a run prefix, for example `fx7e3d4841-recording-worker`. Times show in the browser's local time (UTC+1).
:::
