@promo
Feature: Sellers list and stop their own flash-sale campaigns and shop vouchers (promotion-seller-self-service)
  A seller sees only the campaigns and shop vouchers of their own shop, cancels a SCHEDULED
  campaign and deactivates a shop voucher without being an admin, and none of it is audited.
  Scenario names equal the spec scenario titles of openspec/changes/promotion-seller-self-service.
  Every refusal is paired with a positive control so it cannot pass vacuously.

  Scenario: Seller sees only their own campaigns with status
    Given the self-service sellers A and B, each with a published listing
    And seller A has one scheduled and one active campaign and seller B has one campaign
    When seller A lists their own campaigns
    Then the list holds exactly A's scheduled and active campaigns with seller id A, their statuses and stock figures
    And the list holds none of seller B's campaigns

  Scenario: Status filter narrows the list
    Given the self-service sellers A and B, each with a published listing
    And seller A has one scheduled and one active campaign and seller B has one campaign
    When seller A lists their own campaigns filtered by "CAMPAIGN_STATUS_SCHEDULED"
    Then the list holds only A's scheduled campaign
    And an unfiltered list of seller A still holds both campaigns

  Scenario: Caller without listing.write is denied
    Given the self-service sellers A and B, each with a published listing
    And seller A has one scheduled and one active campaign and seller B has one campaign
    And a self-service buyer
    When the buyer lists their own campaigns and cancels a campaign of seller A
    Then the buyer's campaign calls are refused with "permission_denied" and carry no campaign data
    And seller A can still list their two campaigns

  Scenario: Seller cancels their SCHEDULED campaign
    Given the self-service sellers A and B, each with a published listing
    And seller A has a scheduled campaign for the self-service cancel
    When seller A cancels that campaign
    Then the cancel response shows status "CAMPAIGN_STATUS_CANCELLED" and a cancelled_at
    And seller A's list shows the campaign "CAMPAIGN_STATUS_CANCELLED"
    And the listing of the campaign has no active flash sale

  Scenario: Cancelling an ACTIVE campaign is refused
    Given the self-service sellers A and B, each with a published listing
    And seller A has an active campaign for the self-service cancel
    When seller A cancels that campaign
    Then the self-service call is refused with "failed_precondition"
    And the campaign stays "CAMPAIGN_STATUS_ACTIVE" with no cancelled_at
    And seller A can still cancel a scheduled campaign

  Scenario: Cancelling another seller's campaign is denied like an unknown id
    Given the self-service sellers A and B, each with a published listing
    And seller A has a scheduled campaign for the self-service cancel
    When seller B cancels that campaign and then a campaign id that does not exist
    Then both calls are refused with "permission_denied" and the same message
    And the campaign of seller A keeps status "CAMPAIGN_STATUS_SCHEDULED"
    And seller A can still cancel the campaign

  @destructive
  Scenario: Seller cancellation writes no audit event
    Given the self-service sellers A and B, each with a published listing
    And the seeded admin for the self-service checks
    And seller A has a scheduled campaign for the self-service cancel
    And team-audit is stopped for the self-service checks
    When seller A cancels that campaign
    Then the cancel response shows status "CAMPAIGN_STATUS_CANCELLED" and a cancelled_at
    And no "campaign.cancel" audit event exists for the campaign after team-audit is started again
    And an admin cancel of another scheduled campaign is audited as a positive control

  Scenario: Seller sees own vouchers of every state
    Given the self-service sellers A and B, each with a published listing
    And the seeded admin for the self-service checks
    And seller A has an active, a deactivated and an expired shop voucher
    And seller B has a shop voucher and a platform voucher exists
    When seller A lists their shop vouchers
    Then the voucher list holds exactly A's three vouchers with quota and used set
    And the voucher list holds neither seller B's voucher nor the platform voucher
    And the three vouchers show their active, deactivated and expired state

  Scenario: Buyer without listing.write is denied
    Given the self-service sellers A and B, each with a published listing
    And the seeded admin for the self-service checks
    And seller A has an active, a deactivated and an expired shop voucher
    And a self-service buyer
    When the buyer lists shop vouchers and deactivates the active voucher of seller A
    Then the buyer's voucher calls are refused with "permission_denied" and carry no voucher data
    And the active voucher of seller A is still active

  Scenario: Seller deactivates their voucher
    Given the self-service sellers A and B, each with a published listing
    And the seeded admin for the self-service checks
    And seller A has an active, a deactivated and an expired shop voucher
    And a self-service buyer
    And the buyer can preview the active voucher of seller A
    When seller A deactivates their active shop voucher
    Then the voucher response shows active false and a deactivated_at
    And the buyer previewing the voucher gets valid false

  Scenario: Deactivating another seller's voucher is denied like an unknown id
    Given the self-service sellers A and B, each with a published listing
    And the seeded admin for the self-service checks
    And seller A has an active, a deactivated and an expired shop voucher
    When seller B deactivates the active voucher of seller A and then a voucher id that does not exist
    Then both calls are refused with "permission_denied" and the same message
    And the active voucher of seller A is still active
    And seller A can still deactivate their active voucher

  @destructive
  Scenario: Seller deactivation writes no audit event
    Given the self-service sellers A and B, each with a published listing
    And the seeded admin for the self-service checks
    And seller A has an active, a deactivated and an expired shop voucher
    And team-audit is stopped for the self-service checks
    When seller A deactivates their active shop voucher
    Then the voucher response shows active false and a deactivated_at
    And no "voucher.deactivate" audit event exists for the voucher after team-audit is started again
    And an admin deactivation of another shop voucher is audited as a positive control

  Scenario: Seller reaches the seller RPCs but not the admin ones
    Given the self-service sellers A and B, each with a published listing
    And seller A has one scheduled and one active campaign and seller B has one campaign
    When seller A lists their own campaigns and then calls the admin campaign list
    Then the own-campaign list succeeds
    And the admin campaign list is refused with "permission_denied"

  Scenario: Admin cancel keeps its admin rules
    Given the self-service sellers A and B, each with a published listing
    And the seeded admin for the self-service checks
    And seller A has a scheduled campaign for the self-service cancel
    When the admin cancels that campaign through the admin RPC
    Then the cancel response shows status "CAMPAIGN_STATUS_CANCELLED" and a cancelled_at
    And exactly one "campaign.cancel" audit event exists for the campaign by the admin

  Scenario: Admin deactivate keeps its admin rules
    Given the self-service sellers A and B, each with a published listing
    And the seeded admin for the self-service checks
    And seller A has an active, a deactivated and an expired shop voucher
    When the admin deactivates the active voucher of seller A through the admin RPC
    Then the voucher response shows active false and a deactivated_at
    And exactly one "voucher.deactivate" audit event exists for the active voucher of seller A
