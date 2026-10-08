@tracking @analytics
Feature: A re-sent tracking event is stored once
  The gateway derives the envelope event_id from the visitor and the client eventId, and the
  warehouse sink keeps at most one row per event_id, so a re-sent beacon is not counted twice and
  one visitor cannot suppress another's event (tracking-ingest-integrity).

  Scenario: Posting the same event twice stores one row
    When a visitor posts the same view (same eventId and anonymousId, unique listing id) twice
    Then both envelopes on analytics.events carry the same event_id
    And the warehouse holds exactly one row for that listing id

  Scenario: Two visitors cannot collide on an event id
    When two different anonymous visitors post views with the same eventId
    Then the warehouse holds one row for each visitor
