# language: en
@milestone_1 @export
Feature: Share the same investigation evidence in an exported report
  As an engineer handing an investigation to a colleague
  I want a self-contained export of the report I reviewed
  So that the colleague can inspect the same spans, logs and limitations for the selected trace

  @M1-EXP-001 @core
  Scenario Outline: Export the combined report without requiring access to the running service
    Given I have generated a report containing spans, related log records and a recorded exception
    When I export that report as "<format>"
    Then the artifact should contain the same evidence identifiers and values as the displayed report
    And it should include the selected TraceId, time bounds and report generation time
    And it should identify its scope as one selected trace within those time bounds
    And it should preserve span relationships, log associations and evidence source labels
    And its stored contents should be readable without querying the running Troubleshooting service

    Examples:
      | format |
      | HTML   |
      | JSON   |

  @M1-EXP-002
  Scenario Outline: Preserve incomplete evidence in the export
    Given the displayed report has an unavailable log source and a truncated span result
    When I export that report as "<format>"
    Then the artifact should retain both limitations
    And it should distinguish unavailable evidence from a successful empty result
    And it should not present the report as complete

    Examples:
      | format |
      | HTML   |
      | JSON   |

  @M1-EXP-003
  Scenario: Export the reviewed report rather than silently running a new investigation
    Given I have generated a report containing log record "log-original"
    And log record "log-late" arrives after that report was generated
    When I export the displayed report
    Then the export should contain "log-original"
    And it should not silently add "log-late"
    When I explicitly regenerate the report
    Then the new report should include "log-late" if it matches the selected trace and window

  @M1-EXP-004
  Scenario: Treat recorded message text as content in an HTML export
    Given a related log message contains:
      """
      <script>alert('recorded-log-text')</script>
      """
    When I export the investigation report as "HTML"
    And I open the exported artifact
    Then the recorded message should be visible as text
    And the message should not execute as a script

  @M1-EXP-005
  Scenario Outline: Keep all generated evidence when exporting from a focused span view
    Given I have generated a report for trace "recording-A" containing spans "request" and "upload"
    And the generated report includes logs associated with each span and a trace-level log without a SpanId
    And the report displays a link to trace "background-B" marked as not inspected
    When I select span "upload" to focus its related logs
    And I export the report as "<format>"
    Then the artifact should retain both spans and all log records from the generated report
    And it should preserve the trace-level log without attaching it to span "upload"
    And it should preserve the reference to "background-B" and its not-inspected status
    And the focused view should not narrow the exported investigation scope

    Examples:
      | format |
      | HTML   |
      | JSON   |
