@engagement
Feature: Engagement consumer logs a key fingerprint, never the raw key
  As an operator
  I want engagement consumer failure logs to carry a key fingerprint instead of user ids
  So that log readers cannot learn which user an event concerns

  Scenario: Engagement key redaction: handler failure logs fingerprint
    Given a poison record keyed by a seller id is published to listing.events
    When the consumer has retried and parked it
    Then the handler failure log carries key_fp equal to the first 12 hex of HMAC-SHA256 of the key under the log secret and the logs never contain the raw key

  Scenario: Engagement key redaction: DLQ parking logs fingerprint
    Given a poison record keyed by a seller id is published to listing.events
    When the consumer has retried and parked it
    Then the parking log carries key_fp, the dlq topic and offset, has no key attribute, the logs never contain the raw key and the DLQ record key is the raw key

  Scenario: Engagement key redaction: empty key
    Given a poison record with a nil key is published to listing.events
    When the consumer has retried and parked it
    Then the parking log carries the keyed fingerprint of the empty key, not the plain SHA-256 prefix, and no key attribute

  Scenario: Engagement key redaction: fingerprint is exactly 12 hex and stable
    Given two poison records with the same key and one with a different 200-byte key are published to listing.events
    When the consumer has retried and parked it
    Then the two same-key parking logs share one key_fp, the third differs, and every key_fp matches 12 lowercase hex
