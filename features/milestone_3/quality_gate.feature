@milestone3
Feature: Check a test run against quality rules
  A developer or a coding agent runs the tests of a change with its spans
  tagged troubleshooting.run, then checks that run against rules kept in the
  repository. Every failure points to the execution behind it. Missing
  evidence never passes. Contract quality-gate-v1 (docs/quality-gate.md).

  Background:
    Given the rules for recording-api start_session (server):
      | id           | type           | limit                | target |
      | startup-p95  | latency        | p95 <= 800 ms        | 500 ms |
      | db-calls     | db_calls       | <= 3 per execution   | 2      |
      | no-directory | forbidden_call | no call to directory |        |
    And the reference runs of 200 executions each:
      | run       | p95    | database calls per execution          | directory call |
      | baseline  | 420 ms | permission_lookup 1, insert_session 1 | no             |
      | candidate | 910 ms | permission_lookup 20, insert_session 1 | yes, every one |
      | fixed     | 560 ms | permission_batch_lookup 1, insert 1   | no             |
    And only the entry service's spans carry the run attribute

  @QG-REF-001
  Scenario: A change that adds work fails with its evidence
    When I check run candidate with baseline baseline
    Then the gate is failed
    And startup-p95 fails with 910 ms over 200 executions, baseline 420 ms, target open
    And db-calls fails with 21 per execution in all 200 executions, baseline 2
    And its details are "permission_lookup × 20" and "insert_session × 1"
    And no-directory fails in 200 of 200 executions, with the CLIENT call as evidence
    And each evidence window opens the execution's whole trace

  @QG-REF-002
  Scenario: The fix passes; a pass is not "every number improved"
    When I check run fixed with baseline baseline
    Then the gate is passed
    And startup-p95 passes with 560 ms, its target 500 ms is still open, and the baseline 420 ms is shown
    And db-calls passes with 2, target met
    And no-directory passes as "not observed in this run"

  @QG-EVD-001
  Scenario Outline: Missing evidence never passes
    When I check a run with <situation>
    Then the affected checks are insufficient with reason <reason>
    And the gate is unresolved

    Examples:
      | situation                                  | reason             |
      | 19 executions for a p95 (20 needed)        | too_few_executions |
      | a run id without executions                | no_executions      |
      | a window outside raw span retention        | outside_retention  |
      | a query over its limit                     | query_limit        |
      | more spans than the evaluation bound       | too_many_spans     |

  @QG-VAL-001
  Scenario: Invalid rules are rejected before any query
    When the rules have a target above the limit, a duplicate id, an unknown type or no entry
    Then the API answers 400 naming the check
    And the CLI exits 3 without a result

  @QG-LAT-001
  Scenario: The limit is inclusive
    Given a run whose p95 is exactly 800 ms
    Then startup-p95 passes, and at 800.001 ms it fails

  @QG-GATE-001
  Scenario: Gate precedence
    Then a blocking failure fails the gate even when other checks lack data
    And a blocking check without enough data leaves it unresolved
    And advisory checks are shown but never decide the gate
    And the baseline never changes a status

  @QG-DEP-001
  Scenario: Calls to an uninstrumented service and caller filters
    Given a CLIENT span naming billing as peer.service
    Then a forbidden call to billing is found
    And "from" restricts the caller, and calls inside the forbidden service are not calls to it

  @QG-RUN-001
  Scenario: A run is selected by its attribute and window only
    Then spans of another run are never counted
    And entry spans outside the window are not executions
    And replaying every span leaves every value unchanged

  @QG-CLI-001
  Scenario: The CLI exit code follows the gate
    Then it exits 0 passed, 1 failed, 2 unresolved and 3 on any error
    And a network error or a non-API answer is never exit 0
    And misspelled or duplicate keys in the rules file are errors
    And --json writes the evaluation with a viewer link for every evidence

  # Stage 2: reports, the Quality page and CI.

  @QG-API-001
  Scenario: The rules can be sent as the YAML file's text
    When the request carries rules_yaml instead of rules
    Then it is evaluated like the parsed rules
    And unknown or duplicate keys, a second document, more than 64 KiB, or both rules and rules_yaml are a 400

  @QG-OUT-001
  Scenario: HTML and Markdown reports show the evaluation and never execute recorded text
    When the CLI writes --html and --markdown
    Then both show the gate, every check, the baseline and the evidence links
    And recorded names with markup are shown as text, the HTML has no scripts
    And only http(s) evidence links become links
    And the Markdown details only the checks that failed or lack data

  @QG-UI-001
  Scenario: The Quality page checks a run and opens its evidence
    When I paste the rules, name the run and a baseline and press Check run
    Then the rules go to the API as typed, with the selected window
    And the gate and every check are shown with value, limit, target, baseline and executions
    And "Open timeline" opens the evidence execution with its span, and Back returns to Quality

  @QG-UI-002
  Scenario: Invalid rules on the page show the API's message and no result

  @QG-UI-003
  Scenario: A CI result file opens on the page; other files are refused

  @QG-UI-004
  Scenario: The rules and run ids stay in this browser only

  @QG-CI-001
  Scenario: The GitHub Action applies the gate
    Given the action checked a run
    Then the job summary has the Markdown report and the artifact has result.json, report.html and summary.md
    And fail-on unresolved fails the step for failed and unresolved, fail-on failed only for failed, never for neither
    And an error (exit 3) or an unknown fail-on value always fails it
    And inputs reach the scripts only as environment variables

  @QG-CI-002
  Scenario: This repository's CI checks its own API
    Given the test stack's API tags its spans with TROUBLESHOOTING_RUN
    When the regression fixture has run
    Then the database calls per request of trace-report, view-spans, stats/calls, compare and quality/evaluate are within .github/quality.yaml
    And the build fails with the request behind any check over its limit

  # Stage 3: trusted rules, provenance, pull request feedback, an application's CI.

  @QG-INC-001
  Scenario: An incomplete execution can't establish a count or an absence
    Given an execution of the run with a span whose parent was not recorded, starting within the entry
    Then db-calls and no-directory are insufficient with reason incomplete_executions
    And the evidence opens that execution with the orphaned span selected
    But a counted violation still fails, and the latency check is unaffected
    And the entry's own missing parent, or an orphan outside the entry's duration, keeps it complete

  @QG-SRC-001
  Scenario: The result names the tested commit and the rules that decided
    When the CLI checks a run with --commit, --ref, --ci-run and --rules-ref
    Then the result's source has them and the sha256 of the rules file
    And the text, Markdown and HTML reports and the Quality page show them, linking the CI run only if it is http(s)

  @QG-CI-003
  Scenario: A pull request can't loosen its own gate
    Given the action with rules-from base
    When the pull request changes quality.yaml
    Then the gate uses the base branch's rules and the summary says so
    And when the base has no rules file yet, this commit's file is used with a notice
    And on a push the checked-out file is used

  @QG-CI-004
  Scenario: One pull request comment, updated, never stale
    When the action runs again on the same pull request
    Then it updates its own comment instead of adding one, and never edits a person's comment
    And it posts nothing when the pull request's head is no longer the tested commit
    And a read-only token (a fork pull request) skips the comment without failing the step

  @QG-CI-005
  Scenario: An application's CI needs no Troubleshooting server
    Given quality-check/stack started Troubleshooting inside the job
    When the example application's tests export their spans to it
    Then the gate passes, and with the planted regression it fails on database calls and the forbidden call

  @QG-OUT-002
  Scenario: The report keeps the evidence after raw spans expire
    When the CLI writes --json or --html
    Then each evidence keeps a snapshot of its trace's spans, shown as a static timeline with the evidence span highlighted
    And a failed read is noted and never changes a status

  # Stage 4: verdicts that stand on enough evidence; saved evidence in the UI.

  @QG-SUF-001
  Scenario: A run that declares its executions reports lost ones
    Given quality.yaml declares run.executions 50
    When only 41 executions of the entry were recorded
    Then every check is insufficient with reason missing_executions, never a pass

  @QG-SUF-002
  Scenario: No database call in a whole run is not a pass
    Given a db_calls check with max_per_execution 3
    When no execution of the run recorded a database call
    Then it is insufficient with reason no_database_calls and says how to declare an operation that makes none
    But with max_per_execution 0 it passes

  @QG-SUF-003
  Scenario: Executions that never ended can't support a latency pass
    Given 18 of 20 executions have no valid duration and the other 2 are within the limit
    Then the latency check is insufficient with reason invalid_durations and an execution without a duration as evidence
    But a limit already broken by the measured executions still fails

  @QG-SRC-002
  Scenario: A pull request's merge commit and head are recorded separately
    When the action checks a pull request
    Then source.commit is the checked-out commit and source.pr_head its head, shown as "commit …, merging head …"
    And the comment is skipped once the head has moved on

  @QG-UI-005
  Scenario: A CI result's saved snapshot opens when the live spans are gone
    Given a result file whose evidence trace is no longer on this Troubleshooting
    When Open timeline or Open sequence is clicked
    Then the viewer shows the saved snapshot, marked as such, with logs "not kept", not "none"
    But while the live spans exist, the live report is shown

  # Stage 5: requirements and their history.

  @QG-REQ-001
  Scenario: A requirement's status comes from its own checks
    Given rules whose requirement REQ-START lists startup-p95 and db-calls, and REQ-OUTAGE lists no check
    When the candidate run is checked
    Then REQ-START is violated with "failed: startup-p95, db-calls"
    And REQ-OUTAGE is not evaluated: "no check enforces it yet", never met
    And with one check failing and another without data the requirement stays violated and names the missing data
    And the gate is still decided by the checks

  @QG-REQ-002
  Scenario: Requirements are checked like the rest of the rules
    Then a requirement naming a check that isn't in the rules, listing one twice, without a title, with a duplicate id or an unknown key is a 400
    And the project defaults to the entry's service

  @QG-HIST-001
  Scenario: Recorded evaluations become each requirement's history
    Given the baseline, candidate and fix recorded as three commits of main
    Then the history lists them newest first with their gate, commit and rules sha256
    And the catalogue shows REQ-START met, its history met, violated, met, and "met since" the fix's commit, violated before
    And a recorded evaluation keeps its rules text and a snapshot of each evidence execution
    And an unknown evaluation id is a 404

  @QG-HIST-002
  Scenario: A retried record is one record
    When the same evaluation is recorded again with the same inputs
    Then the answer has the first record's id and time, and the history still has three evaluations
    But a new CI attempt is a new record

  @QG-HIST-003
  Scenario: Recording is chosen per run and never decides the gate
    Then the CLI records only with --record and the action by default only on push events
    And a failed write is a warning with the result unchanged and the exit code from the gate

  @QG-UI-006
  Scenario: The Quality page shows a project's requirements with their history
    Given recorded evaluations of a project
    Then the page opens on Requirements, with each requirement's status, owner, reason, checks and since when its status holds
    And its history cells run oldest to newest, and a cell opens that evaluation with its rules and a copyable link

  @QG-UI-007
  Scenario: The history lists recorded evaluations; a link reopens one
    Then History lists the evaluations newest first, page by page
    And #/quality?evaluation=<id> opens that evaluation, and an unknown id is an error, not an empty page

  @QG-UI-008
  Scenario: Nothing recorded yet
    Then the page opens on Check a run, and Requirements explains how to record

  # Stage 6: a correctness pass after the web agent's review of ede9d2e.

  @QG-SUF-004
  Scenario: Lost child spans can't turn a failure into a pass
    Given 200 executions with all entry spans recorded, whose spans record how many children they started
    When 19 of 20 database calls and the whole forbidden branch of every execution are lost
    Then db_calls and forbidden_call are insufficient with reason incomplete_executions, never a pass
    But without recorded child counts the loss can't be seen, and the result's assumptions say so

  @QG-SUF-005
  Scenario: An execution without its database calls is not a pass
    Given a db_calls check with min_per_execution 2
    When 5 executions recorded only 1 database call
    Then the check is insufficient with reason missing_database_calls and the execution with the fewest calls as evidence
    But a limit already broken still fails

  @QG-SUF-006
  Scenario: Every result says what its verdicts rest on
    Then the result's assumptions say that verdicts hold for the recorded spans, which losses are caught, and whether lost child spans can be seen in this data
    And the CLI's reports and the Quality page show them

  @QG-HIST-004
  Scenario: A retry is answered with the record, never with a new verdict
    Given an evaluation recorded as passed
    When the same request is retried after late spans changed the data
    Then the answer is the recorded evaluation, passed, with recorded_before, and nothing new is recorded
    And --reevaluate records a new evaluation with an id of its own, marked as a re-evaluation

  @QG-EVD-002
  Scenario: A snapshot keeps the counterexample in a large trace
    Given 220 spans in the evidence trace that start before the forbidden call
    Then the snapshot keeps the calling span, its ancestors and the callee, within 200 spans
    And a db_calls evidence keeps all its database calls

  # Stage 7: the review of 0011a2d.

  @QG-HIST-005
  Scenario: Concurrent records of one evaluation are one record
    Given two requests that record the same evaluation inputs at the same time, to one API or to two
    When both look up the id before either has inserted
    Then the history keeps one record of that id, and both answers are that record: the same verdict and recorded time
    And an answer whose own result is not the stored record is replaced by it and has recorded_before

  @QG-HIST-006
  Scenario: A streak never spans different criteria
    Given REQ-START met under a latency limit of 1200 ms, then met under 800 ms
    Then the catalogue says "Met since" the first evaluation under 800 ms, "when its checks changed; met under the earlier checks"
    But wording, targets, advisory flags, guardrails and other requirements' checks don't change its criteria
    And for evaluations recorded before criteria existed, a change of the rules file ends the streak

  @QG-REQ-003
  Scenario: A guardrail supports a requirement without deciding its status
    Given REQ-START assessed by startup-p95 with db-calls as its guardrail
    When starts take 400 ms with 20 database calls each
    Then REQ-START is met and says "guardrail failed: db-calls (guardrails decide the gate, not this requirement)"
    And the gate still fails on db-calls
    And a guardrail that isn't in the rules, is listed twice or is also one of the requirement's checks is a 400

  # Stage 8: the review of b7c2e87.

  @QG-SUF-007
  Scenario: Child-span coverage is stated per execution
    Given 200 executions, one complete with child counts and 199 without counts that lost 19 database calls and the directory branch each
    When the run is checked
    Then the gate passes on the recorded spans, and the assumptions say lost child spans are detected in 1 of 200 executions only
    And child_counts is 200 executions, 1 counted, partial
    But with run.child_counts: required, db-calls and no-directory are insufficient with reason unverified_child_counts, and a failure still stands

  @QG-HIST-007
  Scenario: An unconfirmed record is not reported as recorded
    Given the evaluation was inserted but reading the stored record back failed
    Then the answer keeps the id, has record_unconfirmed and says the recorded verdict may differ
    And the CLI prints "Not confirmed: the evaluation was sent to the history as" with the link, never "Recorded as evaluation"
    And the Quality page says the same for such a result file

  # Stage 9: quick wins after the first external pilot.

  @QG-WIN-001
  Scenario: Without a window, each run's own is used
    Given the candidate and baseline runs of the reference fixture
    When the candidate is checked against the baseline without time_from, time_to or a baseline window
    Then the answer is the same as with the runs' exact windows, with window_from_run and baseline_window_from_run
    And both windows lie inside the runs' half hour
    And the CLI sends no window unless --since or --from/--to is given, and falls back to the last --since against an API that needs one

  @QG-EVD-006
  Scenario: Database calls are explained by their statements
    Given each database call of an execution is followed by an otelsql sql:rows_close span with db.system
    Then db_calls counts the calls only, and the details say the rows spans were not counted
    And the details list the worst execution's calls by db.statement, so a missing statement shows

  @QG-DOC-001
  Scenario: Results define a guardrail and what a pass means
    Given a result with a requirement that has guardrails
    Then the CLI text, the HTML report and the Markdown summary define a guardrail once
    And docs/quality-gate.md, the handbook and the README say in their first paragraph that a pass means "not observed in this run"

  # Stage 10: privacy by default.

  @PRV-001
  Scenario: log-sender keeps SQL arguments, tokens and URL credentials out of what it sends
    Given a span with SQL arguments, a token in http.target, a camera URL with user:password as an attribute key and value, and an event with credentials in url.full
    Then the span arrives without the arguments, with the path only, with the URLs without user:password, and with troubleshooting.redacted listing the keys
    And with WithSQLArgs the arguments arrive, log-sender logs that it sends them and the resource has troubleshooting.sql_args=sent
    And log records lose the same, message included

  @PRV-002
  Scenario: The receiver holds plain OpenTelemetry senders to the same rules
    Given the same span from a sender that doesn't redact
    Then the receiver stores it cleaned, keeps SQL arguments only from a sender marked troubleshooting.sql_args=sent, and leaves clean spans unchanged
    And TROUBLESHOOTING_REDACT=off stores everything as sent

  @PRV-003
  Scenario: The privacy scan finds what got through, never showing the value
    Given stored spans with SQL arguments, a token in a query string, URL credentials, a bearer token and a bcrypt hash, and a span whose sender opted in to SQL arguments
    When troubleshooting-check privacy scans the service
    Then it reports each kind with its count and an example span, exits 1, and doesn't flag the opted-in span
    And a clean service exits 0 with "Nothing found"
