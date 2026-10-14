# language: en
@milestone_2 @retention
Feature: Retain trustworthy statistical history beyond raw evidence retention
  As an engineer comparing releases over time
  I want explicit retention, coverage and aggregation freshness
  So that historical trends remain useful without pretending expired details still exist

  Background:
    Given the retention and aggregation contract from METRIC_CONTRACT.md
    And the fixture uses 1-hour UTC buckets, 48-hour raw retention and 30-day statistical retention
    And the fixture's allowed lateness is 24 hours after a bucket ends
    And tests use a controllable clock and bounded background-work completion checks

  @M2-RET-001 @core
  Scenario: Keep finalized statistics after traces and logs have expired
    Given canonical reference spans and logs have produced finalized call and error-group statistics
    And 4 days have passed since their bucket ended
    And their raw spans and logs have been physically removed by retention
    When I open the historical statistics
    Then counts and error-group occurrences should remain unchanged
    And retained percentiles should agree with the earlier results within the documented tolerance
    And the retained population dimensions, source kinds and calculation versions should remain available
    When I request an expired supporting example
    Then the interface should explain that its raw evidence is outside retention
    And it should not invent a trace, log message or stack trace from aggregate data

  @M2-RET-002
  Scenario: Apply explicit retention changes to an existing installation
    Given an existing installation uses the fixture retention policy
    When I apply raw trace retention of 7 days, raw log retention of 3 days and statistical retention of 90 days
    And the supported configuration and migration procedure completes
    Then the effective policy should show all three requested retention periods
    And the existing data stores should use that policy without requiring a fresh installation
    And reapplying the same configuration should not create duplicate statistical data
    And previously physically deleted raw evidence should not be described as restored
    And physical cleanup should be described as asynchronous rather than guaranteed at an exact second

  @M2-RET-003
  Scenario: Distinguish expired statistical history from an observed zero
    Given a requested window is older than the statistical retention period
    And its raw evidence and statistical buckets have expired
    When I request statistics for that window
    Then the result should be unavailable because it is outside retained coverage
    And it should not return a confirmed call count of zero
    And it should not classify the absent measurements as an improvement

  @M2-RET-004
  Scenario: Backfill already recorded evidence without double counting
    Given canonical spans and logs were stored before statistical aggregation was enabled
    And only part of the requested historical range still has complete raw coverage
    When I run the supported backfill for the retained range
    And I repeat the same backfill
    Then every eligible canonical record should contribute exactly once
    And the retained range should reconcile with direct calculation from those raw records
    And the unrecoverable range should remain explicitly unavailable
    And rerunning the job should replace or reuse the same logical results rather than add them again

  @M2-RET-005
  Scenario: Include a late record once while its bucket is still provisional
    Given a provisional bucket contains statistics for 100 canonical calls
    And complete raw evidence remains available for rebuilding that bucket
    When a new matching call arrives before the allowed-lateness deadline
    And the bucket is refreshed
    Then its call count should be 101
    And its status counts and duration distribution should include the new call once
    And its revision and provisional status should be visible
    When that same call is replayed and the bucket is refreshed
    Then its call count should remain 101

  @M2-RET-006
  Scenario: Protect finalized history from old replay and unsupported late arrivals
    Given a bucket has been finalized and its raw evidence has expired
    And the installation restarts while retaining its statistical data and finalization metadata
    When a replayed call and a previously unseen call arrive for that closed bucket
    Then the finalized statistical counts and distributions should remain unchanged
    And the too-late records should have an explicit statistical-exclusion reason
    And the relevant coverage notice should identify that late evidence was excluded
    And no promise of indefinite replay deduplication should be made without retained identity evidence
    And raw ingestion outcome should remain distinct from eligibility for historical statistics

  @M2-RET-007
  Scenario: Combine historical and recent coverage without counting an interval twice
    Given a complete historical hour contains statistics for 100 calls
    And a complete recent hour contains 50 canonical calls and overlapping provisional statistics
    When I request statistics over exactly those two hours
    Then the call count should be 150
    And each hour should contribute through one selected representation
    And the duration distributions should be combined from the contributing calls or mergeable states
    And the existence of both raw records and statistical states should not inflate the result

  @M2-RET-008
  Scenario: Preserve a previous revision when refreshing a bucket fails
    Given a provisional bucket has a published revision with 100 calls
    And a refreshed revision would contain 101 calls
    When the refresh fails before the replacement revision is committed
    Then the previous revision should remain internally consistent
    And the failed refresh and stale revision should be visible
    And no result should mix counts from one revision with percentiles from the other
    When the supported retry succeeds
    Then the new revision should contain 101 calls without retaining additive copies of the older result

  @M2-RET-009
  Scenario: Avoid inventing exact sub-hour statistics after raw evidence expires
    Given only complete 1-hour historical buckets remain
    When I request statistics from "T+10m" to "T+70m"
    Then the interface should explain that exact coverage of those bounds is unavailable
    And it should offer available whole-bucket bounds as an explicit alternative
    And it should not silently widen the window or prorate counts and percentiles
    When I explicitly select the offered whole-bucket window
    Then the result should display those actual bounds
