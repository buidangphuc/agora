@tracking @analytics
Feature: The warehouse stores the loop event types
  The tracking consumer stores EventType 15-21 under their lowercase names, keeps their
  properties, listing id and principal, applies the consent rules, and stores a replayed
  envelope once (analytics-loop-event-storage). Envelopes are produced straight onto analytics.events.

  Scenario: Loop storage: a produced event of each new type reaches the warehouse
    When one tracking event of each new loop type, each with a unique session id, is produced to analytics.events
    Then the warehouse holds exactly one row per produced event whose event_type is the matching lowercase name

  Scenario: Loop storage: properties and principal survive the warehouse
    When an ai_interaction event with properties surface=assistant and action=accepted and listing id "l-1" is produced by a principal
    Then the warehouse row carries exactly those properties, that listing id and the principal id

  Scenario: Loop storage: no analytics consent blanks the client id
    When a sign_up event carrying a ga_client_id and analytics consent false is produced to analytics.events
    Then the warehouse row for it has an empty ga_client_id

  Scenario: Loop storage: a replayed event is stored once
    When the same login envelope is produced to analytics.events twice
    Then the warehouse holds exactly one row for that event id after a 10 second settle
