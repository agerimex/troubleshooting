# language: en
@milestone_2 @comparison
Feature: Identify deterioration and improvement between comparable populations
  As an engineer assessing a release or verifying a fix
  I want explicit baseline and candidate statistics with evidence links
  So that I can investigate meaningful changes without treating correlation as proof

  Background:
    Given the metric contract "metrics-v1" and comparison rules "comparison-v1"
    And baseline and candidate are explicit selections rather than inferred from version name order
    And unless stated otherwise both populations meet coverage and sample requirements

  @M2-CMP-001 @core
  Scenario: Show latency and error regression signals with interpretable differences
    Given the selected operation has these comparable populations:
      | version | calls | error_calls | p95_ms |
      | 1.0.0   | 1000  | 10          | 100    |
      | 1.1.0   | 1000  | 50          | 160    |
    When I compare candidate "1.1.0" against baseline "1.0.0"
    Then the report should show p95 increasing by 60 milliseconds and 60 percent
    And the recorded error fraction should increase from 1 to 5 percent
    And its absolute increase should be 4 percentage points and its relative increase 400 percent
    And both changes should be labeled as regression signals under the recorded rules
    And both population sizes, time windows and filters should be visible
    And the report should not claim that deployment alone proves the cause

  @M2-CMP-002 @core
  Scenario: Verify improvement while retaining the original baseline
    Given the selected operation has these comparable populations:
      | version | calls | error_calls | p95_ms |
      | 1.0.0   | 1000  | 10          | 100    |
      | 1.1.0   | 1000  | 50          | 160    |
      | 1.1.1   | 1000  | 10          | 105    |
    When I compare candidate "1.1.1" against baseline "1.1.0"
    Then the report should show p95 decreasing by 55 milliseconds and approximately 34.38 percent
    And the recorded error fraction should decrease by 4 percentage points and 80 percent relatively
    And both changes should be labeled as improvement signals
    When I compare "1.1.1" against the original baseline "1.0.0"
    Then p95 should be 5 milliseconds and 5 percent higher
    And the recorded error fraction should be unchanged
    And both metrics should be within the configured change thresholds
    And within-threshold results should not be described as proof of identical behavior

  @M2-CMP-003
  Scenario Outline: Handle a zero baseline without an invalid relative percentage
    Given the comparable "<metric>" values are <baseline> and <candidate> in "<unit>"
    When I compare candidate against baseline
    Then the absolute change should be <delta> in "<unit>"
    And the relative percentage should be unavailable because the baseline is zero
    And the change classification should be "<classification>"
    And the report should not show infinity or a fabricated relative percentage

    Examples:
      | metric                 | baseline | candidate | unit              | delta | classification    |
      | p95                    | 0        | 100       | milliseconds      | 100   | regression signal |
      | recorded error fraction| 0        | 2         | percentage points | 2     | regression signal |
      | recorded error fraction| 0        | 0         | percentage points | 0     | within threshold  |

  @M2-CMP-004
  Scenario: Compare two windows without confusing exposure time with call volume changes
    Given version "1.1.0" has 2000 calls in a complete one-hour baseline window
    And it has 4000 calls in a comparable complete two-hour candidate window
    When I compare the two windows
    Then the call count difference should be 2000
    And the observed call rates should both be approximately 33.33 calls per minute
    And the different window durations should be visible
    And the larger raw count alone should not be labeled a latency or reliability regression

  @M2-CMP-005
  Scenario: Preserve mixed results across metrics
    Given comparable baseline and candidate p95 values are 200 and 100 milliseconds
    And their recorded error fractions are 1 and 5 percent
    When I compare the populations
    Then latency should show an improvement signal
    And the recorded error fraction should show a regression signal
    And neither metric should hide the other behind an overall healthy or successful-release label

  @M2-CMP-006
  Scenario Outline: Make a known population or definition mismatch explicit
    Given baseline and candidate differ in "<dimension>" beyond their selected version or time window
    When I request a comparison
    Then both population definitions should be shown
    And the mismatch in "<dimension>" should be explained
    And a qualified conclusion for the affected metric or error group should be unavailable until the mismatch is resolved
    And any individually available statistics should remain inspectable
    And a grouping-rule mismatch alone should not block otherwise compatible call or latency comparisons

    Examples:
      | dimension                  |
      | environment                |
      | service and operation      |
      | span kind                  |
      | declared workload segment  |
      | sampling policy            |
      | metric definition version  |
      | error grouping rule version|

  @M2-CMP-007
  Scenario Outline: Withhold classification when the selected metric lacks enough evidence
    Given the selected comparison metric has "<limitation>"
    When I request its change classification
    Then the classification should be insufficient evidence
    And the specific limitation should be visible
    And any available counts and measurements should retain their own quality labels
    And missing data should not be replaced by zero or an older successful query

    Examples:
      | limitation                                              |
      | only 50 candidate duration samples for p95               |
      | only 50 candidate calls for recorded error fraction      |
      | an unknown sampling policy                              |
      | a collection gap in the selected window                  |
      | an unavailable baseline query                           |

  @M2-CMP-008
  Scenario: Keep an operation with no candidate observations visible
    Given the baseline contains 1000 recorded calls for operation "publish_recording"
    And the candidate query succeeds with no matching calls
    When I compare operation populations
    Then the baseline-only operation should remain visible with candidate call count 0
    And candidate latency and recorded error fraction should be unavailable
    And the result should state that no candidate calls were observed
    And it should not describe the operation as fixed, faster or removed from the application
