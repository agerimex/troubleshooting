# language: en
@milestone_1 @reference_example
Feature: Reproduce the combined investigation on a supported installation
  As another engineer evaluating Troubleshooting
  I want to run a documented example with a supported application sender
  So that I can verify ingestion, investigation and export using known trace and log evidence

  @M1-REF-001 @core
  Scenario: Complete a reference investigation that requires both evidence types
    Given a clean installation with the documented prerequisites
    And a supported application sender configured to deliver spans and correlated logs
    When I follow the documented setup and load the reference recording failure
    And the sender confirms that the reference records have been stored
    And I find the reference ERROR log from service "recording-worker" containing "AccessDenied" within the reference investigation window
    And I open the investigation report from that log without entering a TraceId
    Then the spans should show the request and its failed upload step
    And a related log should show the diagnostic detail "Storage returned 403 AccessDenied"
    And that diagnostic detail should not be present in the fixture's span attributes or span events
    And selecting the upload span should expose its associated warning and error logs
    And I should be able to return to the full trace context and export the same report

  @M1-REF-002
  Scenario: Retain both traces and logs through container recreation
    Given the reference spans and correlated log records have been stored
    And they remain within their configured retention periods
    When the storage container is recreated while its persistent data volume is retained
    And the installation becomes ready
    And I regenerate the reference report without sending the fixture again
    Then the previously stored spans and log records should still be available
    And their identities and associations should be unchanged

  @M1-REF-003
  Scenario: Make a failed log delivery visible to the supported sender
    Given the configured log storage cannot accept a reference log record
    When the supported sender sends and flushes that log record
    Then it should receive a failure or an explicit unconfirmed-delivery result
    And it should not receive a false confirmation that the record was stored
    When storage recovers and the sender retries the same record identity
    And the sender confirms that it was stored
    Then the related investigation report should contain that logical log record exactly once
