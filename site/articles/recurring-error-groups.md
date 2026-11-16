---
title: "Recurring errors, not single errors: grouping failures across releases"
description: Group ERROR spans, exception events and ERROR logs by operation and error signature, count affected calls, and keep the groups stable across releases.
---

# Recurring errors, not single errors: grouping failures across releases

An error rate says something is failing. It doesn't say *what*, whether it's one failure or five, or whether it was already there before the release.

Troubleshooting turns failures into **error groups** and lists the ones that keep happening as **problems**.

## How errors are grouped

Three kinds of evidence are grouped separately, because they mean different things:

- **ERROR spans**: a call ended with error status.
- **Exception events**: a call recorded an exception (type, message, stack).
- **ERROR logs**: a log record at ERROR severity.

Each group is keyed by operation and an **error signature**: the message with identifiers replaced: UUIDs, long hex ids, ids after words like `order` or `request`, and numbers of six or more digits (rule set `errors-v1`). Short numbers such as status code 403 stay, so “returned 403” and “returned 500” remain different groups. The same failure in version 1.0.0 and 1.1.0 lands in the same group, so you can tell “new in this release” from “recurring in both”.

The measure is **affected calls**, not records: a call that logged the same error ten times counts once. Errors whose records name no span are listed as “Call attribution unavailable” and never cross a threshold on their own.

## Problems: the short list

**Problems** evaluates one service and window with the current detection rules and lists what crosses them: error groups, slow calls and release regressions.

![Problem list](/screenshots/problems-list.png)

**Investigate** shows the counts, the versions the problem was seen in, first and last occurrence, and **Why was this flagged?**: the rule, what was observed, the threshold, the grouping and the collection coverage.

![Problem detail with why flagged](/screenshots/problems-detail.png)

Up to 20 example calls are retained. Each opens as a full execution with the failing span and log selected.

## Tune what counts as a problem

**Settings → Detection** holds the rules: errors open after N affected calls, slow calls above N ms after N occurrences, release regressions at a p95 increase of at least N % **and** N ms. Every saved revision is kept, and a save never silently overwrites someone else's.

→ Full reference: [Problems](../guide/problems) and [Settings](../guide/settings).
