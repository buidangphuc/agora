@auth @handoff
Feature: One-time hand-off code through the gateway
  As a user signed in on one app
  I want a single-use code that opens my session on the other app
  So that I do not have to sign in twice and no refresh token is shared

  Scenario: Authenticated user mints a code for the seller app
    Given a signed-in buyer
    When the buyer mints a code for the seller app
    Then the mint succeeds with a 43-character base64url code
    And the code expires 60 seconds after the gateway clock
    And the hand-off table holds one row keyed by the SHA-256 of the code and no row contains the plaintext

  Scenario: Mint writes its audit entry and the row commits together
    Given a signed-in buyer
    When the buyer mints a code for the seller app
    Then team-audit holds one "auth.handoff_mint" entry for the buyer
    And that entry names the seller app and carries no code, hash or sid
    And the hand-off table holds one row for the buyer

  Scenario: Unspecified target is refused
    Given a signed-in buyer
    When the buyer mints a code with an unspecified target
    Then the call fails with "invalid_argument"
    And the hand-off table holds no row for the buyer
    And team-audit holds no "auth.handoff_mint" entry for the buyer
    And a mint for the seller app by the same buyer then succeeds

  Scenario: Anonymous caller cannot mint
    Given a signed-in buyer
    When an anonymous caller mints a code for the seller app
    Then the call fails with "unauthenticated"
    And the hand-off table holds no row for the buyer
    And a mint for the seller app by the same buyer then succeeds

  Scenario: Non-ACTIVE account cannot mint
    Given a signed-in buyer
    And a mint for the seller app by the same buyer then succeeds
    When an admin suspends the buyer
    And the buyer mints a code for the seller app with the still-valid access token
    Then the call fails with "permission_denied"
    And the hand-off table holds exactly 1 row for the buyer

  @destructive
  Scenario: Audit down blocks the mint
    Given a signed-in buyer
    And team-audit is stopped
    When the buyer mints a code for the seller app
    Then the call fails with "unavailable"
    And the hand-off table holds no row for the buyer
    When team-audit is started again
    Then a mint for the seller app by the same buyer then succeeds

  Scenario: Eleventh live code is refused
    Given a signed-in buyer
    And the buyer has minted 10 live codes
    When the buyer mints a code for the seller app
    Then the call fails with "resource_exhausted"
    And the hand-off table holds exactly 10 rows for the buyer
    And team-audit holds exactly 10 "auth.handoff_mint" entries for the buyer

  @slow
  Scenario: Expired and redeemed codes do not count toward the cap
    Given a signed-in buyer
    And the buyer has minted 5 codes that are left to expire
    And the buyer has minted and redeemed 5 codes
    And 61 seconds have passed since the first mint
    And the buyer has minted 9 live codes
    When the buyer mints a code for the seller app
    Then the mint succeeds with a 43-character base64url code
    And the buyer has exactly 10 live codes
    And a mint for the seller app by the same buyer then fails with "resource_exhausted"

  Scenario: Concurrent mints cannot exceed the cap
    Given a signed-in buyer
    When the buyer mints 20 codes at once
    Then exactly 10 mints succeed and 10 fail with "resource_exhausted"
    And the buyer has exactly 10 live codes
    And team-audit holds exactly 10 "auth.handoff_mint" entries for the buyer

  Scenario: Peek returns the username and does not consume
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the code is peeked 3 times
    Then every peek returns the buyer's username and an expiry
    And the code then redeems successfully

  Scenario: Peek writes nothing
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    And the buyer's audit entry count is noted
    When the code is peeked 1 times
    Then the peek returns the buyer's username
    And the code is still unredeemed in the table
    And the buyer's audit entry count is unchanged

  Scenario: Peek of an invalid code is indistinguishable
    Given a signed-in buyer
    And the invalid codes: unknown, used, wrong-target, malformed and revoked-source
    And a fresh seller-target code of the buyer
    When each invalid code is peeked
    Then every response is 401 "unauthenticated" with one identical message
    And the valid code is peeked successfully as a control

  Scenario: Peek needs no token
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When an anonymous caller peeks the code
    Then the peek returns the buyer's username

  Scenario: Peek needs no token at the gateway
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When an anonymous caller peeks the code
    Then the peek returns the buyer's username
    And the same peek with a garbage bearer token is also answered

  Scenario: Valid code opens a new session for the target app
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the code is redeemed for the seller app
    Then the redeem returns an access token and a refresh token
    And the new session's sid differs from the source sid
    And the source session still refreshes normally

  Scenario: Derived session survives source logout
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    And the code is redeemed for the seller app
    When the source session is logged out
    Then the derived session still refreshes successfully

  Scenario: Reused code is refused
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    And the code is redeemed for the seller app
    When the code is redeemed for the seller app again
    Then the last call is 401 "unauthenticated"
    And the buyer's active sessions changed by 1

  Scenario: Concurrent redeems create one session
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the code is redeemed 20 times at once
    Then exactly 1 redeem succeeds and 19 fail with "unauthenticated"
    And the buyer's active sessions changed by 1
    And team-audit holds exactly 1 "auth.handoff_redeem" entry for the buyer
    And the 19 losers each answered within 150 ms

  @slow
  Scenario: Expired code is refused
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    And 61 seconds have passed since the first mint
    When the code is redeemed for the seller app
    Then the last call is 401 "unauthenticated"
    And the buyer's active sessions changed by 0

  Scenario: Wrong target is refused
    Given a signed-in buyer
    And a fresh buyer-target code of the buyer
    When the code is redeemed for the seller app
    Then the last call is 401 "unauthenticated"
    And the code stays unredeemed in the table
    And the code redeems for the buyer app as a control

  Scenario: Revoked source session is refused
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    And the source session is logged out
    When the code is redeemed for the seller app
    Then the last call is 401 "unauthenticated"
    And the buyer's active sessions changed by -1

  Scenario: Account suspended after minting
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    And an admin suspends the buyer
    When the code is redeemed for the seller app
    Then the last call is 401 "unauthenticated"
    And another buyer's fresh code still redeems as a control

  @slow
  Scenario: Every failure is indistinguishable
    Given a signed-in buyer
    And the invalid codes: unknown, used, expired, wrong-target, malformed and revoked-source
    And a fresh seller-target code of the buyer
    When each invalid code is redeemed
    Then every response is 401 "unauthenticated" with one identical message
    And the valid code redeems successfully as a control

  @destructive
  Scenario: Audit down blocks the redeem and keeps the code usable (accepted oracle)
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    And team-audit is stopped
    When the code is redeemed for the seller app
    Then the call fails with "unavailable"
    And the buyer's active sessions changed by 0
    When team-audit is started again
    Then the code redeems successfully within its TTL

  Scenario: Redeem audit entry
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the code is redeemed for the seller app
    Then team-audit holds one "auth.handoff_redeem" entry for the buyer
    And that entry names the seller app with result "ok" and carries no code, hash or sid

  Scenario: Code absent from logs even on failure
    Given a signed-in buyer
    And log capture is started for identity and gateway
    And a fresh seller-target code of the buyer
    And the code is redeemed for the seller app
    When the code is redeemed for the seller app again
    Then the last call is 401 "unauthenticated"
    And the captured logs contain neither the code, its hash, the user id nor the sid

  Scenario: Redeem needs no token
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When an anonymous caller redeems the code through the gateway
    Then the redeem returns an access token and a refresh token

  Scenario: Mint without a token is refused
    Given a signed-in buyer
    When an anonymous caller mints a code for the seller app
    Then the call fails with "unauthenticated"
    And the hand-off table holds no row for the buyer
    And a mint for the seller app by the same buyer then succeeds

  # TTL override stack: compose/handoff-ttl.override.yaml (ENV=test, HANDOFF_TTL_OVERRIDE=5s on team-identity).
  @handoff_ttl_override
  Scenario: Override is the TTL of minted codes in the running app
    Given a signed-in buyer
    When the buyer mints a code for the seller app
    Then the mint succeeds with a 43-character base64url code
    And the code expires 5 seconds after the gateway clock under the TTL override
    And once the override TTL has passed the code is refused with "unauthenticated"
