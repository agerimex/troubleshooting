---
description: Browse the distinct execution shapes of a window and open one execution as a timeline or a sequence diagram with its logs.
---

# Timeline & Sequence

This page opens on the **sequence library**, the list of distinct execution shapes in a window. From there, or from any other page, you open one execution as a Timeline or a Sequence.

## Sequence library

A shape is the entry operation plus the set of distinct caller → callee calls it makes. Repeats and call order don't split a shape. An execution that fails early and makes fewer calls is another shape. Each row shows:

- **Entry**: operation and the services the shape touches.
- **Calls**: the number of distinct calls. The arrow expands them.
- **Executions** and **Failed**: traces whose root span started in the window, and the share that failed.
- **p50 / p95 / p99** of the root span's duration. A percentile needs at least 20, 200 or 1000 executions; with fewer it shows “—”.
- **Open sequence**: p50, p95, p99 or slowest opens a representative execution as a sequence.

The window defaults to the last 24 hours. Service, entry operation and “with failed executions” filter on the server. Ten rows per page. A notice says when buckets still come from provisional statistics.

![Sequence library with expanded calls](/screenshots/library-calls.png)
*The library of `fxdc108d54-qg-api`, 10:00–11:00: three shapes of `start_session`. The first is expanded to its three distinct calls (219 executions, none failed); the third also calls the directory service.*

## Open one execution by trace id

At the bottom of the library, **Open one execution by trace id** takes a 32-hex-digit trace id and reads it within the From/To window above.

![Trace id lookup](/screenshots/library-lookup.png)
*Exact lookup by trace id.*

## The execution report

An opened execution is one combined report of the trace's spans and logs:

- **Header**: entry operation, service, trace id, the exact scope (one trace within a window) and when this revision was generated.
- **Completeness**: “Complete” means every source returned all matching records. Each section (spans, logs) has its own status and its own Retry.
- **Regenerate** builds a new revision; **Widen by 1 h on each side** reads a wider window.
- **Investigation link**, **Export JSON** and **Export HTML** share or save the report.
- **Back to …** returns to the page it was opened from, in the same state.

## Timeline

Each span is a bar on a common time axis, indented under its parent, with status, duration and its ERROR log count. Logs of the trace are listed on the right with their offset from the trace start and the span that recorded them.

![Timeline with trace logs](/screenshots/exec-timeline.png)
*Timeline of the failed `publish_recording` with all 12 trace logs.*

Click a span to select it. The right panel then shows only that span's logs: **Task lanes** place each log on a lane by its `task.id` (dot INFO, diamond WARN, cross ERROR), **Log list** lists them.

![Selected span with task lanes](/screenshots/exec-timeline-span.png)
*`upload_recording` selected: its logs on the task lanes `checksum` and `transfer`, with the ERROR “Storage returned 403 AccessDenied” in `transfer`.*

## Sequence

The Sequence view draws services as lanes and calls as arrows. A CLIENT span with exactly one SERVER child in another service is one arrow; a client whose peer sends no spans points to “unknown peer”. Start, end and ERROR-log markers sit on each service's lane. Vertical spacing shows order, not elapsed time. A notice points out overlapping operations. At most 6 lanes and 80 marks fit on a page.

![Sequence view](/screenshots/exec-sequence.png)
*A failed `start_recording` as a sequence: the API and worker lanes, start and end markers, and the ERROR log flagged as recorded outside its span's interval.*

![Sequence with client/server arrows](/screenshots/exec-sequence-clientserver.png)
*A trace with instrumented peers: `GenerateThumbnail` and `PutObject` arrows between three services.*

## Span evidence

**Events and links** on a selected span opens its evidence: status message, exception type and message, stack trace, and links to other traces. If no events were recorded, it says so, without claiming no error happened.

![Span evidence dialog](/screenshots/exec-evidence.png)
*The evidence of a failed `upload_recording`: its status message and ids, no events recorded (which doesn't show that no error happened), and its link to another trace.*
