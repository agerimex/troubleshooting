---
title: "Execution shapes: the distinct paths requests take through your services"
description: Group thousands of traces by their entry operation and the set of caller-to-callee calls they make, then see how often each shape runs, fails and how long it takes.
---

# Execution shapes: the distinct paths requests take through your services

A service that handled 30,000 requests yesterday didn't do 30,000 different things. Most requests follow one of a handful of paths: the normal one, the cached one, the one that fails at authorization, the one that retries storage.

Troubleshooting calls such a path a **shape**.

## What a shape is

A shape is the **entry operation** plus the **set of distinct caller → callee calls** an execution makes.

- Repeats don't split a shape: calling storage three times or once is the same shape.
- Call order doesn't split a shape either.
- An execution that fails early and makes fewer calls *is* a different shape, and that's the point: failures that take a different path show up as their own row.

## The sequence library

**Timeline & Sequence** opens on the library: the shapes of a window (the last 24 hours by default), ten per page. Each row shows the entry, the services it touches, the number of distinct calls, executions, the share that failed, and p50 / p95 / p99 of the root span's duration.

![Sequence library with expanded calls](/screenshots/library-calls.png)

Expand **Calls** to see which caller calls which callee. Filter by service, entry operation or “with failed executions”.

## From a shape to one execution

**Open sequence** with p50, p95, p99 or slowest opens a representative execution as a sequence diagram: services as lanes, calls as arrows, with start, end and ERROR-log markers.

![Sequence with client/server arrows](/screenshots/exec-sequence-clientserver.png)

A CLIENT span with exactly one SERVER child in another service is drawn as one arrow. A client whose peer sends no spans points to an “unknown peer”, so missing instrumentation is visible rather than silently dropped.

## Why it helps

- **New paths after a release** stand out as new rows.
- **Rare failing paths** aren't averaged away into a service-wide error rate.
- **The slow tail** of one path is one click away from a real execution to read.

Shapes are computed per hour and stored with the statistics, so the library stays fast over a full day of traffic.

→ Full reference: [Timeline & Sequence](../guide/timeline-sequence).
