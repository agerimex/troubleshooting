# language: en
@milestone_2 @latency
Feature: Inspect duration distributions for recorded operations
  As an engineer investigating slow calls
  I want p50, p95 and p99 with units, sample sizes and calculation details
  So that tail latency remains visible and comparable

  Background:
    Given the metric contract "metrics-v1" from METRIC_CONTRACT.md
    And the selected population and data revision are fixed
    And the default sample thresholds and fixture tolerances apply

  @M2-LAT-001 @core
  Scenario: Convert stored duration units consistently
    Given 1000 matching canonical spans each have duration 150000 microseconds
    When I request latency statistics
    Then p50, p95 and p99 should each be 150 milliseconds within the fixture tolerance
    And the valid duration sample count should be 1000
    And the percentile method, method version and output unit should be recorded
    And the numeric values should represent duration rather than wall-clock timestamps

  @M2-LAT-002 @core
  Scenario: Preserve the slow tail of the duration distribution
    Given the matching canonical spans have these durations:
      | calls | duration_ms |
      | 900   | 100         |
      | 80    | 300         |
      | 20    | 1000        |
    When I request latency statistics
    Then p50 should be 100 milliseconds within the fixture tolerance
    And p95 should be 300 milliseconds within the fixture tolerance
    And p99 should be 1000 milliseconds within the fixture tolerance
    And all three values should state a sample count of 1000

  @M2-LAT-003
  Scenario: Combine unequal time buckets using their distributions
    Given one complete bucket contains 990 calls of 100 milliseconds
    And another complete bucket contains 10 calls of 1000 milliseconds
    When I request p95 over both buckets
    Then the duration sample count should be 1000
    And p95 should be 100 milliseconds within the fixture tolerance
    And the result should not be the arithmetic average of the two bucket p95 values
    And recomputing from the same canonical raw records should agree within the documented tolerance

  @M2-LAT-004
  Scenario Outline: Apply explicit sample-size heuristics to each percentile
    Given the selected population has <samples> valid duration samples
    When I request "<percentile>" under the default sample policy
    Then its evidence status should be "<status>"
    And the sample count and required minimum should be visible
    And the minimum should be described as a product heuristic rather than a statistical guarantee

    Examples:
      | percentile | samples | status              |
      | p50        | 19      | insufficient sample |
      | p50        | 20      | available           |
      | p95        | 199     | insufficient sample |
      | p95        | 200     | available           |
      | p99        | 999     | insufficient sample |
      | p99        | 1000    | available           |

  @M2-LAT-005
  Scenario: Keep invalid durations out of percentiles without hiding recorded calls
    Given 1000 matching canonical spans have valid duration 100 milliseconds
    And 1 additional matching canonical span has a valid recorded duration of zero
    And 2 additional matching canonical spans have invalid or missing durations
    When I request call and latency statistics
    Then the recorded call count should be 1003
    And the valid duration sample count should be 1001
    And the excluded duration count should be 2
    And p50, p95 and p99 should each be 100 milliseconds within the fixture tolerance
    And no missing duration should be silently replaced by zero
    And the zero-duration span should remain included in the valid sample count

  @M2-LAT-006
  Scenario: Measure the selected span's elapsed duration rather than summed child work
    Given 1000 selected SERVER spans each have duration 200 milliseconds
    And each has overlapping dependency spans of 120 and 180 milliseconds
    When I request latency percentiles for the SERVER population
    Then p50, p95 and p99 should each be 200 milliseconds within the fixture tolerance
    And the summed dependency duration of 300 milliseconds should not replace request duration
