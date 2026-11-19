---
description: Check a test run against latency limits, database calls per execution and forbidden calls kept in your repository, in CI or on the Quality page, with the execution behind every failure; name the requirements the checks enforce and keep their history.
---

# Quality gate

The quality gate answers “does this change keep its limits?” before it ships. Your tests run against the instrumented application; Troubleshooting then checks that **test run** against rules kept in your repository and links every failure to the execution behind it. A pass means the limits were not observed to be broken **in this run**; what the tests didn't execute, or what wasn't instrumented, can't be judged. It works from the command line, as a GitHub Action, and on the **Quality** page.

A change can pass every functional test and still add twenty database calls per request, or call a service it must not. The gate measures what the run actually did, from its spans.

| Word | Meaning |
| --- | --- |
| **Run** | One test run, identified by its `troubleshooting.run` tag |
| **Entry** | The operation a check starts from, such as `POST /api/v1/login` (the `entry:` of `quality.yaml`) |
| **Execution** | One run of the entry and everything it caused: usually one request, or one session in a desktop app |
| **Check** | One measurable rule, for example "at most 3 database calls per execution" |
| **Requirement** | Why checks exist: a title, an owner and a reason. It is met only when its checks pass |
| **Result** | Pass, fail, or not enough data |
| **Evidence** | The executions behind a result, saved with the report so they survive after the raw data expires |

## 1. Tag the test run

Give the spans of the run an id with the attribute `troubleshooting.run`, preferably on the resource, so the application needs no code change:

```
OTEL_RESOURCE_ATTRIBUTES=troubleshooting.run=pr-42-1
```

With `log-sender`: `sender.WithResourceAttributes(map[string]string{"troubleshooting.run": "pr-42-1"})`. Only the entry service needs it: spans of downstream services belong to the run through their recorded parent in the same trace. Use a new id for every CI run and attempt, <span v-pre>`ci-${{ github.run_id }}-${{ github.run_attempt }}`</span> (plus the job name when a matrix runs the same tests): a pull request number repeats on every push, and two runs with one id mix their executions.

## 2. Write the rules

Keep a `quality.yaml` next to the code:

```yaml
version: 1
run:                         # optional: what the test run declares
  executions: 200            # each checked entry runs this often: fewer means lost executions
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
  - id: db-calls
    type: db_calls
    max_per_execution: 3
    target_per_execution: 2  # optional, advisory
    min_per_execution: 2     # optional: fewer means its database spans are missing
  - id: no-directory
    type: forbidden_call
    to: directory-api
    from: recording-api      # optional: only calls from this service
```

| Check | Measures | Passes when |
| --- | --- | --- |
| `latency` | The percentile of the entry spans’ own durations (exact nearest rank) | The value is at most `max_ms` (equal passes), over at least `min_executions` (default 20), and every execution ended (an execution without a valid duration makes a pass “not enough data”) |
| `db_calls` | Client spans with `db.system` (or `db.system.name`) per execution | **Every** execution has at most `max_per_execution`, not on average; a run without any database call is “not enough data” unless the limit is 0; with `min_per_execution`, so is an execution with fewer calls (its database spans are missing) |
| `forbidden_call` | A span of service `to` called from another service, or a client span naming it in `peer.service` / `server.address` | No execution calls it: “not observed in this run” |
| `errors` | Executions whose entry span ended with status error | At most `max_failed` of them (`0`: none). On a latency check, `exclude_failed: true` leaves failed executions out of the percentile, since a failure that ends early makes a run look faster |

A check can have its own `entry`, and `advisory: true` shows it without letting it decide. Unknown and duplicate keys are errors, and so is a field of another check type (`max_ms` on a `db_calls` check), so a misspelled or misplaced limit is never silently ignored. SQL text and logs are never parsed.

## 3. Check the run

```
go install github.com/agerimex/troubleshooting/quality-check/cmd/troubleshooting-check@v0.1.0
troubleshooting-check --url http://localhost:8095 --rules quality.yaml \
    --run pr-42-1 --baseline main-118 --since 1h \
    --json result.json --html report.html --markdown summary.md
```

The exit code is the gate: **0** passed, **1** failed, **2** unresolved (not enough data), **3** error (invalid rules, unreachable API, …). Real output, from the reference fixture’s candidate run:

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

`--html` writes a self-contained report without scripts; `--markdown` a summary for a CI job page or a pull request comment.

## In GitHub Actions

```yaml
- name: Quality gate
  uses: agerimex/troubleshooting/quality-check@quality-check/v0.1.0   # a release; @main for the newest
  with:
    url: http://localhost:8095          # where your Troubleshooting runs
    run: ci-${{ github.run_id }}-${{ github.run_attempt }}
    rules: quality.yaml
    baseline: main-118                  # optional: the id of a run you tagged earlier
    rules-from: base                    # the rules of the pull request's base branch
    fail-on: unresolved                 # or "failed", or "never" (report only)
    record: auto                        # keep the result as history on push events
```

Without `since`, the window is the run's own, found from its spans, so a re-run of the job checks the same executions.

![The quality gate's comment on a pull request of this repository](/screenshots/quality-pr-comment.png)

*Troubleshooting gates its own pull requests: this is the comment on one of them, checking how many ClickHouse queries each API request runs and how fast the trace report answers. The last line says lost child spans could be detected in 350 of 355 executions: the other 5 started no child span, and a span that records 0 children looks the same as one that records nothing, so those 5 can't show that nothing was lost.*

The action builds the CLI, checks the run, writes the Markdown summary to the job page and as a pull request comment, uploads `result.json`, `report.html` and `summary.md` as an artifact, and then applies the gate. With `fail-on: unresolved` (the default) the step fails when the gate failed or had not enough data; an error always fails it. Outputs: `gate` and `exit-code`.

- **The limits come from the base branch** with `rules-from: base`, so a pull request can’t loosen its own gate. When it changes the rules file, the summary says that the base’s rules were used. Without `rules-from`, the checked-out file decides.
- **Every result names what was tested**: the commit that was checked out (for a pull request its merge commit, shown with the head it merges), the branch, the CI run, and the rules file with its SHA-256.
- **Pushes are recorded** (`record: auto`) as the history of the project's requirements; see [Requirements and their history](#requirements-and-their-history).
- **One comment per pull request**, updated on every run instead of adding new ones, and never posted for a commit that is no longer the head. The job needs `pull-requests: write`; for a pull request from a fork the token is read-only, so the comment is skipped and the result stays in the job summary.
- **The report keeps the evidence.** `report.html` contains a snapshot of each evidence execution (its spans as a small timeline), so it still shows what happened after the raw spans have expired.

### No Troubleshooting server? Start one in the job

`quality-check/stack` starts a private Troubleshooting inside the CI job (ClickHouse, receiver and API; Linux runners). Your tests export their spans to it with the standard OpenTelemetry variables:

```yaml
- id: troubleshooting
  uses: agerimex/troubleshooting/quality-check/stack@quality-check/v0.1.0
- run: go test ./...
  env:
    OTEL_EXPORTER_OTLP_ENDPOINT: ${{ steps.troubleshooting.outputs.otlp-endpoint }}
    OTEL_EXPORTER_OTLP_INSECURE: "true"
    OTEL_RESOURCE_ATTRIBUTES: troubleshooting.run=ci-${{ github.run_id }}-${{ github.run_attempt }}
- uses: agerimex/troubleshooting/quality-check@quality-check/v0.1.0
  with:
    url: ${{ steps.troubleshooting.outputs.url }}
    ui: ${{ steps.troubleshooting.outputs.ui }}   # none: the job's server is gone afterwards; report.html keeps the evidence
    run: ci-${{ github.run_id }}-${{ github.run_attempt }}
    rules-from: base
```

[`examples/quality-gate`](https://github.com/agerimex/troubleshooting/tree/main/examples/quality-gate) is a complete example: a small orders service with its `quality.yaml`. Troubleshooting’s CI runs it on every push, once as written (2 database calls per request: passed) and once with a planted change that reads prices one by one and calls `legacy-pricing` (9 database calls and a forbidden call: failed). [Catch an N+1 query in a pull request](/articles/catch-n-plus-one-in-a-pull-request) follows that change through the gate, with the real output.

::: warning Trust
The workflow file is part of the pull request too. Protect `main` and review changes under `.github/`. Don’t run untrusted fork code with secrets that reach a private Troubleshooting.
:::

Troubleshooting’s own CI is the first user: its integration job tags the API’s spans with the CI run id and checks the ClickHouse queries per request (`trace-report`, `view-spans`, `stats/calls`, `compare`, `quality/evaluate`) against `.github/quality.yaml`. A change that adds a query per row fails the build, with the request behind it.

## The Quality page

The **Quality** page checks a run interactively: paste the rules, name the run and optionally a baseline, pick a window (last 15 minutes to 24 hours) and click **Check run**. The rules are sent as typed and parsed as strictly as the CLI does.

![Quality page](/landing/quality.webp)
*The fixture’s candidate run against its baseline: all three checks fail. REQ-START names its own failed check and, separately, its failed guardrail `db-calls`.*

Every check shows its value, the limit with the advisory target, the baseline and the number of executions, then the details and the evidence. **Open timeline** and **Open sequence** open the evidence execution in Timeline & Sequence; **Back to Quality** returns to the result.

![Evidence opened from the quality check](/landing/quality-evidence.webp)
*The execution with the most database calls: twenty `permission_lookup` calls where the baseline makes one.*

**Open a result file** shows a CI `result.json` the same way, so a failed build can be investigated in the UI. Its evidence opens live while the raw spans are retained; when they are gone (expired, or recorded by a Troubleshooting started inside the job) **Open timeline** and **Open sequence** show the saved snapshot from the file instead, marked as such: the spans with their names, services, kinds, times and status, without logs, attributes or events. **Export JSON** and **Export HTML** save the result as shown. The rules and run ids are remembered in this browser only, and a check made on the page is not recorded.

## Requirements and their history

Checks say *what* is measured; **requirements** say *why*, and who cares. They live in the same `quality.yaml`, so a change to a requirement is reviewed like code:

```yaml
project: recording      # optional; default: the entry's service
requirements:
  - id: REQ-START
    title: A recording session starts within 800 ms
    owner: recording-team
    why: Participants leave a session that takes more than a second to start.
    checks: [startup-p95]
    guardrails: [db-calls]    # supports it; decides the gate, not its status
  - id: REQ-DIRECTORY
    title: Starting a session must not call the directory
    owner: recording-team
    checks: [no-directory]
  - id: REQ-OUTAGE
    title: Recording continues during an analytics outage
    owner: platform-team      # no checks yet: listed, never met
```

A requirement's status comes from its own checks, in the same run:

| Status | When |
| --- | --- |
| Violated | One of its checks failed (a missing measurement of another is still named) |
| Not enough data | None failed, but one had not enough data |
| Met | Every one of its checks passed |
| Not evaluated | It has no checks yet: written down, never met |

**Guardrails** support a requirement without assessing it. The query budget protects REQ-START, but a start with four database calls in 400 ms isn't slow, so `db-calls` is its guardrail: it is shown with the requirement and still fails the gate, but it never marks “starts within 800 ms” as violated. Word a requirement as what its checks can show: a forbidden call proves a call was made, not that the caller waited for it.

The gate itself is still decided by the checks. The CLI, the job summary, the pull request comment and the HTML report list the requirements above the checks.

**Record** an evaluation to give each requirement a history: `troubleshooting-check --record`, or the Action's `record` input, which is on for **push** events by default (so the history follows `main` and your tags, not every pull request push). Troubleshooting then keeps the result exactly as it was, the rules text it used with its sha256, the tested commit and CI run, and a snapshot of every evidence execution, so a failure from months ago still shows what happened after its spans expired. A retried step is answered with its first record, exactly as recorded (the CLI says “Already recorded”), so one evaluation id never has two verdicts, even when two retries arrive at once; a new CI attempt, or `--reevaluate`, is a new record. Recording needs a Troubleshooting that outlives the job, not `quality-check/stack`.

On the **Quality** page, **Requirements** shows a project's catalogue: every requirement of the latest recorded evaluation with its owner and reason, its checks, since when its status holds (“Violated since commit 9be31d07a4c2 (2 evaluations); met before.”) and its history, oldest on the left. A streak never spans different criteria: when a requirement's checks change, its status counts from there (“Met since … when its checks changed; violated under the earlier checks.”). A cell opens that evaluation; a cell is outlined where the requirement's checks changed. **History** lists every recorded evaluation, and **Copy link** on an opened one gives a link that reopens it.

![Requirements on the Quality page](/landing/quality-requirements.webp)
*Five recorded evaluations of `recording-app`: the baseline, the regression (both requirements violated), a run with too few executions, the fix, and the fix again after REQ-START was tightened to 700 ms. REQ-START's streak restarts there (“when its checks changed”); REQ-DIRECTORY's check didn't change, so its streak spans both passes. REQ-OUTAGE has no check yet.*

## How to read a result

| Status | Meaning |
| --- | --- |
| Pass | The run kept the limit |
| Fail | The run broke the limit; the evidence names an execution that did |
| Not enough data | No executions, fewer than needed or than `run.executions` declares, incomplete executions (a span below the entry names a parent that was not recorded, a span started more child spans than were recorded, or the execution runs more than 5 minutes past the window), spans outside retention, or a query limit. Never a pass |

- **Gate**: any failed blocking check → *failed*; otherwise any check without enough data → *unresolved*; otherwise *passed*.
- **Targets** are *met* or *open* and never change a status. **Advisory** checks are shown and never decide.
- **The baseline** is evaluated with the same rules and shown next to each value; it never changes a status. A pass is not “every number improved”.
- A pass describes this run only. What the tests didn’t execute, or what wasn’t instrumented, can’t be judged.
- **A pass holds for the recorded spans.** It assumes every call is instrumented, not sampled out and exported before the check. Lost executions (`run.executions`), spans naming a parent that was never recorded and executions that run past the window are caught. A span that started more children than were recorded is caught only when the sender records child counts: `log-sender` does, plain OpenTelemetry (OTLP) doesn’t carry them, so there a child span lost without any trace of it can’t be seen. Every result lists these assumptions and says in how many of its executions lost child spans can be seen; a run that mixes senders is covered only partly. Set `run: {child_counts: required}` to make such a pass unresolved.
- Each span counts once, so retries and replays of the same spans never change a number.

Limits: a window is at most 24 hours and within raw span retention; at most 50 checks and 200 000 spans per run (more is “not enough data”, never a partial pass).

The full contract (`quality-gate-v1`) is in [`docs/quality-gate.md`](https://github.com/agerimex/troubleshooting/blob/main/docs/quality-gate.md).
