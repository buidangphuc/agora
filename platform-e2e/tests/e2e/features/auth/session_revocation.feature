@auth @buyer
Feature: Session revocation and trusted client context
  As a signed-in buyer
  I want revoking a session on another device to cut that device off at once
  And I want each session to show the device and IP it was created from
  So that I can trust the security page

  Scenario: Login returns a token bound to a listed session
    Given a registered buyer
    When the buyer logs in directly at the gateway
    Then the token's sid equals the id of a session ListSessions returns for the buyer

  # identity -> outbox -> Kafka identity.events -> gateway denylist; the gateway still
  # verifies each token locally (no per-request call to identity).
  Scenario: A revoked session's token stops working
    Given a buyer who is signed in on two devices
    When the buyer revokes the other device's session from the current device
    Then the other device's token is rejected with 401 within 5 seconds
    And the other device's token is also rejected on a public route
    And the current device's token still works

  @destructive
  Scenario: Revocations survive a gateway restart
    Given a buyer who is signed in on two devices
    When the buyer revokes the other device's session from the current device
    And the other device's token is rejected with 401 within 5 seconds
    And the gateway is restarted
    Then the other device's token is rejected with 401 within 20 seconds
    And the current device's token still works

  # The frontend calls the gateway server-side, so the gateway's peer is the Next.js
  # server; it forwards the browser's user agent and IP, which the gateway trusts only
  # from the frontend (TRUSTED_PROXIES).
  Scenario: A new session records device and IP
    Given a registered buyer
    When the buyer logs in through the login form
    And the buyer opens the account security page directly
    Then the newest session lists this browser's user agent and a non-empty IP

  Scenario: A client cannot spoof the forwarded IP
    Given a registered buyer
    When the buyer logs in directly at the gateway claiming to be 1.2.3.4
    Then the recorded session IP is not 1.2.3.4 and is not empty

  # The denylist entry lives until the revoked token's expiry. A SessionRevoked event with a
  # short expiry is produced to identity.events (the real topic the gateway consumes); the
  # gateway's own gauge (scraped by Prometheus) shows the entry added, then pruned.
  @slow
  Scenario: Expired denylist entries are dropped
    Given the gateway's denylist size is recorded
    When a SessionRevoked event expiring in 15 seconds is published to identity.events
    Then the gateway denylist grows by that entry
    And once the expiry has passed the gateway denylist no longer holds it
