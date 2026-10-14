# language: en
@milestone_1 @correlation
Feature: Associate log records with the correct trace and span
  As an engineer reading an investigation report
  I want each association to follow recorded correlation identifiers
  So that unrelated records cannot mislead my investigation

  Background:
    Given an investigation window containing the reference records
    And the trace and log sources are available

  @M1-COR-001 @core
  Scenario: Keep traces separate when their spans have the same SpanId
    Given traces "trace-A" and "trace-B" each contain a span with SpanId "shared-span"
    And log record "log-A" records TraceId "trace-A" and SpanId "shared-span"
    And log record "log-B" records TraceId "trace-B" and SpanId "shared-span"
    When I generate an investigation report for trace "trace-A"
    Then "log-A" should be associated with the span in trace "trace-A"
    And neither the span nor the log record from trace "trace-B" should appear as related evidence

  @M1-COR-002 @core
  Scenario: Correlate evidence across services
    Given trace "recording-A" includes spans from "recording-api" and "recording-worker"
    And both services produced log records with TraceId "recording-A"
    When I generate an investigation report for trace "recording-A"
    Then related log records from both services should be included
    And their recorded service names should remain visible

  @M1-COR-003
  Scenario Outline: Retain trace-related logs without inventing a span association
    Given log record "log-context" records TraceId "recording-A"
    And its SpanId is <span_reference>
    When I generate an investigation report for trace "recording-A"
    Then "log-context" should be included as evidence for that trace
    And its span association should be described as "<association>"
    And it should not be attached to a different span based on its timestamp

    Examples:
      | span_reference                              | association                                  |
      | not recorded                                | No span identifier recorded                  |
      | recorded but not found in the report window | Referenced span not found within the window  |

  @M1-COR-004
  Scenario: Do not infer a trace association from SpanId or time alone
    Given log record "log-unlinked" has no TraceId
    And its SpanId and timestamp match a span in trace "recording-A"
    When I generate an investigation report for trace "recording-A"
    Then "log-unlinked" should not be presented as correlated evidence for that trace
    And the report should not invent its missing TraceId

  @M1-COR-005
  Scenario: Keep span events distinct from independent log records
    Given a span in trace "recording-A" contains an exception event with a recorded stack trace
    And that span has a recorded link to trace "background-B"
    And the log source returns no related log records
    When I generate an investigation report for trace "recording-A"
    Then the exception and its stack trace should be shown as a span event
    And the target identifiers of the span link should be visible
    And the event should not be counted or labeled as an ingested log record
    And the report should not claim that trace "background-B" has been inspected

  @M1-COR-006
  Scenario: Show related logs when the selected trace has no available spans
    Given the trace source returns no spans for TraceId "recording-A" within the window
    And the log source returns log records with TraceId "recording-A" within the window
    When I generate an investigation report for trace "recording-A"
    Then the related log records should remain available
    And the report should state that no spans were found within the searched window
    And it should not manufacture an execution tree from the log messages
