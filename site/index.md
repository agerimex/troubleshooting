---
layout: home
title: Trace and log investigation for Go and OpenTelemetry
titleTemplate: false

hero:
  name: Troubleshooting
  text: Did the new release make it worse? See the calls that prove it.
  tagline: Open-source, self-hosted investigation for Go and OpenTelemetry services. Spans and logs of one execution in one view, recurring problems grouped, and releases compared by p95 latency and error groups, down to the example calls behind every number.
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: Compare releases
      link: /articles/compare-p95-between-releases
    - theme: alt
      text: GitHub
      link: https://github.com/agerimex/troubleshooting

features:
  - title: One execution, spans and logs together
    details: Every log record carries the trace and span it was written in, so a timeline or sequence diagram shows exactly which call logged what. No guessing by timestamp.
    link: /guide/timeline-sequence
  - title: Releases compared, with evidence
    details: Pick a baseline and a candidate release. Get call counts, error fraction and p50/p95/p99 per operation, the error groups of both sides, and the slow calls behind each number.
    link: /guide/releases
  - title: Recurring problems, not alert noise
    details: Error groups, slow calls and release regressions of one service, each with why it was flagged and retained example calls to open.
    link: /guide/problems
  - title: Execution shapes
    details: The distinct paths requests take through your services, with how often each runs, how often it fails and how long it takes.
    link: /articles/execution-shapes
  - title: Standard ingestion
    details: A small Go library for spans and correlated logs, or any OpenTelemetry SDK or Collector over OTLP/gRPC. Stored in ClickHouse.
    link: /guide/getting-started
  - title: Runs on your machine
    details: Two make commands start ClickHouse, the receiver, the API and the UI with Docker. Your telemetry stays with you.
    link: /guide/getting-started
---

<div class="home-shot">

![Release comparison: storage.PutObject p95 +66.7% in version 1.1.0](/screenshots/compare-result.png)

</div>

<style>
.home-shot { max-width: 1152px; margin: 48px auto 0; padding: 0 24px; }
.home-shot img { border: 1px solid var(--vp-c-divider); border-radius: 8px; }
</style>
