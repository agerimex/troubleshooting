# language: en
@milestone_2 @reference_example
Feature: Reproduce a regression investigation and verify the subsequent fix
  As another engineer evaluating Troubleshooting
  I want a documented example spanning statistics, errors and combined evidence
  So that I can verify the original product workflow before relying on it for real incidents

  Background:
    Given the reference fixture from REFERENCE_FIXTURE.md
    And a supported sender stores its spans and structured correlated logs
    And both populations use the declared unsampled workload and metric contracts

  @M2-REF-001 @core
  Scenario: Detect a known regression and investigate its diagnostic evidence
    Given reference versions "1.0.0" and "1.1.0" have been ingested
    And their eligible statistical revisions are ready
    When I compare candidate "1.1.0" against baseline "1.0.0"
    Then each version should show 1000 observed upload calls
    And p50, p95 and p99 should change from 100 to 160 milliseconds within the fixture tolerance
    And the recorded error fraction should change from 1 to 5 percent
    And the StorageError group should show 10 baseline and 50 candidate exception occurrences
    When I open a candidate error example in the combined investigation report
    Then its recorded upload span and parent request context should be visible
    And its associated log should show "Storage returned 403 AccessDenied"
    And that diagnostic message should not be present in the fixture's span attributes or span events
    And the report should not claim a specific credential or policy is the confirmed cause
    When I export the comparison with that combined report
    Then the observed regression and its selected trace-and-log evidence should be inspectable together

  @M2-REF-002 @core
  Scenario: Verify a fix against both the affected release and the original baseline
    Given the regression comparison for "1.0.0" and "1.1.0" has been saved
    And reference version "1.1.1" is ingested using the same workload and sampling policy
    And its eligible statistical revision is ready
    When I compare "1.1.1" against "1.1.0"
    Then its 1000 calls should show p50, p95 and p99 of 105 milliseconds within the fixture tolerance
    And the recorded error fraction should return to 1 percent
    And the report should show improvement signals for latency and recorded error fraction
    When I compare "1.1.1" against "1.0.0"
    Then the same metrics should be within the configured change thresholds
    And the original regression report should remain available as a separate result
    And the verification should be limited to the measured technical operation and workload
    And no customer-delivery success rate should be inferred from these span statistics
