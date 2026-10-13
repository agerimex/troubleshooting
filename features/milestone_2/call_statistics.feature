# language: en
# Acceptance specification; product behavior and step definitions are not implemented.
@milestone_2 @statistics
Feature: Understand recorded call volume and error fraction for an explicit population
  As an engineer investigating operation behavior
  I want counts and rates with a clear population and coverage
  So that I can identify changes without confusing spans, logs and customer outcomes

  Background:
    Given the metric contract "metrics-v1" from METRIC_CONTRACT.md
    And statistics use canonical records admitted by the configured event-time policy

  @M2-CAL-001 @core
  Scenario: Count the selected operation without counting dependency work as requests
    Given the selected population has 5 SERVER spans for service "recording-api" and operation "start_recording"
    And their recorded statuses are 2 ERROR, 2 OK and 1 UNSET
    And their traces also contain 12 CLIENT dependency spans
    And one of the 5 SERVER spans has a recorded parent
    When I request call statistics for that SERVER population
    Then the recorded call count should be 5
    And the recorded error count should be 2 and its fraction should be 40 percent
    And the OK count should be 2 and the UNSET count should be 1
    And the CLIENT spans should not inflate the request count
    And having a parent should not exclude a matching SERVER span
    And neither the OK count nor the non-error count should be labeled as completed customer tasks

  @M2-CAL-002
  Scenario: Keep versions and environments separate, including missing metadata
    Given these canonical spans match the selected service, operation and span kind:
      | environment | version      | calls |
      | production  | 1.0.0        | 2     |
      | production  | 1.1.0        | 3     |
      | production  | not recorded | 1     |
      | test        | 1.0.0        | 4     |
    When I view the production population grouped by version
    Then versions "1.0.0" and "1.1.0" should have counts 2 and 3
    And an explicit unknown-version group should have count 1
    And the test population should not be included
    And a missing version should not be filled from another service's span in the same trace

  @M2-CAL-003
  Scenario: Use the declared event-time window and elapsed time for call rate
    Given the selected window is from "T" inclusive to "T+120s" exclusive
    And matching spans start at "T", "T+60s" and "T+120s"
    And the source is available throughout that window
    When I request call statistics
    Then the recorded call count should be 2
    And the observed call rate should be 1 call per minute
    And the window, its 120-second duration and the rate unit should be visible
    And the span at "T+120s" should be outside the population

  @M2-CAL-004 @core
  Scenario: Keep logical counts stable through replay and colliding span identifiers
    Given traces "trace-A" and "trace-B" each have one matching span with SpanId "shared-span"
    And the two distinct canonical spans have been included in statistics
    When the same payloads are delivered again in different batch boundaries
    And statistics are refreshed before storage background merges have completed
    Then the recorded call count should remain 2
    And status counts and duration samples should remain unchanged
    And both composite span identities should remain available for investigation

  @M2-CAL-005
  Scenario Outline: Distinguish no observed calls from an unavailable query
    Given the statistics query outcome is "<outcome>"
    When I open the statistics result
    Then the call count should be presented as "<count>"
    And the rate should be presented as "<rate>"
    And the recorded error fraction should be unavailable
    And the source condition should be presented as "<condition>"

    Examples:
      | outcome                                      | count       | rate        | condition                       |
      | successful query with no matching records    | 0           | 0           | no observed calls in this scope |
      | query deadline exceeded without a result     | unavailable | unavailable | statistics query timed out      |

  @M2-CAL-006
  Scenario: Preserve sampling and collection gaps in statistics
    Given the source declares 20 percent head sampling
    And 200 matching spans were recorded
    And the selected window contains a known collection gap
    When I request call statistics
    Then the observed call count should be 200
    And it should not be silently extrapolated to 1000 calls
    And the sampling policy and collection gap should be visible
    And a bucket affected by the gap should not be presented as a confirmed zero
    And the error fraction should describe recorded spans rather than all application traffic

  @M2-CAL-007
  Scenario: Surface conflicting records instead of publishing an unqualified comparison
    Given two different payloads have the same TraceId and SpanId
    And they disagree on the duration or recorded status of a selected call
    When statistics are generated under the configured canonical-record policy
    Then the identity should contribute no more than one logical call
    And the conflict and applied policy should be visible as data-quality information
    And affected populations should be ineligible for a qualified regression or improvement signal
    And a background merge should not silently remove the report's conflict notice

  @M2-CAL-008
  Scenario: Show a time series without turning collection gaps into quiet periods
    Given three consecutive one-hour buckets use the same selected population
    And the first has complete coverage with 100 recorded calls
    And the second has complete coverage with no recorded calls
    And the third has unavailable collection coverage
    When I view the call-count time series
    Then the first two buckets should show 100 and 0 calls respectively
    And the third should show a coverage gap rather than a zero
    And the bucket bounds, UTC time basis and population filters should be visible
    And the three-hour summary should retain an incomplete-coverage qualification
