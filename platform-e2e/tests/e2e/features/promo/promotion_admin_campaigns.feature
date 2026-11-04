@promo
Feature: Admin lists, cancels and edits flash-sale campaigns, and creates them safely (promotion-admin-campaigns)
  An admin sees every campaign with its derived status, cancels a campaign without losing sold or
  held units, edits a scheduled one, and creates campaigns with server-side validation and an
  idempotency key. Scenario names equal the spec scenario titles of
  openspec/changes/promotion-admin-campaigns. Every refusal is paired with a positive control
  (the same admin succeeds on a valid call) so it cannot pass vacuously.

  Scenario: List filters by status
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And one scheduled, one active, one ended and one cancelled campaign exist
    When the admin lists campaigns filtered by "CAMPAIGN_STATUS_SCHEDULED"
    Then the list holds the scheduled campaign with status "CAMPAIGN_STATUS_SCHEDULED"
    And the list holds none of the active, ended and cancelled campaigns
    And an unfiltered list shows the four campaigns with their derived statuses

  @destructive
  Scenario: Kill-switch off is reported
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And an active campaign on the listing
    And the active campaign list shows it with the kill-switch on
    And the flash-sale kill-switch is switched off
    When the admin lists campaigns filtered by "CAMPAIGN_STATUS_ACTIVE"
    And anyone lists the active campaigns
    Then the admin list still holds the campaign and reports flash_sale_disabled
    And the active campaign list is empty and reports flash_sale_disabled

  Scenario: Non-admin cannot list all campaigns
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And an active campaign on the listing
    And the admin list holds the campaign
    When the seller lists all campaigns
    And an anonymous caller lists all campaigns
    Then the seller is refused with "permission_denied" and no campaign is returned
    And the anonymous caller is refused with "unauthenticated" and no campaign is returned

  Scenario: Cancel stops new holds, keeps existing holds and sold
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And an active campaign on the listing with stock cap 10
    And 2 units of the campaign are sold and 1 unit is held
    When the admin cancels the campaign successfully
    And a buyer tries to reserve 1 unit of the campaign
    And the held unit is committed
    Then the new reserve is refused with reason "not_active"
    And the commit of the held unit succeeds
    And the campaign shows status "CAMPAIGN_STATUS_CANCELLED" and stock_sold 3

  @destructive
  Scenario: Cancel while audit is down
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And an active campaign on the listing
    And team-audit is stopped for the campaign admin checks
    When the admin cancels the campaign
    Then the call is refused with "unavailable"
    And once team-audit is back the campaign is still "CAMPAIGN_STATUS_ACTIVE"
    And the admin can then cancel the campaign

  Scenario: Anonymous cannot cancel
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And an active campaign on the listing
    When an anonymous caller cancels the campaign
    Then the call is refused with "unauthenticated"
    And the campaign is still "CAMPAIGN_STATUS_ACTIVE"
    And the admin can then cancel the campaign

  Scenario: Edit a scheduled campaign
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And a scheduled campaign on the listing
    When the admin updates the campaign to sale price 90000, stock cap 50 and a later window
    Then the update response shows those values with status "CAMPAIGN_STATUS_SCHEDULED"
    And a following list shows those values with status "CAMPAIGN_STATUS_SCHEDULED"
    And exactly one "campaign.update" audit event exists for the campaign

  Scenario: Edit a started campaign is refused
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And one scheduled, one active, one ended and one cancelled campaign exist
    When the admin updates the active, the ended and the cancelled campaign
    Then each update is refused with "failed_precondition"
    And the stored values of those three campaigns are unchanged
    And the admin can still update the scheduled campaign

  Scenario: Anonymous cannot edit
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And a scheduled campaign on the listing
    When an anonymous caller updates the campaign
    Then the call is refused with "unauthenticated"
    And the stored values of the campaign are unchanged
    And the admin can still update the scheduled campaign

  Scenario: Price not below listing price
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And an admin create with a valid payload succeeds on another listing
    When the admin creates a campaign with sale price equal to the listing price
    Then the call is rejected with "invalid_argument" on field "sale_price"
    And no campaign exists for the listing and no create audit entry was written
    And the admin can create a campaign with the sale price one below the listing price

  Scenario: Unknown listing
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And an admin create with a valid payload succeeds on another listing
    When the admin creates a campaign for a listing id that does not exist
    Then the call is rejected with "invalid_argument" on field "listing_id"

  @destructive
  Scenario: Listing service down
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And an admin create with a valid payload succeeds on another listing
    And team-listing is stopped for the campaign admin checks
    When the admin creates a campaign on the listing and the time is measured
    Then the call fails with "unavailable" and reason "LISTING_UNAVAILABLE" within 2.5 seconds
    And once team-listing is back no campaign exists for the listing

  @destructive
  Scenario: Create with audit down
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And team-audit is stopped for the campaign admin checks
    When the admin creates a campaign on the listing with a fresh key
    Then the call fails with "unavailable" and reason "AUDIT_UNAVAILABLE" in domain "promotion.platform"
    And once team-audit is back no campaign exists for the listing
    And retrying the create with the same key succeeds

  Scenario: Retry after timeout returns same campaign
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    When the admin creates a campaign on the listing with a fresh key twice with the same payload
    Then both calls return the same campaign id
    And the list holds exactly one campaign for the listing

  Scenario: Same key different payload
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And an admin create on the listing succeeded with a fresh key
    When the admin reuses the key with a different sale price
    Then the call is refused with "already_exists"
    And the original campaign is unchanged and is the only one for the listing

  Scenario: Replay after cancel returns current state
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And an admin create on the listing succeeded with a fresh key
    And the admin has cancelled the created campaign
    When the admin replays the original create request with the same key and payload
    Then the replay returns the same campaign id with status "CAMPAIGN_STATUS_CANCELLED"
    And the list holds exactly one campaign for the listing

  @admin
  Scenario: Filter and cancel from the list
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And an active campaign on the listing
    And an admin is signed in the admin app for the campaign checks
    And the admin has opened the campaign list screen
    When the admin picks the filter "active" and confirms cancel on the campaign row
    Then the row shows status cancelled
    And the active filter no longer lists the campaign
    And the stored campaign is "CAMPAIGN_STATUS_CANCELLED"

  @admin
  Scenario: Create retry reuses the key
    Given the seeded admin for the campaign admin checks
    And a seller with a published listing for the campaign admin checks
    And an admin is signed in the admin app for the campaign checks
    And the admin has the campaign create form filled for the listing
    And the first create request loses its response
    When the admin submits the form and then submits it again
    Then both create requests carry the same idempotency_key
    And the list holds exactly one campaign for the listing
