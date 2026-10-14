# language: en
@milestone_1 @reliability
Feature: Keep the report consistent when evidence is retried or refreshed
  As an engineer comparing evidence during an investigation
  I want retries and delayed responses to preserve correct record identity
  So that repeated delivery or an old response cannot change the meaning of my report

  @M1-REL-001 @core
  Scenario: Replaying the same span and log records does not inflate the report
    Given an accepted fixture contains 2 distinct spans and 3 distinct related log records
    And a report has been generated from that fixture
    When the same payloads are delivered twice more with unchanged record identities and contents
    And ingestion has completed
    And I regenerate the report
    Then it should still contain 2 logical spans and 3 logical log records
    And the span relationships and log associations should be unchanged
    And any displayed matching counts should describe that same logical population

  @M1-REL-002
  Scenario: Preserve separate log events with identical message text
    Given 2 related log records have different stable record identifiers
    And their timestamps, severity and message text are identical
    When I generate the investigation report
    Then both log records should appear
    And they should not be collapsed merely because their contents match

  @M1-REL-003
  Scenario: Incorporate late evidence through an explicit refresh
    Given a report contains a child span whose parent has not arrived
    And it contains a related log whose referenced span has not arrived
    When the missing parent and referenced span are accepted
    And I regenerate the report
    Then the child should be attached to the correct parent
    And the log should be associated with its recorded span
    And the resolved missing-evidence notices should be removed

  @M1-REL-004
  Scenario: Do not let a stale response replace the selected investigation
    Given I requested a report for trace "trace-A"
    And before it completed I requested a report for trace "trace-B"
    When the response for trace "trace-B" arrives
    And the older response for trace "trace-A" arrives afterwards
    Then the displayed report should remain for trace "trace-B"
    And neither its span section nor its log section should be replaced with evidence from "trace-A"

  @M1-REL-005
  Scenario: Retry an unavailable source after recovery
    Given a report shows available spans and an unavailable log source
    When the log source recovers
    And I retry the failed evidence request
    Then the report should include the returned related log records
    And the unavailable-log notice should be removed
    And the report should preserve its selected trace and investigation window
