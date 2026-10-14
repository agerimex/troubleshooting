# language: en
@milestone_2 @investigation
Feature: Follow a statistical finding into its combined trace and log evidence
  As an engineer investigating a regression or recurring error
  I want the contributing examples to open the Milestone 1 investigation report
  So that statistics lead to a concrete next investigation action

  Background:
    Given the combined report follows the current Milestone 1 feature specifications
    And the comparison has a recorded analysis revision and explicit population filters

  @M2-INV-001 @core
  Scenario: Open a slow candidate call with its complete available trace context
    Given the candidate population selects service "recording-api", operation "start_recording" and version "1.1.0"
    And it selects environment "production", span kind SERVER and a declared time window
    And the comparison shows a p95 regression signal
    And a retained slow example matches that population and the displayed slow-example threshold
    When I open the example's investigation report
    Then the selected TraceId and SpanId should match the contributing example
    And the originating metric, population and comparison revision should remain identifiable
    And the initial investigation window should be the originating population's window
    And the report should show the available spans and correlated logs for that trace within the window
    And related spans from other services or versions should remain available as execution context
    And supporting INFO and WARN logs should not be hidden by the example-selection filters
    And explicit widening of the investigation window should not change the stored comparison

  @M2-INV-002 @core
  Scenario: Open a log error group even when recorded spans are OK
    Given a candidate error group contains a retained ERROR log with a recorded TraceId and SpanId
    And all recorded spans in that trace have status OK
    When I open that error example's investigation report
    Then the selected error log should be visible with its recorded span association
    And the spans should retain their recorded OK statuses
    And the trace's other available log context should remain accessible
    And the statistical span-error fraction should remain unchanged
    And the report should not declare that the customer task succeeded

  @M2-INV-003
  Scenario: Bound example retrieval without changing aggregate results
    Given the analysis contains 1000 calls and 40 retained calls matching the slow-example threshold
    And the example limit is 5
    When I request slow examples
    Then the first result should show 5 examples with an explicit continuation or truncation indicator
    And all examples should match the selected population and analysis cutoff
    And the statistical call count should remain 1000
    And the example list should not be presented as the full contributing population
    And examples should be labeled as observed slow calls rather than a unique p95 trace

  @M2-INV-004
  Scenario: Distinguish a later investigation from the earlier statistical revision
    Given a saved comparison was generated before a related log arrived
    And that log is now available within the selected trace's investigation window
    When I open a newly generated combined report from the saved comparison
    Then the combined report may include the later log and should show its own generation time
    And the earlier comparison's counts, percentiles and analysis revision should remain unchanged
    And the interface should distinguish the statistical revision from the newer evidence report
    When I explicitly regenerate the comparison
    Then the new comparison should have its own revision and updated applicable evidence
