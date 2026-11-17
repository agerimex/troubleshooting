# Quality gate (milestone 3)

Check one **test run** against rules kept in your repository, and get the
execution behind every failure. Contract `quality-gate-v1`. Acceptance scope:
`features/milestone_3/quality_gate.feature`; scenario-to-test map:
`docs/milestone3-completion.md`.

It is a small, working slice of the direction proposed in
`spec-16.11/Troubleshooting_Implementation_Spec` (not committed). From that
spec it keeps: separate pass / fail / not-enough-data states, limits apart
from advisory targets, a baseline that is shown but never decides, call counts
per execution (never from shape edges), and "not observed" instead of
"impossible". It leaves out for now: imported, digest-pinned requirement
revisions, a run registry and lifecycle, stored evaluations,
load and outage experiments, and an enforced CI trust model (see "Trust" below).

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
   `--html report.html` writes a self-contained page (no scripts, recorded
   text escaped) and `--markdown summary.md` a summary for a CI job or a pull
   request comment: a table of every check, details only for the checks that
   failed or lack data.
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

## In GitHub Actions

`quality-check/action.yml` is a composite action: it builds the CLI, checks
the run, appends the Markdown summary to the job summary, uploads
`result.json`, `report.html` and `summary.md` as an artifact and then applies
the gate.

```yaml
- name: Quality gate
  uses: agerimex/troubleshooting/quality-check@main   # pin a commit for real use
  with:
    url: http://localhost:8095          # where your Troubleshooting runs
    run: pr-${{ github.event.number }}-${{ github.run_attempt }}
    rules: quality.yaml
    baseline: main-latest               # optional
    since: 1h
    rules-from: base                    # read the rules from the PR's base branch
    fail-on: unresolved                 # or "failed", or "never" (report only)
    comment: auto                       # PR comment on pull_request events; "true" / "false"
```

Outputs: `gate` (passed / failed / unresolved / error) and `exit-code`.
Inputs reach the scripts only as environment variables. `fail-on:
unresolved` (the default) fails the step when the gate failed or had not
enough data; an error always fails it.

**Which rules decide (`rules-from`).** Empty (default): the checked-out
commit's file, so a pull request can change its own limits (visible in the
diff, not prevented). `base`: the file of the pull request's base branch,
fetched from origin: the candidate can't loosen its gate, and when it changes
the file the summary says that the base's rules were used. If the base has no
such file yet, the gate is being introduced and this commit's file is used,
with a notice. On a push (no base) the checked-out file is used. Any other
value is a branch or tag fetched from origin.

**What was tested.** The CLI adds `source` to the result: `commit` (the
action passes a pull request's head, not its merge commit), `ref`, `ci_run`
(link to the run attempt), `rules` (the file as given), `rules_ref` (e.g.
`main@89ab01234567:quality.yaml` with `rules-from`) and `rules_sha256` of the
file's bytes. Text, Markdown and HTML reports show it in one line, and the
Quality page shows it for an opened result file. CLI flags `--commit`,
`--ref`, `--ci-run`, `--rules-ref`.

**Pull request comment.** `troubleshooting-check comment` posts the Markdown
summary as one comment and updates that same comment on later runs (found by
a hidden marker per `artifact` name; comments by people are never edited), so
retries never add a second, contradicting comment. It posts nothing when the
pull request's head is no longer the tested commit. The job needs
`pull-requests: write`; without it (a pull request from a fork gets a
read-only token) the comment is skipped with a notice and the step does not
fail: the result is still in the job summary and the artifact. Comments over
GitHub's size limit are cut at a line end.

Trust: with `rules-from: base` the limits come from the base branch. The
workflow file itself is still part of the pull request; branch protection and
review of `.github/` changes are what keep that honest. Don't run untrusted
fork code with secrets to reach a private Troubleshooting.

### An application's CI: Troubleshooting in the job

An application doesn't need a Troubleshooting server its CI can reach:
`quality-check/stack` starts a private one inside the job (ClickHouse in
Docker, the receiver and the API built from this repository; Linux runners).
The tests export spans over OTLP to it, then the gate checks them:

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write                # for the comment
    steps:
      - uses: actions/checkout@v4
      - id: troubleshooting
        uses: agerimex/troubleshooting/quality-check/stack@main
      - run: go test ./...
        env:
          OTEL_EXPORTER_OTLP_ENDPOINT: ${{ steps.troubleshooting.outputs.otlp-endpoint }}
          OTEL_EXPORTER_OTLP_INSECURE: "true"
          OTEL_RESOURCE_ATTRIBUTES: troubleshooting.run=pr-${{ github.run_id }}-${{ github.run_attempt }}
      - uses: agerimex/troubleshooting/quality-check@main
        with:
          url: ${{ steps.troubleshooting.outputs.url }}
          run: pr-${{ github.run_id }}-${{ github.run_attempt }}
          rules-from: base
```

Outputs of the stack: `url`, `otlp-endpoint`, `sender-address` (for
log-sender) and `logs` (the services' logs). Inputs: the four ports and the
ClickHouse image. Evidence links then point to the job's own API, which is
gone after the job: the reports keep a snapshot of each evidence execution
(below).

`examples/quality-gate/` is such an application (an orders service with
`quality.yaml`), and this repository's CI job `quality-example` runs exactly
the steps above. It also runs the tests again with `EXAMPLE_REGRESSION=1`, a
planted change that passes every functional test (prices read one by one, 9
database calls instead of 2, and a call to `legacy-pricing`), and requires
that gate to fail.

**This repository is the first user.** The CI job `integration` runs the test
stack with `TROUBLESHOOTING_RUN=ci-<run id>-<attempt>`, so the API tags its
own spans; after the regression fixture the action checks those requests
against `.github/quality.yaml`: at most 16 ClickHouse queries per
`trace-report`, 8 per `view-spans`, 6 per `stats/calls`, 20 per `compare`, 3
per `quality/evaluate` (measured 12, 5, 3, 14, 2) and an advisory p95. A
change that adds a query per row fails the build with the request behind it.

## The Quality page

The UI's **Quality** page (between Releases and Settings) checks a run
interactively: paste the rules, name the run and optionally a baseline, pick a
window (last 15 minutes to 24 hours), **Check run**. The result shows the
gate, every check with value, limit, target, baseline and executions, the
details and the evidence; **Open timeline** / **Open sequence** open the
execution in Timeline & Sequence, and "Back to Quality" returns. **Open a
result file** shows a CI result (`result.json` of the CLI or the action)
the same way, with working evidence buttons while its raw spans are retained.
**Export JSON / HTML** save the result as shown. The rules and run ids are
remembered in the browser only; evaluations are not stored.

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
`no_executions`, `too_few_executions`, `incomplete_executions`, `outside_retention`, `too_many_spans`,
`query_limit`, `query_unavailable`, `query_error`). Targets are `met` or
`open` and never change a status.

**Incomplete executions.** An execution is incomplete when a span of its
trace names a parent that was not recorded (lost, sampled out, not
instrumented) and starts within the entry's duration (from its start on if
the duration is invalid), so it may belong below the entry. The entry's own
missing parent (e.g. an uninstrumented gateway above it) doesn't count: it
hides nothing below the entry. `db_calls` and `forbidden_call` can't pass
with incomplete executions: they are `insufficient` with reason
`incomplete_executions`, the first incomplete execution as evidence (its
orphaned span selected) and the value counted so far. A failure stands, since
the calls it counted were recorded. Latency is the entry's own duration and
is unaffected.

**Evidence snapshots.** With `--json` or `--html` the CLI reads each evidence
trace once (`trace-report`, at most 200 spans) and keeps it as `snapshot` in
the result; `report.html` shows it as a static timeline (start offset,
duration bar, nesting, the evidence span highlighted). Raw spans expire, so a
link in an old pull request stops working; the snapshot still shows what the
execution did. A failed read is noted and never changes a status.
`--no-snapshot` skips it.

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

Instead of `rules`, `rules_yaml` carries the rules file as text (the UI sends
it so); it is parsed as strictly as the CLI does it: unknown or duplicate keys,
more than one document or more than 64 KiB are 400.

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
  (`evaluateRun`, `requireComplete`, `gateOf`); unit tests in `quality_test.go`.
- `log-analysis/cmd/rest_api/quality.go`: the endpoint.
- `quality-check/` (module `github.com/agerimex/troubleshooting/quality-check`):
  the CLI, standard library plus `gopkg.in/yaml.v3`: `main.go` (check,
  `source`), `reports.go`, `snapshot.go` (evidence snapshots), `comment.go`
  (`troubleshooting-check comment`).
- `quality-check/action.yml` (the gate) and `quality-check/stack/action.yml`
  (Troubleshooting inside a CI job).
- `examples/quality-gate/`: the example application and its `quality.yaml`
  (CI job `quality-example`).
- `fixtures/regression/qg.go`: the reference runs and their literal answers.
- `log-ui/src/components/QualityView.vue` with the pure parts in
  `src/services/quality.ts` (unit tests `quality.spec.ts`, browser tests
  `e2e/quality.e2e.ts`).
- `.github/quality.yaml`: this repository's rules; `TROUBLESHOOTING_RUN` tags
  the API's own spans (`log-analysis/cmd/rest_api/main.go`).

Stages 1 (API, CLI, fixture), 2 (reports, Quality page, action, own CI) and 3
(trusted rules, provenance, PR comment, Troubleshooting in the job, evidence
snapshots, incomplete executions) are recorded in
`docs/milestone3-completion.md`.

## Not done (only if asked)

Stored evaluations and a history per run, a "latest run on main" baseline
(needs a run registry), imported requirement revisions with digests, load and
outage experiments, sync-call semantics and allowlists, published container
images for the in-job stack (it builds the receiver and API, about a minute),
showing snapshots on the Quality page when raw spans have expired.
