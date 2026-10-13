# language: en
@milestone_1 @completeness
Feature: Make the limits of investigation evidence visible
  As an engineer deciding what an investigation supports
  I want unavailable, missing and limited evidence to be explicit
  So that an incomplete report cannot look like proof that no error occurred

  @M1-CMP-001 @core
  Scenario Outline: Preserve available evidence when one source fails
    Given trace and log records exist for trace "recording-A" within the investigation window
    And the "<failed_source>" source is unavailable
    And the other source responds successfully
    When I generate an investigation report for trace "recording-A"
    Then the report should include the evidence returned by the responding source
    And it should mark "<failed_source>" evidence as unavailable
    And it should mark the report as incomplete
    And it should not describe the failed source as having zero matching records

    Examples:
      | failed_source |
      | trace         |
      | log           |

  @M1-CMP-002
  Scenario: Distinguish a successful empty log search from an unavailable source
    Given the log source successfully returns no related records for trace "recording-A"
    When I generate its investigation report within the selected window
    Then the log section should say "No correlated logs found within the selected window"
    And it should not say that the log source is unavailable
    And it should not claim that the operation produced no logs outside that window

  @M1-CMP-003
  Scenario Outline: Describe a result limit accurately
    Given trace "recording-A" has <available> matching "<kind>" records within the selected window
    And the configured report limit for "<kind>" records is 3
    When I generate its investigation report
    Then the report should include 3 "<kind>" records
    And that section should be marked "<completeness>"
    And the report should label that number as records shown

    Examples:
      | kind | available | completeness |
      | span | 3         | complete     |
      | span | 4         | truncated    |
      | log  | 3         | complete     |
      | log  | 4         | truncated    |

  @M1-CMP-004
  Scenario: Use the selected window rather than a parent's duration as the evidence boundary
    Given a parent span ends before its asynchronous child starts
    And a related log is recorded after the parent ends
    And all 3 records are inside the selected investigation window
    When I generate the report for their trace
    Then the parent, child and related log should all be included
    And the report should show the selected time bounds

  @M1-CMP-005
  Scenario: Explain a missing parent within the searched scope
    Given a child span is inside the selected investigation window
    And its recorded parent is outside that window
    When I generate the report for their trace
    Then the child should remain visible
    And its parent should be described as not found within the searched window
    When I widen the investigation window to include the parent
    And I regenerate the report
    Then the parent should be visible with its correct child association

  @M1-CMP-006
  Scenario: Explain a retention limit without inventing deleted evidence
    Given spans for trace "recording-A" remain within the retained trace history
    And the selected window is older than the configured log retention period
    And expired log records in that window have already been removed
    When I generate an investigation report for trace "recording-A"
    Then the available spans should be shown
    And the report should warn that related logs may have expired under the configured retention policy
    And it should not interpret their absence as proof that no error was logged

  @M1-CMP-007
  Scenario: A timed-out query cannot produce a complete-looking report
    Given one evidence query reaches its configured deadline before completing
    When the investigation report is displayed
    Then the affected section should show a timeout and offer retry
    And the report should be marked incomplete
    And any partial result should not be represented as a complete result

  @M1-CMP-008
  Scenario: Do not retain an old total when the current count is unavailable
    Given a previous report displayed 12 related log records
    And a new report request returns 3 related log records successfully
    And the request for the new matching total fails
    When the new report is displayed
    Then it should show the 3 returned records
    And the matching total should be shown as unavailable
    And it should not reuse 12 or display zero as the matching total
