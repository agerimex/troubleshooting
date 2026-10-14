# language: en
@milestone_2 @report
Feature: Share the comparison and its selected investigation evidence
  As an engineer reviewing a release with a colleague
  I want a reproducible report of observed changes and supporting examples
  So that the colleague can understand the evidence and its limitations

  Background:
    Given the comparison follows "metrics-v1", "errors-v1" and "comparison-v1"
    And embedded combined reports follow the Milestone 1 evidence and export contracts

  @M2-REP-001 @core
  Scenario Outline: Export the reviewed comparison with selected combined reports
    Given I have reviewed a baseline and candidate comparison
    And I have explicitly included 2 generated combined trace-and-log reports as supporting examples
    When I export the reviewed comparison as "<format>"
    Then the artifact should preserve the displayed population definitions, counts, rates, percentiles and differences
    And it should include sample sizes, calculation methods, rule versions and evidence-quality notices
    And it should include analysis cutoffs or aggregate revisions and report generation times
    And it should identify the 2 embedded reports as selected examples rather than the whole population
    And their stored spans, logs, correlation and limitations should remain inspectable offline
    And any live-only evidence links should be distinguished from embedded evidence
    And recorded text in an HTML artifact should remain content rather than executable markup

    Examples:
      | format |
      | HTML   |
      | JSON   |

  @M2-REP-002
  Scenario: Keep the reviewed report stable when source data or rules change
    Given a reviewed comparison used 1000 candidate calls and rules "comparison-v1"
    And 10 more eligible calls arrive afterward
    And the installation's configured comparison rules are changed
    When I export the reviewed comparison
    Then it should still show 1000 candidate calls and rules "comparison-v1"
    And it should retain the exact reviewed percentile values rather than recomputing an approximation
    When I explicitly regenerate the comparison
    Then the new report should have its own revision and the current applicable rules
    And it should show 1010 candidate calls once those records are included in its analysis revision
    And the older report should remain identifiable as the earlier result

  @M2-REP-003
  Scenario: Preserve unavailable log statistics alongside available span statistics
    Given call counts and latency statistics are available
    And the error-log statistics source is unavailable
    When I generate and export the comparison
    Then the call counts and latency statistics should remain available
    And error-log statistics should be marked unavailable rather than zero
    And error-log conclusions should be unavailable
    And unaffected span metrics should retain their own coverage and eligibility assessment
    And the export should preserve the same limitation as the reviewed report
