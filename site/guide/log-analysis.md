---
description: Search spans in a time window and expand each one into its child spans and the log records written inside it.
---

# Log Analysis

Log Analysis is the default page: one table of spans in which each span expands into its child spans and the log records written inside it.

## Search

- **From / To** set a half-open window `[From, To)`. An empty To means now. The line under the fields shows the exact window that was searched.
- **Root spans** lists only entry spans. **Any depth** lists every matching span. Filtering switches to Any depth unless you picked a scope yourself.
- **Trace id** + **Open timeline** jumps straight to one execution.
- The header filters select spans by their own name, status (`ok` / `error` / `unset`) and service. They never search log text.
- The chips above the table (`msg`, `service`) hide or show columns.

![Log Analysis: spans of the recording services](/screenshots/logs-table.png)
*Spans of the `fxdc108d54-recording` services since 11:52. A service filter switches the scope to “Any depth”, so the worker's `upload_recording` is listed next to the two `start_recording` executions.*

## Expanding a span

Click the arrow on a row. Its child spans appear, and between them the log records that recorded exactly this span, in time order. Each log row shows its severity, message and “recorded this span”. Logs that name no span, or a span not in this trace, form a separate group under the root span. An arrow appears only on spans that have child spans or logs of their own.

![Expanded execution with inline logs](/screenshots/logs-expanded.png)
*The failed execution expanded: `start_recording` with its INFO log, and the worker's `upload_recording` with its INFO, WARN and ERROR logs inline. Logs of the trace that name no span are grouped under “Trace-level / unmatched-span logs”.*

## Row actions

Every row, span or log, has the same actions. Open them with the `…` button, a right-click or the keyboard (Enter, arrows, Escape):

| Action | What it does |
| --- | --- |
| Open timeline | Opens the execution as a timeline, with this span or log selected |
| Open sequence | Opens the execution as a sequence diagram |
| Copy investigation link | Copies a link with identifiers only. Without clipboard permission it shows the link to select |
| Record details / Events and links | Shows the record, or the span's events, exception and links |

![Row action menu](/screenshots/logs-actions.png)
*The actions of the failed `upload_recording` span.*

## Finding failures

Set the status filter to `error` to see only failed spans. Paging moves forward with Next. The total (“Rows 1–1 of 1”) is counted once per search; if the count fails, the rows stay and the page says “Total unavailable”.

![Status filter error](/screenshots/logs-errors.png)
*Status filter `error` on the `fxdc108d54-recording` services: one failed span.*
