# Reference regression and fix fixture

This document describes data to generate through supported application senders. It is not an implemented sender or a recording of measured production behavior.

## Scenario and population

A recording request is accepted by recording-api. A recording-worker operation uploads the result. Some uploads emit a recorded error and diagnostic log. All observations are synthetic.

The measured operation is service recording-worker, operation upload_recording, span kind CONSUMER, environment fixture, installation reference-local and workload segment upload-small. Select the version recorded on that worker span. The API request and storage dependency are supporting evidence, not additional upload calls.

Use one generated UTC base time T on a whole-hour boundary. Version 1.0.0 uses [T, T+1h), version 1.1.0 uses [T+2h, T+3h), and version 1.1.1 uses [T+4h, T+5h). Put 1000 operation starts three seconds apart beginning five seconds into each window; each operation and its supporting records fit within that window. Comparison windows have identical duration.

Choose T sufficiently in the past that completed buckets can be finalized within the fixture retention policy, or exercise a controllable clock. All reference data is received within allowed lateness. Record readiness and finalized generations before asserting comparison eligibility.

## Recorded data

| Worker version | Canonical upload calls | Duration of each upload | ERROR uploads | OK uploads | UNSET uploads |
| --- | --- | --- | --- | --- | --- |
| 1.0.0 | 1000 | 100 ms | 10 | 990 | 0 |
| 1.1.0 | 1000 | 160 ms | 50 | 950 | 0 |
| 1.1.1 | 1000 | 105 ms | 10 | 990 | 0 |

Every call has a distinct valid TraceId and selected SpanId. Generate real protocol identifiers from isolated fixture aliases; never send the human-readable aliases as OTLP hexadecimal IDs. A replay reuses the exact original payload, identifiers and timestamps.

Each trace contains:

1. A recording-api SERVER parent span start_recording with status OK and duration 30 ms.
2. The selected recording-worker CONSUMER span upload_recording, starting after the request was accepted.
3. A CLIENT storage.PutObject dependency span within the upload, with a different operation identity so it does not inflate upload-call statistics.

Each selected span and emitted log records the worker version where applicable, environment, installation and workload segment. Logs also record their own service, operation, event timestamp, severity and stable source/event identity. Metadata on the API parent belongs to that API service; it is not used to fill worker metadata.

Every trace has an INFO request-accepted log associated with the API span and an INFO upload-start log associated with the worker span. Each failed upload additionally has:

- A recorded ERROR status and generic status message Upload failed.
- One exception event with exception.type StorageError, application frame storage.Upload and generic message Upload failed.
- A WARN log Upload retry 1 of 3.
- An ERROR log Storage returned 403 AccessDenied associated with the worker span.

Only the independent ERROR log contains the 403 AccessDenied diagnostic detail. It must not appear in the fixture's span attributes, exception text or status message. The distinction demonstrates why the combined report needs both signals.

Successful upload spans have OK status and an INFO completion log. These fixture statements describe the technical upload, not a verified customer-delivery business outcome.

## Expected statistics

For the selected worker population:

| Version | Observed calls/minute | Recorded error fraction | p50 | p95 | p99 | Exception occurrences | ERROR log occurrences |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1.0.0 | 16.6667 | 1 percent | 100 ms | 100 ms | 100 ms | 10 | 10 |
| 1.1.0 | 16.6667 | 5 percent | 160 ms | 160 ms | 160 ms | 50 | 50 |
| 1.1.1 | 16.6667 | 1 percent | 105 ms | 105 ms | 105 ms | 10 | 10 |

Each version also has the same number of ERROR-status span occurrences as failed uploads. Those span, exception and log occurrences are separate evidence types for the same fixture failures. Do not add their counts and claim three times as many failed uploads.

The exception group uses the same errors-v1 fingerprint across all versions: recording-worker, upload_recording, fixture, exception event, StorageError, storage.Upload. Version filters produce 10, 50 and 10 occurrences. Its known affected trace count equals those occurrence counts in this fixture because each failed trace has one exception event.

The latency plateaus intentionally give unambiguous expected values for an approximate percentile implementation. Apply the 1 ms fixture tolerance. The separate distribution and unequal-bucket scenarios exercise nonconstant samples.

## Expected comparisons

| Candidate against baseline | p95 difference | Relative p95 change | Error-fraction difference | Relative error change | Rule result |
| --- | --- | --- | --- | --- | --- |
| 1.1.0 against 1.0.0 | +60 ms | +60 percent | +4 percentage points | +400 percent | Both regression signals |
| 1.1.1 against 1.1.0 | -55 ms | -34.375 percent | -4 percentage points | -80 percent | Both improvement signals |
| 1.1.1 against 1.0.0 | +5 ms | +5 percent | 0 percentage points | 0 percent | Within thresholds |

Percent differences may be displayed rounded to two decimals. Calculate classifications using unrounded values under comparison-v1, retain the original numeric values in the report, and avoid interpreting display rounding as a changed threshold.

Opening a selected failed candidate trace must preserve the API parent and worker/dependency context plus related logs. The evidence supports checking storage authorization; it does not establish which credential or policy caused the rejection.

## Additional controlled cases

The individual feature files define smaller isolated cases for colliding SpanIds, replay, missing metadata, invalid durations, low sample sizes, missing source coverage, zero baselines, unequal bucket sizes, late arrivals and expired evidence.

Keep those records out of the main three-release population unless the relevant scenario explicitly adds them. The application's own telemetry is also outside the reference population. Use bounded readiness checks and fail with an explicit timeout; do not hide incomplete ingestion with arbitrary sleeps.

## Practical evaluation

After the controlled path works, use a representative real or replayed incident with another engineer. Preserve the measured service/operation and comparison context, record the existing investigation method, and assess whether the new workflow provides useful evidence or a better next check. Measure active effort before claiming a time saving. Passing the synthetic fixture does not prove market demand or general causal diagnosis.
