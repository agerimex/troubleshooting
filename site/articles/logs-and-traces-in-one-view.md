---
title: "Logs and traces in one view: correlating Go logs with OpenTelemetry spans"
description: Attach every zerolog line to the trace and span it was written in, then see each span's logs inline on a timeline. No matching by timestamp.
---

# Logs and traces in one view: correlating Go logs with OpenTelemetry spans

A trace tells you which call failed. The logs tell you why. Most setups keep them in two tools and join them by timestamp and service name, which works until two requests run at the same time.

The fix is simple: write the trace id and span id into every log record, at the moment it's written, and never infer them later.

## Write the span into each log line

With zerolog and the `log-sender` library:

```go
exporter, _ := sender.NewLogExporter("recording-worker")
w := sender.NewLogWriter(exporter, time.Second)
defer w.Close(context.Background())

logger := zerolog.New(io.MultiWriter(os.Stdout, w)).Hook(sender.TraceHook{})

ctx, span := tracer.Start(ctx, "upload_recording")
defer span.End()
logger.Error().Ctx(ctx).Msg("Storage returned 403")
```

`TraceHook` reads the span from the event's context and adds `trace_id`, `span_id` and a `record_id`. A log written outside any span simply has no span. It is never guessed.

The writer never blocks your application on the network. It buffers up to 10,000 records, sends them in the background, and retries unconfirmed records with the same `record_id`, so a retry is counted once.

## See each span's logs where they happened

In **Log Analysis**, expanding a span shows its child spans and, between them, the log records that recorded exactly that span, in time order:

![Expanded execution with inline logs](/screenshots/logs-expanded.png)

Open the same execution as a **timeline** and select a span: its logs appear on task lanes (by `task.id`), so concurrent work inside one span stays readable.

![Selected span with task lanes](/screenshots/exec-timeline-span.png)

Logs that name no span, or a span outside this trace and window, are shown in a separate trace-level group. Nothing is attached to a span it didn't come from.

## Share the investigation, not the logs

Every opened execution has an address with identifiers only (`#/executions?trace=…&span=…`), so a link in a ticket never leaks log text. **Export HTML** and **Export JSON** save the full report when you need to attach it.

→ Full reference: [Log Analysis](../guide/log-analysis) and [Timeline & Sequence](../guide/timeline-sequence).
