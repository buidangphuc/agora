@tracking @analytics
Feature: The warehouse records when it ingested each tracking row
  The sink stamps ingested_at with its own clock at write time, while occurred_at stays the time
  the edge received the event (tracking-ingest-integrity).

  Scenario: A stored event has an ingest time after its occurrence
    When a visitor posts one view with a unique listing id and the sink writes it
    Then the warehouse row has a non-null ingested_at that is not earlier than its occurred_at
