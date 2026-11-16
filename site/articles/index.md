---
description: Short articles on investigating Go and OpenTelemetry services from their traces and logs.
---

# Articles

Short, practical pieces on questions that traces and logs can answer, and how Troubleshooting answers them.

- [How to compare p95 latency between two releases from OpenTelemetry traces](./compare-p95-between-releases): what a fair release comparison needs, and how to get from a percentage to the slow calls behind it.
- [Logs and traces in one view: correlating Go logs with OpenTelemetry spans](./logs-and-traces-in-one-view): attach every zerolog line to the span it was written in, and stop matching by timestamp.
- [Execution shapes: the distinct paths requests take through your services](./execution-shapes): group thousands of traces into a handful of shapes and see which one fails.
- [Recurring errors, not single errors: grouping failures across releases](./recurring-error-groups): turn ERROR spans, exceptions and ERROR logs into a short list of problems with examples.
