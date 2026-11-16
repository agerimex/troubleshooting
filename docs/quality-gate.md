# Quality gate (milestone 3, stage 1)

Check one **test run** against rules kept in your repository, and get the
execution behind every failure. Contract `quality-gate-v1`. Acceptance scope:
`features/milestone_3/quality_gate.feature`; scenario-to-test map:
`docs/milestone3-completion.md`.

This is the first, small slice of the direction proposed in
`spec-16.11/Troubleshooting_Implementation_Spec` (not committed). From that
spec it keeps: separate pass / fail / not-enough-data states, limits apart
from advisory targets, a baseline that is shown but never decides, call counts
per execution (never from shape edges), and "not observed" instead of
"impossible". It leaves out for now: imported, digest-pinned requirement
revisions, a run registry and lifecycle, stored evaluations, a Quality page,
load and outage experiments, and the CI trust model.

## How it works

1. Tag the spans of a test run with **`troubleshooting.run`**, preferably as
   a resource attribute so the application needs no code change:

   ```
   OTEL_RESOURCE_ATTRIBUTES=troubleshooting.run=pr-42-1
   ```

   With log-sender: `sender.WithResourceAttributes(map[string]string{"troubleshooting.run": "pr-42-1"})`.
   A span attribute of the same name works too. Only the entry service needs
   it: spans of downstream services belong to the run through their recorded
   parent in the same trace.

2. Write the rules (`quality.yaml`; example in
   `quality-check/quality.example.yaml`):

   ```yaml
   version: 1
   entry:                       # the operation an execution starts with
     service: recording-api
     operation: start_session
     span_kind: server          # optional
   checks:
     - id: startup-p95
       type: latency
       percentile: 95           # default 95
       max_ms: 800              # the limit: decides the gate
       target_ms: 500           # optional, advisory
       min_executions: 20       # default 20 for latency, 1 otherwise
     - id: db-calls
       type: db_calls
       max_per_execution: 3
       target_per_execution: 2  # optional, advisory
     - id: no-directory
       type: forbidden_call
       to: directory-api
       from: recording-api      # optional: only calls from this service
     - id: report-p99
       type: latency
       entry: {service: report-api, operation: GET /report}   # per-check entry
       percentile: 99
       max_ms: 2000
       advisory: true           # shown, never fails the gate
   ```

   Unknown keys and duplicate keys are errors (a misspelled limit is never
   silently ignored).

3. Run the tests against your instrumented application, then:

   ```
   go install github.com/agerimex/troubleshooting/quality-check/cmd/troubleshooting-check@latest
   troubleshooting-check --url http://localhost:8095 --rules quality.yaml \
       --run pr-42-1 --baseline main-118 --since 1h --json result.json
   ```

   Exit code: **0** passed, **1** failed, **2** unresolved (not enough
   data), **3** error (no usable result: invalid rules, unreachable API, …).
   `--url` is the UI origin (which serves `/api`) or the API itself; `--ui`
   sets the origin of evidence links if it differs. `--from`/`--to` (RFC 3339)
   replace `--since`; `--baseline-since`/`--baseline-from`/`--baseline-to` give
   the baseline its own window (default: the run's). With Caddy basic auth set
   `TROUBLESHOOTING_BASIC_AUTH=user:password`.

Output (the reference fixture's candidate):

```
Quality gate: FAILED
run fx…-qg-candidate, baseline fx…-qg-baseline, 3 checks

FAIL    db-calls  fx…-qg-api / start_session (server)
        up to 21 database calls per execution, limit 3, 200 executions, 200 over the limit
        baseline: 2 (200 executions)
        target 2 per execution not met yet (advisory)
        permission_lookup × 20
        insert_session × 1
        evidence: execution with the most database calls: 21
        http://localhost:8095/#/executions?from=…&span=…&to=…&trace=…&view=timeline
```

## Semantics (quality-gate-v1)

**Executions.** An execution is a span of the run (`troubleshooting.run`
equal to `--run`) with the entry's service, operation name and (if given) span
kind, starting in the half-open window `[from, to)`, plus every span below it
in the same trace by recorded parentage. Spans are read up to 5 minutes after
the window, so an execution that ends after it is complete. Each span counts
once: one canonical row per (TraceId, SpanId) with the milestone 1 rule
(`winnerOrder`), so retries and replays never change a number.

**latency**: the percentile of the entry spans' own durations (never a sum
of children), exact nearest rank over all executions with a valid duration
(the smallest value that p% of the executions don't exceed). The limit is
`<=`: equal passes. Executions without a valid duration are left out and
reported. Fewer than `min_executions` is *insufficient*; the value so far is
still shown. Evidence: the slowest execution.

**db_calls**: per execution, the CLIENT spans with `db.system` or
`db.system.name` (the entry included). The value is the **maximum** over
executions, i.e. "at most N per execution" holds for every execution, not on
average; the result also lists how many executions exceed the limit. Details
name the operations of the worst execution with their counts. SQL text and
logs are never parsed. If no database call was recorded at all, the check
passes with a note to verify the instrumentation.

**forbidden_call**: an execution calls `to` when a span of service `to` has a
parent of another service, or a CLIENT span names `to` in `peer.service` (else
`server.address`), for a callee that isn't instrumented. The caller is that
parent's (or the CLIENT's) service; `from` restricts it. Spans inside the
forbidden service calling each other, and the entry itself, are not calls to
it. A pass reads "not observed in this run": a trace cannot prove a call can
never happen. Whether the call was synchronous is not judged.

**Statuses.** A check is `pass`, `fail` or `insufficient` (with a reason:
`no_executions`, `too_few_executions`, `outside_retention`, `too_many_spans`,
`query_limit`, `query_unavailable`, `query_error`). Targets are `met` or
`open` and never change a status. Spans whose parent was not recorded are
reported: calls below a missing span are not part of an execution.

**Gate.** Any failed blocking check → `failed`; otherwise any blocking check
with insufficient data → `unresolved`; otherwise `passed`. Advisory checks are
reported and never decide. The baseline is evaluated with the same rules and
shown next to each value; it never changes a status or the gate.

**Limits.** A window is at most 24 h and within raw span retention; at most
50 checks and 200 000 spans per run (more is `too_many_spans`, never a partial
pass). The usual query limits apply (`QUERY_TIMEOUT`, …).

## API

`POST /api/v1/quality/evaluate`

```json
{"rules": {…the rules as JSON…}, "run": "pr-42-1", "time_from": "<ns>", "time_to": "<ns>",
 "baseline": "main-118", "baseline_time_from": "<ns>", "baseline_time_to": "<ns>"}
```

Invalid rules or windows are 400 with a message naming the check. Otherwise
200 with `data.Evaluation`: `contract`, `method`, `run`, windows, `gate`,
`evaluated_at` and per check `id`, `type`, `entry`, `advisory`, `status`,
`reason`, `summary`, `unit`, `value`, `limit`, `target`, `target_status`,
`executions`, `violations`, `details`, `evidence` (trace id, span id, window
to open it) and `baseline` (status, value, executions, summary). A run that
can't be read is a 200 whose checks are `insufficient`, so the gate is
unresolved, never passed. Nothing is stored; the same request on the same data
gives the same answer.

## Code

- `log-analysis/internal/data/quality.go`: request validation, the bounded
  canonical span query (`qualityQuery`) and the pure evaluator
  (`evaluateRun`, `gateOf`); unit tests in `quality_test.go`.
- `log-analysis/cmd/rest_api/quality.go`: the endpoint.
- `quality-check/` (module `github.com/agerimex/troubleshooting/quality-check`):
  the CLI, standard library plus `gopkg.in/yaml.v3`.
- `fixtures/regression/qg.go`: the reference runs and their literal answers.

## Next (stage 2)

A Quality page (rules, latest results per run, baseline vs candidate, click
through to the evidence), a GitHub Action example that posts the summary on a
PR, this repository's own CI as the first real user, and an HTML report.
