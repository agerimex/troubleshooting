# language: en
@milestone_2 @errors
Feature: Find recurring error evidence and investigate its examples
  As an engineer reviewing failures
  I want stable error groups with distinct occurrence and affected-record counts
  So that I can prioritize repeated problems without inventing business impact

  Background:
    Given the error grouping contract "errors-v1" from METRIC_CONTRACT.md
    And raw diagnostic text remains available under the configured retention policy

  @M2-ERR-001 @core
  Scenario: Group equivalent exceptions while preserving different failure locations
    Given the selected population has these exception events:
      | event   | exception_type | application_frame | request_id |
      | event-A | StorageError   | storage.Upload    | request-A  |
      | event-B | StorageError   | storage.Upload    | request-B  |
      | event-C | StorageError   | auth.Authorize    | request-C  |
    When I open error groups
    Then "event-A" and "event-B" should belong to one exception group with 2 occurrences
    And "event-C" should belong to a separate exception group
    And the grouping rule version and recorded source kind should be visible
    And the grouping should not claim that every event in a group has the same confirmed root cause

  @M2-ERR-002
  Scenario: Label message-based grouping and preserve meaningful error codes
    Given exception metadata is absent from these ERROR logs:
      | log   | message                                     |
      | log-A | Request request-A failed with status 403     |
      | log-B | Request request-B failed with status 403     |
      | log-C | Request request-C failed with status 500     |
    And the configured normalization rule replaces request identifiers but preserves status codes
    When I open error groups
    Then "log-A" and "log-B" should share a message-based group
    And "log-C" should belong to a different group
    And the fallback grouping method should be visible
    And each example should retain its original recorded message

  @M2-ERR-003 @core
  Scenario: Keep log error occurrences separate from failed-call statistics
    Given one matching span has recorded status OK
    And its trace contains 3 distinct correlated ERROR logs in the same error group
    When I inspect the error group and call statistics
    Then the group should show 3 log occurrences and 1 affected trace
    And the recorded span-error count should remain 0 out of 1 call
    And the ERROR logs should remain available for investigation
    And neither 3 failed calls nor 1 failed customer task should be inferred

  @M2-ERR-004
  Scenario: Preserve an unlinked error without inventing release attribution
    Given an ERROR log has a recorded service but no TraceId, SpanId or service version
    And spans from version "1.1.0" occurred at a similar time
    When I inspect error groups for that service
    Then the log should appear as unlinked evidence under an unknown-version population
    And its original text and available event time should remain inspectable
    And it should not be assigned to version "1.1.0" using time proximity
    And missing trace linkage should not remove the error occurrence

  @M2-ERR-005
  Scenario: Compare group occurrence counts within explicit baseline and candidate scopes
    Given comparable baseline and candidate populations use the same grouping rule version
    And their error-group occurrence counts are:
      | group | baseline | candidate |
      | A     | 10       | 3         |
      | B     | 0        | 2         |
      | C     | 4        | 0         |
    When I compare error groups
    Then group "A" should be labeled recurring within the comparison
    And group "B" should be labeled newly observed in the candidate scope
    And group "C" should be labeled not observed in the candidate scope
    And each group should show first and last occurrence within each selected scope
    And those labels should not claim first-ever occurrence or a permanently resolved problem
    When I regenerate the comparison while baseline error coverage is unavailable
    Then the newly generated result should label group "B" novelty as undetermined
    And it should not reuse the earlier zero as a current confirmed baseline count

  @M2-ERR-006
  Scenario: Deduplicate recorded events without collapsing separate observations
    Given two distinct ERROR log identities have identical messages and timestamps
    And a canonical ERROR span also contains one exception event
    When the same logs and span payload are replayed
    And error groups are refreshed
    Then the log occurrence count should remain 2
    And the error-status span occurrence count should remain 1
    And the exception-event occurrence count should remain 1
    And the three source kinds should remain distinguishable
    And their occurrence counts should not be summed and labeled as distinct incidents
