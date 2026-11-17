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
