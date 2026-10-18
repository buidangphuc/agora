@admin @auth
Feature: User enforcement
  An admin restricts, suspends or bans a user through AccountAdminService. The change is
  audited first, revokes the user's sessions, ends their access within a minute, and is
  enforced again at login and refresh. Scenario names echo the spec scenarios of
  openspec/changes/archive/2026-10-18-identity-user-enforcement. Every refusal is paired with a positive
  control so it cannot pass vacuously.

  Scenario: User enforcement: Ban writes one audit event and revokes sessions
    Given the seeded admin and a buyer signed in on 3 devices
    When the admin bans the buyer with reason code "scam"
    Then the ban succeeds and reports as many sessions revoked as the buyer had
    And the admin sees the buyer as "banned" with reason "scam"
    And team-audit holds exactly 1 user.set_status event for the buyer by the admin with status "banned"

  @destructive
  Scenario: User enforcement: Audit down refuses the change
    Given the seeded admin and a buyer signed in on 2 devices
    And team-audit is down for this scenario
    When the admin suspends the buyer for 1 hour with reason code "scam"
    Then the call is refused as "unavailable"
    And the buyer still logs in with the correct password
    When team-audit is started again
    Then team-audit holds no user.set_status event for the buyer
    And the admin sees the buyer as "active" with reason ""
    And suspending the buyer for 1 hour now succeeds

  Scenario: User enforcement: Non-admin cannot set a status
    Given the seeded admin and a buyer signed in on 1 devices
    And a signed-in seller
    When the seller calls SetUserStatus on the buyer
    And an anonymous caller calls SetUserStatus on the buyer
    Then the seller call is refused as "permission_denied"
    And the anonymous call is refused as "unauthenticated"
    And team-audit holds no user.set_status event for the buyer
    And the admin sees the buyer as "active" with reason ""
    And the admin can set the buyer to "restricted"

  Scenario: User enforcement: Admin cannot change their own status
    Given the seeded admin and a buyer signed in on 1 devices
    When the admin bans the admin's own account with reason code "scam"
    Then the call is refused as "failed_precondition"
    And team-audit holds no user.set_status event for the admin
    And the admin can still log in
    And the admin can set the buyer to "restricted"

  Scenario: User enforcement: Admin cannot change another admin
    Given the seeded admin and a buyer signed in on 1 devices
    And a second admin
    When the admin bans the second admin with reason code "scam"
    Then the call is refused as "failed_precondition"
    And team-audit holds no user.set_status event for the second admin
    And the second admin can still log in
    And the admin can set the buyer to "restricted"

  Scenario: Login enforcement: Banned user is refused after a correct password
    Given the seeded admin and a buyer signed in on 1 devices
    And another active buyer
    When the admin bans the buyer with reason code "scam"
    And the buyer logs in with the correct password
    Then the login is refused as "permission_denied" with the generic locked message
    And the other active buyer logs in with the correct password successfully

  Scenario: Login enforcement: Wrong password reveals no status
    Given the seeded admin and a buyer signed in on 1 devices
    And an active buyer, a suspended buyer and a banned buyer
    When each of the three logs in with a wrong password
    Then all three refusals have the identical code and message and none says the account is locked
    And the suspended buyer is refused with the correct password
    And the active buyer logs in with the correct password successfully

  Scenario: Login enforcement: Expired suspension is lifted
    Given the seeded admin and a buyer signed in on 1 devices
    And the status events on identity.events are being tapped
    When the admin suspends the buyer until 4 seconds from now with reason code "scam"
    Then the buyer is refused at login while the suspension runs
    And within 30 seconds a login with the correct password succeeds
    And the admin sees the buyer as "active" with reason ""
    And exactly 1 UserStatusChanged event has reason code "suspension_expired" from "suspended" to "active"
    And team-audit holds exactly 1 user.set_status event for the buyer by the admin with status "suspended"

  Scenario: Login enforcement: Restricted seller token lacks listing.write
    Given the seeded admin and a signed-in seller
    Then the seller's token carries listing.write
    When the admin restricts the seller with reason code "scam"
    And the seller logs in with the correct password
    Then the seller's token lacks listing.write but keeps listing.read, engagement:write and search:write

  Scenario: User enforcement: Refresh tokens of revoked sessions are dead
    Given the seeded admin and a buyer signed in on 1 devices
    And another active buyer
    When the admin bans the buyer with reason code "scam"
    Then the buyer's earlier refresh token is refused as "unauthenticated"
    And the other active buyer's refresh token still works

  Scenario: User enforcement edge: Anonymous caller is rejected at the edge
    Given the seeded admin and a buyer signed in on 1 devices
    When an anonymous caller calls ListUsers and SetUserStatus
    Then both calls are refused as "unauthenticated"
    And the admin can list users

  Scenario: User enforcement edge: Seller without admin scope is denied
    Given the seeded admin and a signed-in seller
    When the seller calls ListUsers and SetUserStatus
    Then both calls are refused as "permission_denied"
    And the admin can list users

  Scenario: User enforcement edge: Banned user's token is rejected within a minute
    Given the seeded admin and a buyer signed in on 1 devices
    And another active buyer
    Then the buyer's access token is accepted
    When the admin bans the buyer with reason code "scam"
    Then within 60 seconds the buyer's old access token is rejected as unauthenticated
    And the other active buyer's access token is still accepted

  Scenario: User enforcement edge: Banned user's open stream ends
    Given the seeded admin and a buyer signed in on 1 devices
    And another active buyer
    And both buyers hold an open notification stream
    When the admin bans the buyer with reason code "scam"
    Then within 60 seconds the banned buyer's stream ends
    And the other active buyer's stream is still open
