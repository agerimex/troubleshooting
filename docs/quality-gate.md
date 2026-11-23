# Quality gate (milestone 3)

Check one **test run** against rules kept in your repository, and get the
execution behind every failure. A pass means the limits were **not observed
to be broken in this run**: it says nothing about what the tests didn't
execute or what wasn't instrumented. Contract `quality-gate-v1`. Acceptance scope:
`features/milestone_3/quality_gate.feature`; scenario-to-test map:
`docs/milestone3-completion.md`.

It is a small, working slice of the direction proposed in
`spec-16.11/Troubleshooting_Implementation_Spec` (not committed). From that
spec it keeps: separate pass / fail / not-enough-data states, limits apart
from advisory targets, a baseline that is shown but never decides, call counts
per execution (never from shape edges), and "not observed" instead of
"impossible", requirements with an owner and a reason assessed only by their
own checks (a requirement without a check is never met), and stored
evaluations as a requirement's history (stage 5). It leaves out for now:
imported, digest-pinned requirement revisions (the rules file in Git is the
revision; each record keeps its text and sha256), a run registry and
lifecycle, load and outage experiments, and an enforced CI trust model (see
"Trust" below).

## How it works

1. Tag the spans of a test run with **`troubleshooting.run`**, preferably as
   a resource attribute so the application needs no code change:

   ```
   OTEL_RESOURCE_ATTRIBUTES=troubleshooting.run=pr-42-1
   ```

   With log-sender: `sender.WithResourceAttributes(map[string]string{"troubleshooting.run": "pr-42-1"})`.
   A span attribute of the same name works too. Only the entry service needs
   it: spans of downstream services belong to the run through their recorded
   parent in the same trace. In CI use a new id for every run and attempt,
   `ci-${{ github.run_id }}-${{ github.run_attempt }}` (plus the job name when
   a matrix runs the same tests): a pull request number repeats on every push,
   and two runs with one id mix their executions.

2. Write the rules (`quality.yaml`; example in
   `quality-check/quality.example.yaml`):

   ```yaml
   version: 1
   run:                         # optional: what the test run declares
     executions: 200            # each checked entry runs this often: fewer = lost
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
       min_executions: 20       # default run.executions (at least 20 for latency), else 20 for latency, 1 otherwise
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

   Unknown keys and duplicate keys are errors, and so is a field of another
   check type (e.g. `max_ms` on a `db_calls` check): a misspelled or misplaced
   limit is never silently ignored. A file holds one YAML document of at most
   64 KiB; the CLI and the API apply the same limits.

3. Run the tests against your instrumented application, then:

   ```
   go install github.com/agerimex/troubleshooting/quality-check/cmd/troubleshooting-check@v0.1.0
   troubleshooting-check --url http://localhost:8095 --rules quality.yaml \
       --run pr-42-1 --baseline main-118 --json result.json
   ```

   Exit code: **0** passed, **1** failed, **2** unresolved (not enough
   data), **3** error (no usable result: invalid rules, unreachable API, …).
   `--html report.html` writes a self-contained page (no scripts, recorded
   text escaped) and `--markdown summary.md` a summary for a CI job or a pull
   request comment: a table of every check, details only for the checks that
   failed or lack data.
   `--url` is the UI origin (which serves `/api`) or the API itself; `--ui`
   sets the origin of evidence links if it differs (`none`: no links, for a
   Troubleshooting that is gone after the job; the evidence then names its
   trace and its snapshot in report.html). Without a window the API takes
   **the run's own**: from its first to its last entry span within the last
   24 h (and the baseline's own), so a re-run of the job evaluates, and
   records, the same executions; the result says `window_from_run`.
   `--since` (executions of the last duration) or `--from`/`--to` (RFC 3339)
   set a window instead; `--baseline-since`/`--baseline-from`/`--baseline-to`
   give the baseline its own (default: the run's window). Against an older
   Troubleshooting that needs a window, the CLI says so and checks the last
   `--since` (1 h). `--record` keeps the
   evaluation in Troubleshooting as the project's history and prints its link
   (see "Requirements and history"); an evaluation recorded before is shown
   as that record ("Already recorded"), and `--reevaluate` records a new one.
   The text report ends with the result's assumptions. With Caddy basic auth set
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
  uses: agerimex/troubleshooting/quality-check@quality-check/v0.1.0   # a release; @main for the newest
  with:
    url: http://localhost:8095          # where your Troubleshooting runs
    run: ci-${{ github.run_id }}-${{ github.run_attempt }}
    rules: quality.yaml
    baseline: main-118                  # optional: the id of a run you tagged earlier
    since: 1h
    rules-from: base                    # read the rules from the PR's base branch
    fail-on: unresolved                 # or "failed", or "never" (report only)
    comment: auto                       # PR comment on pull_request events; "true" / "false"
    record: auto                        # keep the evaluation as history on push events; "true" / "false"
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
tested commit: the action records what is checked out, `git rev-parse HEAD`,
which for a pull request is its merge commit), `pr_head` (the pull
request's head, when it differs; the comment is skipped once the head moves
on), `ref`, `ci_run`
(link to the run attempt), `rules` (the file as given), `rules_ref` (e.g.
`main@89ab01234567:quality.yaml` with `rules-from`) and `rules_sha256` of the
file's bytes. Text, Markdown and HTML reports show it in one line, and the
Quality page shows it for an opened result file. CLI flags `--commit`,
`--pr-head`, `--ref`, `--ci-run`, `--rules-ref`.

**Pull request comment.** `troubleshooting-check comment` posts the Markdown
summary as one comment and updates that same comment on later runs (found by
a hidden marker per `artifact` name and the token's own author: the
github-actions bot for `GITHUB_TOKEN`, the token's user for a personal access
token; comments by anyone else are never edited), so
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
The built receiver and API are cached under a hash of their sources (Actions
cache, saved right after the build), so only the first job of each action
version builds them.
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
        uses: agerimex/troubleshooting/quality-check/stack@quality-check/v0.1.0
      - run: go test ./...
        env:
          OTEL_EXPORTER_OTLP_ENDPOINT: ${{ steps.troubleshooting.outputs.otlp-endpoint }}
          OTEL_EXPORTER_OTLP_INSECURE: "true"
          OTEL_RESOURCE_ATTRIBUTES: troubleshooting.run=ci-${{ github.run_id }}-${{ github.run_attempt }}
      - uses: agerimex/troubleshooting/quality-check@quality-check/v0.1.0
        with:
          url: ${{ steps.troubleshooting.outputs.url }}
          ui: ${{ steps.troubleshooting.outputs.ui }}   # none: no links to a server that is gone after the job
          run: ci-${{ github.run_id }}-${{ github.run_attempt }}
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
remembered in the browser only; a check made on the page is not recorded.

The page has three tabs: **Requirements**, **History** and **Check a run**
(the above). It opens on Requirements once something is recorded, and keeps
the tab chosen last.

- **Requirements** is a project's catalogue (pick the project, optionally a
  branch or tag): the latest recorded evaluation (gate, commit, time, "Open
  evaluation"), then every requirement of its rules with status, owner, reason,
  its checks and guardrails with their status, since when the status holds
  under the current criteria ("Violated since commit 0123456789ab (2
  evaluations); met before.") and its history as a row
  of cells, oldest on the left, one per recorded evaluation (at most 30):
  green met, red violated, amber not enough data, grey not evaluated, dashed
  when that evaluation's rules didn't have it, outlined when the requirement's
  criteria changed from the evaluation before. A cell opens that evaluation. Checks no
  requirement lists are named below.
- **History** lists the recorded evaluations, newest first (time, gate,
  commit and branch, run, requirement counts), 50 at a time.
- An opened evaluation shows exactly what was recorded, with its rules
  ("Rules of this evaluation") and **Copy link**: `#/quality?evaluation=<id>`
  reopens it. Its evidence opens live while the raw spans are retained and as
  the recorded snapshot after that.

## Requirements and history (stage 5)

**Requirements** say why the checks exist. They are a list in the same rules
file, next to the checks, so they are reviewed like code:

```yaml
project: recording          # optional; names the history (default: the entry's service)
requirements:
  - id: REQ-START
    title: A recording session starts within 800 ms
    owner: recording-team
    why: Participants leave a session that takes more than a second to start.
    checks: [startup-p95]
    guardrails: [db-calls]    # supports it; decides the gate, not its status
  - id: REQ-DIRECTORY
    title: Starting a session must not call the directory
    checks: [no-directory]
  - id: REQ-OUTAGE
    title: Recording continues during an analytics outage
    owner: recording-team
```

Ids follow the check id rules and are unique; a title is required (at most 200
characters), owner (100) and why (1 000) are optional; every listed check must
exist in the file, once. At most 50 requirements. A check may serve several
requirements, or none.

**Checks and guardrails.** `checks` are the checks that *assess* the
requirement; `guardrails` are checks that support it without assessing it. A
query budget protects a latency requirement, but a start with four database
calls in 400 ms doesn't show that starting is slow, so the budget is REQ-START's
guardrail. A guardrail is listed with the requirement (`guardrails`,
`guardrail_summary`: "guardrail failed: db-calls (guardrails decide the gate,
not this requirement)") and decides the gate like every check, but never the
requirement's status. A guardrail must exist in the rules, once, and can't
also be one of the same requirement's checks; a check listed only as a
guardrail is not "without a requirement".

Word a requirement as what its checks show. `forbidden_call` sees that a call
was made, not whether the caller waited for it, so "Starting a session must not
call the directory" is what it can assess; "never waits for the directory" is
not.

A requirement's **status** comes from its own checks in the same evaluation:
`violated` if one of them failed (missing data of another is still named),
else `insufficient` if one had not enough data, else `met`; a requirement
without checks is `not_evaluated` ("no check enforces it yet"): written down,
never met. Advisory checks count for their requirement too (advisory only
means a check doesn't decide the gate). Guardrails never count for it. The gate itself is unchanged: it is
decided by the checks. Every result has `project` and `requirements` (`[]`
without requirements), and the CLI's text, Markdown and HTML reports list them
above the checks.

**Recording.** With `record: true` (CLI `--record`, action input `record`)
the API stores the evaluation as it answered it, under the rules' project, in
`quality_evaluations` (created by the receiver; no TTL), together with:

- the exact rules text (so the request must send `rules_yaml`, as the CLI and
  the UI do) and its sha256, computed by the API;
- `source` as sent (commit, PR head, ref, CI run, where the rules were read
  from);
- a **snapshot** of each evidence trace (up to 200 spans), taken from the spans
  the evaluation already read, in the format of the CLI's `result.json`; the
  CLI then doesn't read those again.

The answer then has `id` (24 hex digits) and `recorded_at`. A recorded
evaluation is never changed. The id is derived from the inputs (project, run,
windows, rules, commit, ref, CI run), so a retried request records once, and
it is answered with **that record, exactly as recorded** (`recorded_before:
true`), never with the result just computed: one id has one verdict, even if
late spans arrived meanwhile. A new CI attempt is a new CI run and a new
record; `reevaluate: true` (CLI `--reevaluate`) records a deliberate
re-evaluation of the same inputs under an id of its own (`reevaluated: true`).
Concurrent requests with one id are one record too: an API instance records
one id at a time, and the table drops a second insert of an id (the id is the
insert's deduplication token; the receiver sets
`non_replicated_deduplication_window` on `quality_evaluations`), so two
instances recording at once keep one insert. Every request reads the record
back after its insert and answers with what is stored, so all of them answer
with one verdict; reads take the earliest row of an id, for rows recorded
twice before this existed.

**Streaks and criteria.** Each requirement of a recorded evaluation carries
`criteria_sha256`: the sha256 of what decides its status, its checks'
definitions without their ids, targets and advisory flag, and
`run.executions`. A streak ("Met since …") is the newest run of evaluations
with the same status *and* the same criteria: two passes against different
limits are two streaks, and the catalogue says so ("Met since commit … (1
evaluation), when its checks changed; met under the earlier checks.",
`streak.criteria_changed`). Wording, owners, targets, guardrails and other
requirements' checks don't change the criteria. For evaluations recorded
before criteria existed, any change of the rules file's sha256 ends a streak.
Not evaluated and "not in the rules" have no criteria; only their status
counts. A failed write leaves the result valid and says so in
`record_error` (the CLI prints a warning; the exit code still follows the
gate). When the insert went through but reading the stored record back
failed, the answer keeps the `id`, has `record_unconfirmed: true` and is the
local verdict: another instance may have recorded a different one under that
id. The CLI then prints "Not confirmed: the evaluation was sent to the
history as …" with the link, never "Recorded as evaluation", and the Quality
page says the same for such a result file; open the id for the recorded
verdict.

The action records on **push** events by default (`record: auto`), so a
project's history follows its branches and tags, not every pull request push;
`true` or `false` choose explicitly. Recording only helps with a
Troubleshooting that outlives the job, not with `quality-check/stack`.

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
`db.system.name` (the entry included), except spans that report on a call
rather than make one: go.nhat.io/otelsql's `sql:rows_close`, `sql:rows_next`,
`sql:rows_affected`, `sql:last_insert_id` and `sql:reset_session` (they
carry `db.system` too and would count a query twice; the details name them
as "not counted"). The value is the **maximum** over
executions, i.e. "at most N per execution" holds for every execution, not on
average; the result also lists how many executions exceed the limit. Details
list the calls of the worst execution with their counts, by `db.statement`
(`db.query.text`, at most 160 characters, as recorded) or by span name when a
call recorded no statement, so a drop from five calls to four shows which
one is missing. SQL text and
logs are never parsed. A run without any database call is `no_database_calls`
(below). `min_per_execution` (optional, at most the limit) is an
instrumentation guard: an execution with fewer recorded calls is missing its
database spans, so a pass is `missing_database_calls` (evidence: the
execution with the fewest); a failure stands.

**forbidden_call**: an execution calls `to` when a span of service `to` has a
parent of another service, or a CLIENT span names `to` in `peer.service` (else
`server.address`), for a callee that isn't instrumented. The caller is that
parent's (or the CLIENT's) service; `from` restricts it. Spans inside the
forbidden service calling each other, and the entry itself, are not calls to
it. A pass reads "not observed in this run": a trace cannot prove a call can
never happen. Whether the call was synchronous is not judged.

**errors**: the executions whose entry span ended with status error, at most
`max_failed` (`0`: none may fail). The status is what the application
recorded: an execution that failed without setting it is not seen as failed.
Evidence: the first failed execution. Lost executions (`run.executions`) are
insufficient, as for every check.

**exclude_failed** (latency, optional): failed executions are left out of the
percentile, because a failure that ends early makes a run look faster. All
executions still count against `run.executions`; the percentile then needs
at least 20 successful ones, and the details say how many failed ones were
left out.

**Statuses.** A check is `pass`, `fail` or `insufficient` (with a reason:
`no_executions`, `too_few_executions`, `missing_executions`, `incomplete_executions`,
`invalid_durations`, `no_database_calls`, `missing_database_calls`, `outside_retention`, `too_many_spans`,
`query_limit`, `query_unavailable`, `query_error`). Targets are `met` or
`open` and never change a status.

**What a pass needs.** Traces can't show what was never recorded, so a pass
only stands on data that can carry it:

- `run.executions` declares how many executions of each checked entry the
  tests make. It is every check's default minimum (latency: at least 20), and
  fewer is `missing_executions`: executions were lost (not exported, not
  tagged, outside the window), not a smaller run. A check's own
  `min_executions` wins.
- A `db_calls` check with no database call in any execution is
  `no_database_calls`: missing instrumentation is far more likely than an
  operation that reads nothing. If it really makes none, the limit says so:
  `max_per_execution: 0`.
- Executions without a valid duration (the entry never ended, or ended before
  it started) are often the slowest ones, so a latency pass with any of them
  is `invalid_durations`; a failure measured on the others stands.
- A child span lost without any trace of it (no recorded span names it as
  parent) can't be seen in plain OpenTelemetry data. A sender that records how
  many children a span started (`log-sender` does; OTLP doesn't carry it)
  makes it visible: such an execution is incomplete (below). Otherwise
  `min_per_execution` catches executions without their database calls, but
  not a partial loss.
- Whether an execution records its child counts is decided per execution: its
  entry span reports a count, and so does every span with recorded children
  of its own service (a recorded 0 and no count at all look the same, so an
  execution whose spans start no children can't establish it). A run that
  mixes senders is covered only partly. `run.child_counts: required` makes a
  `db_calls` or `forbidden_call` pass with any execution without counts
  `insufficient` with reason `unverified_child_counts` (evidence: the first
  such execution); a failure stands. It is part of the requirements'
  criteria.

**Assumptions.** Every result lists in `assumptions` what its verdicts rest
on: they hold for the recorded spans, with every call assumed to be
instrumented, not sampled out and exported before the check; which losses are
caught as not enough data; and in how many of the checked executions lost
child spans can be seen: all ("Lost child spans are detected too"), some
("Lost child spans are detected in 1 of 200 executions only: …") or none
("Child spans lost without a trace can't be detected"). One span with a count
never speaks for the whole run. `child_counts` has the numbers
(`executions`, `counted`, `status` all/partial/none). The CLI's reports,
the PR comment and the Quality page show the assumptions.

**Incomplete executions.** An execution is incomplete when a span of its
trace names a parent that was not recorded (lost, sampled out, not
instrumented) and starts within the entry's duration (from its start on if
the duration is invalid), so it may belong below the entry. The entry's own
missing parent (e.g. an uninstrumented gateway above it) doesn't count: it
hides nothing below the entry, and neither do the entry's siblings under that
same missing parent (the gateway's other calls). An execution is also
incomplete when it runs past the spans read (the window's end plus 5
minutes): calls at its end were not read; end the window later. And an
execution is incomplete when one of its spans recorded that it started more
child spans in its own process than were recorded below it with the same
service (the sender's child count, e.g. `log-sender`'s; a remote child in
another service isn't counted by it); evidence: that span.
`db_calls` and `forbidden_call` can't pass with incomplete executions: they are `insufficient` with reason
`incomplete_executions`, the first incomplete execution as evidence (its
orphaned span selected, or the entry for one that runs past the spans read)
and the value counted so far. A failure stands, since
the calls it counted were recorded. Latency is the entry's own duration and
is unaffected.

**Evidence snapshots.** With `--json` or `--html` the CLI reads each evidence
trace once (`trace-report`, up to 1 000 spans) and keeps at most 200 of them as
`snapshot` in the result (a recorded evaluation gets the API's, from the spans
it read). The counterexample is kept first: the evidence span with its
ancestors, for a `db_calls` check the execution's database calls, then the
spans below the evidence span, then the rest in start order
(`SnapshotSelection`), so a large trace can't push the offending span out; `report.html` shows it as a static timeline (start offset,
duration bar, nesting, the evidence span highlighted). Raw spans expire, so a
link in an old pull request stops working; the snapshot still shows what the
execution did. A failed read (including a query limit, which returns no
spans) is noted and never changes a status; up to 4 traces are read at the
same time. `--no-snapshot` skips it. With `--ui none` (a Troubleshooting
started inside the job) the evidence has no link, and the Markdown points to
the HTML report for the snapshot only when one is written. The Quality page
opens a result file's snapshot in Timeline & Sequence when the live spans of
its trace are gone (marked as a saved snapshot; logs, attributes and events
are not kept), and the live report whenever the spans still exist.

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

`time_from`/`time_to` may both be left out: the window is then the run's
own (its first to its last entry span within the last 24 h and raw
retention; none found: those 24 h, so its checks report no executions), and
the baseline without a window gets its own the same way. The result has
`window_from_run` / `baseline_window_from_run` and the windows used, and a
recorded evaluation keeps them.

Invalid rules or windows are 400 with a message naming the check. Otherwise
200 with `data.Evaluation`: `contract`, `method`, `run`, windows, `gate`,
`evaluated_at` and per check `id`, `type`, `entry`, `advisory`, `status`,
`reason`, `summary`, `unit`, `value`, `limit`, `target`, `target_status`,
`executions`, `violations`, `details`, `evidence` (trace id, span id, window
to open it) and `baseline` (status, value, executions, summary). A run that
can't be read is a 200 whose checks are `insufficient`, so the gate is
unresolved, never passed. Without `record` nothing is stored, and the same
request on the same data gives the same answer. With `record: true` (and
`source`, optional) see "Requirements and history" above; the evaluation then
also has `id`, `recorded_at` or `record_error` (with `record_unconfirmed`
when the record couldn't be read back), `recorded_before` or
`reevaluated`, and evidence `snapshot`s (`reevaluate: true` needs `record`).
Every evaluation has `assumptions` and, when spans were read, `child_counts`
(see "Assumptions" above).

The history (GET, newest first):

- `GET /api/v1/quality/projects`: `data.Projects`, each `project`,
  `evaluations`, `last_recorded_at`, `last_gate`.
- `GET /api/v1/quality/evaluations?project=&ref=&before=&limit=`:
  `data.Evaluations` (summaries: `id`, `project`, `recorded_at`, `run`,
  `gate`, `source`, the status of each check and requirement) and `data.Next`,
  the `before` (Unix ns) of the next page or empty. `limit` 1–200, default 50.
- `GET /api/v1/quality/evaluations/{id}`: `data.Record` with `evaluation`
  (exactly as recorded) and `rules` (the text); 404 for an unknown id.
- `GET /api/v1/quality/requirements?project=&ref=`: `data.Catalogue` with
  `latest` (the newest summary, or null), `requirements` (the latest
  evaluation's, each with `history`: up to 30 points newest first, `status`
  `absent` where that evaluation's rules didn't have it; and `streak`: `since`,
  the oldest point of the current run of equal statuses, `evaluations` in it,
  `status_before`, or `at_least` when it reaches the end of what was read),
  `checks` (the latest evaluation's, without snapshots) and `unlinked` (checks
  no requirement lists).

## Code

- `log-analysis/internal/data/quality.go`: request validation, the bounded
  canonical span query (`qualityQuery`) and the pure evaluator
  (`evaluateRun`, `requireComplete`, `gateOf`); unit tests in `quality_test.go`.
- `log-analysis/internal/data/quality_requirements.go`: requirements
  (validation, status from checks), `source`, evidence snapshots of a record;
  `quality_history.go`: recording (a native batch insert: the rules text has
  newlines, which a `{name:String}` parameter can't carry), the history and the
  catalogue (`buildCatalogue`, `streakOf`, pure); tests in
  `quality_requirements_test.go` (also every rules file of this repository).
- `log-receiver/internal/driver/schema.go`: `quality_evaluations`.
- `log-analysis/cmd/rest_api/quality.go`: the endpoints.
- `quality-check/` (module `github.com/agerimex/troubleshooting/quality-check`):
  the CLI, standard library plus `gopkg.in/yaml.v3`: `main.go` (check,
  `source`), `reports.go`, `snapshot.go` (evidence snapshots), `comment.go`
  (`troubleshooting-check comment`).
- `quality-check/action.yml` (the gate) and `quality-check/stack/action.yml`
  (Troubleshooting inside a CI job).
- `examples/quality-gate/`: the example application and its `quality.yaml`
  (CI job `quality-example`).
- `fixtures/regression/qg.go`: the reference runs and their literal answers;
  `qghistory.go`: the same runs recorded as three commits (QG-HIST-001).
- `log-ui/src/components/QualityView.vue` (tabs, recorded projects, opening
  a record) with `components/quality/QualityResult.vue` (one evaluation),
  `QualityRequirements.vue` (catalogue) and `QualityHistory.vue`; the pure
  parts in `src/services/quality.ts` (unit tests `quality.spec.ts`,
  `qualityHistory.spec.ts`; browser tests `e2e/quality.e2e.ts`,
  `e2e/qualityHistory.e2e.ts`).
- `.github/quality.yaml`: this repository's rules; `TROUBLESHOOTING_RUN` tags
  the API's own spans (`log-analysis/cmd/rest_api/main.go`).

Stages 1 (API, CLI, fixture), 2 (reports, Quality page, action, own CI) and 3
(trusted rules, provenance, PR comment, Troubleshooting in the job, evidence
snapshots, incomplete executions), 4 (evidence sufficiency, provenance of a
pull request, saved snapshots in the UI) and 5 (requirements, recorded
evaluations, the catalogue and history on the Quality page) are recorded in
`docs/milestone3-completion.md`.

## Not done (only if asked)

A "latest run on main" baseline (needs a run registry; the history now has the
latest recorded evaluation per branch, but not its run's spans after raw
retention), imported requirement revisions with digests and a diff between
them (the record keeps each rules text; the UI marks where it changed),
requirement kinds and support relations, filters of the catalogue by status,
deleting recorded evaluations (today only with SQL: `ALTER TABLE
quality_evaluations DELETE WHERE "Project" = '…'`), load and outage
experiments, sync-call semantics and allowlists, published container images
for the in-job stack (it builds the receiver and API once per version of the
action, about 40 s, and restores them from the Actions cache afterwards).
