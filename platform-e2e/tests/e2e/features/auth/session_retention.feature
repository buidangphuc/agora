@auth @buyer
Feature: Session and login-history retention
  As the platform operator
  I want revoked sessions and old login history purged after a retention window
  So that device, IP and user-agent data is not kept forever

  # Short-retention stack: compose/session-retention.override.yaml
  # (SESSION_RETENTION=5s, LOGIN_HISTORY_RETENTION=5s, SESSION_SWEEP_INTERVAL=2s).
  @slow @retention_override
  Scenario: Session retention: revoked session disappears end to end
    Given a buyer who is signed in on two devices with a refresh token on the active one
    And ListSessions shows both sessions
    When the buyer revokes the other device's session from the active device
    And 12 seconds pass
    Then ListSessions returns only the active session
    And the revoked session row is gone from identity_db
    And the active session still accepts RefreshSession

  @slow @retention_override
  Scenario: Session retention: old login history disappears end to end
    Given a buyer who logs in once and sees a login history entry
    When 12 seconds pass
    Then ListLoginHistory no longer contains that login entry

  # Default stack (no override): control for the two scenarios above.
  @slow @retention_default
  Scenario: Session retention: default stack keeps rows
    Given the stack runs without the short-retention override
    And a buyer who is signed in on two devices with a refresh token on the active one
    When the buyer revokes the other device's session from the active device
    And 12 seconds pass
    Then ListSessions still returns the revoked session
    And the revoked session row in identity_db carries a revoked_at timestamp
