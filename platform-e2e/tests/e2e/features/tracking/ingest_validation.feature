@tracking
Feature: Each tracking event in a batch is validated on its own
  POST /api/track drops an invalid event (unknown type, or a field out of bounds) and still produces
  the valid ones. It answers 202 with the accepted and dropped counts; only a body with no valid
  event is refused (tracking-ingest-integrity).

  Scenario: One bad event does not drop the batch
    When a visitor posts a batch of three events, two valid views carrying unique markers and one of type "teleport"
    Then the response is 202 with accepted 2 and dropped 1
    And both valid markers reach analytics.events

  Scenario: An oversized properties map drops only that event
    When a visitor posts a batch with one valid view and one view whose properties has 21 keys
    Then the response is 202 with accepted 1 and dropped 1
    And only the valid view reaches analytics.events
