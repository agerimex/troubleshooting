# language: en
# Acceptance specification; step definitions and product behavior are not implemented by this file.
@milestone_1 @report
Feature: Investigate one selected trace using spans and logs in a single report
  As an engineer investigating a reported failure
  I want the execution steps and their related log messages in one report
  So that I can gather the evidence needed for the next investigation or repair action

  Background:
    Given an investigation window containing the reference operation
    And the trace and log sources are available

  @M1-RPT-001 @core
  Scenario: Combine execution evidence and diagnostic log messages
    Given trace "recording-A" contains these recorded spans:
      | span    | parent  | service          | operation        | status | duration_ms | version |
      | request | none    | recording-api    | start_recording  | OK     | 150         | 1.4.0   |
      | upload  | request | recording-worker | upload_recording | ERROR  | 900         | 1.4.0   |
    And these distinct log records belong to trace "recording-A":
      | record       | span    | severity | message                             |
      | log-accepted | request | INFO     | Recording request accepted          |
      | log-retry    | upload  | WARN     | Upload retry 1 of 3                  |
      | log-denied   | upload  | ERROR    | Storage returned 403 AccessDenied   |
    When I generate an investigation report for trace "recording-A"
    Then the same report should contain both recorded spans and all 3 related log records
    And I should see each span's service, operation, recorded status, duration and available version
    And I should be able to inspect the upload span together with its warning and error messages
    And the report should distinguish the accepted request from the failed upload

  @M1-RPT-002 @core
  Scenario: Find a failed child and open the surrounding investigation
    Given trace "recording-A" contains an OK root span and an ERROR grandchild span
    And the trace has related INFO, WARN and ERROR log records
    When I search for ERROR spans within the investigation window
    And I open the report from the matching grandchild
    Then the report should select trace "recording-A"
    And it should show the available parent-child context including the OK root
    And it should include the related INFO, WARN and ERROR log records
    And the error-only search should not hide the report's supporting context

  @M1-RPT-003 @core
  Scenario: Preserve an error log even when all recorded spans have status OK
    Given every recorded span in trace "recording-A" has status OK
    And a related ERROR log contains "Recording could not be published"
    When I generate an investigation report for trace "recording-A"
    Then the report should show the spans with their recorded OK status
    And it should show the related ERROR log
    And it should distinguish span status from log severity
    And it should not claim that the recording was successfully delivered

  @M1-RPT-004
  Scenario: Keep absent metadata explicit
    Given a recorded span in trace "recording-A" has no service version attribute
    And its related log has no recorded stack trace
    When I generate an investigation report for trace "recording-A"
    Then the service version should be shown as unavailable
    And the log message should remain available
    And the report should not invent a version or stack trace

  @M1-RPT-005 @core
  Scenario: Find an error log and open its report when all spans are OK
    Given trace "recording-A" contains recorded spans "request" and "upload" with status OK
    And these log records belong to trace "recording-A" within the investigation window:
      | record          | span    | service          | severity | message                          |
      | log-accepted    | request | recording-api    | INFO     | Recording request accepted       |
      | log-retry       | upload  | recording-worker | WARN     | Publication retry 1 of 3          |
      | log-unpublished | upload  | recording-worker | ERROR    | Recording could not be published |
    When I search for ERROR logs from service "recording-worker" containing "could not be published" within the investigation window
    Then the search should find "log-unpublished" without requiring me to supply a TraceId
    When I open the investigation report from "log-unpublished"
    Then the report should select its recorded TraceId "recording-A" and preserve the investigation window
    And the selected error log and its association with span "upload" should be visible
    And both spans should retain their recorded OK status
    And the report should include "log-accepted" and "log-retry" as supporting context
    And the log search's service, severity and message filters should not restrict the report's evidence

  @M1-RPT-006
  Scenario: Inspect a matching error log when it has no trace identifier
    Given ERROR log "log-unlinked" is from service "recording-worker" within the investigation window
    And its message is "Recording could not be published"
    And it has no recorded TraceId
    And its SpanId and timestamp match a span in trace "recording-A"
    When I search for ERROR logs from service "recording-worker" containing "could not be published" within the investigation window
    And I select "log-unlinked"
    Then I should be able to inspect that log record
    And the interface should explain that trace correlation is unavailable because no TraceId was recorded
    And it should not open a report for "recording-A" based on the matching SpanId or timestamp

  @M1-RPT-007 @core
  Scenario: Focus on a span's logs and return to the full trace context
    Given trace "recording-A" contains root span "request" and child span "upload"
    And these log records belong to trace "recording-A" within the investigation window:
      | record           | span         | offset_ms | severity | message                           |
      | log-accepted     | request      | 10        | INFO     | Recording request accepted        |
      | log-queued       | not recorded | 20        | INFO     | Recording queued for processing   |
      | log-denied       | upload       | 1000      | ERROR    | Storage returned 403 AccessDenied |
      | log-upload-start | upload       | 200       | INFO     | Upload started                    |
      | log-retry        | upload       | 400       | WARN     | Upload retry 1 of 3                |
    And I have generated the report for trace "recording-A"
    When I select span "upload" to inspect its related logs
    Then the focused log view should show "log-upload-start", "log-retry" and "log-denied" in recorded timestamp order
    And it should show the selected span's recorded service, operation, status and duration
    And the parent-child context should remain accessible in the same report
    And "log-accepted" and "log-queued" should not be labeled as logs of span "upload"
    When I return to all logs for the selected trace
    Then all 5 log records should be visible in recorded timestamp order
    And "log-queued" should remain trace-level evidence without an invented span association
    And the selected TraceId and investigation window should remain unchanged

  @M1-RPT-008
  Scenario: Make the boundary of a single-trace investigation visible
    Given traces "recording-A" and "background-B" belong to business operation "recording-42"
    And both traces have stored spans and correlated logs within the investigation window
    And a span in "recording-A" records a link to a span in "background-B"
    When I generate an investigation report for trace "recording-A"
    Then the report should identify its scope as trace "recording-A" within the selected time bounds
    And its span and log evidence should contain only records correlated with "recording-A"
    And the link to "background-B" should be visible as a reference whose trace has not been inspected
    And the shared business operation identifier should not cause the other trace's evidence to be merged into this report
    And the report should state that it does not establish complete coverage or the final outcome of business operation "recording-42"
