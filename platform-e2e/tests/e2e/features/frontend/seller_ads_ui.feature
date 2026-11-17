@seller @promo @ads
Feature: The seller ads page lists and manages campaigns from the service (seller-ads-ui)
  OpenSpec change seller-ads-ui. Campaigns, buyer clicks and pauses go through the real gateway
  (SponsoredService); the browser is signed in as the seller and reads /seller/ads. No route is
  fulfilled. Scenario names echo the spec scenarios.

  Scenario: Campaigns persist after reload
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 1000 and budget 50000 on that listing
    When the seller opens the ads page in the seller app
    And the seller reloads the ads page
    Then the ads table shows exactly 1 campaign row
    And the campaign row shows "₫1.000" bid, "₫50.000" budget, "₫0" spent, "₫50.000" remaining, 0 impressions, 0 clicks and "Đang chạy"

  Scenario: Spend and counters come from the service
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 1000 and budget 50000 on that listing
    And a buyer is served a sponsored slot for that listing in that category
    When the buyer records a click for the served token
    Then the click is charged and names the listing
    When the seller opens the ads page in the seller app
    Then the campaign row shows "₫1.000" spent, "₫49.000" remaining, 1 click and at least 1 impression

  Scenario: Pause an active campaign
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 1000 and budget 50000 on that listing
    And a buyer is served a sponsored slot for that listing in that category
    When the seller opens the ads page in the seller app
    And the seller activates "Tạm dừng" on the campaign row
    Then the campaign row shows the status "Tạm dừng" and the button "Tiếp tục"
    When the seller reloads the ads page
    Then the campaign row shows the status "Tạm dừng" and the button "Tiếp tục"
    When a buyer lists the sponsored slots for placement "search" in that category with limit 4
    Then the slots do not contain that listing

  Scenario: Resume a paused campaign
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 1000 and budget 50000 on that listing
    And the seller pauses the campaign
    When the seller opens the ads page in the seller app
    Then the campaign row shows the status "Tạm dừng" and the button "Tiếp tục"
    When the seller activates "Tiếp tục" on the campaign row
    Then the campaign row shows the status "Đang chạy" and the button "Tạm dừng"
    When the seller reloads the ads page
    Then the campaign row shows the status "Đang chạy" and the button "Tạm dừng"
    And the seller's listed campaign reads status "AD_CAMPAIGN_STATUS_ACTIVE" and spent 0

  Scenario: Another seller's campaign is not found
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 1000 and budget 50000 on that listing
    And a second seller
    When the second seller pauses the first seller's campaign
    Then the campaign call fails with code "not_found"
    When the second seller resumes the first seller's campaign
    Then the campaign call fails with code "not_found"
    And the seller's listed campaign reads status "AD_CAMPAIGN_STATUS_ACTIVE" and spent 0
    When the second seller opens the ads page in the seller app
    Then the ads table shows "Chưa có chiến dịch nào" and no campaign row

  Scenario: Bid of exactly 500 is accepted
    Given an ads seller with a published listing in a fresh category
    When the seller opens the ads page in the seller app
    And the seller submits the campaign form with bid 500 and budget 5000
    Then the ads table shows exactly 1 campaign row
    And the campaign row shows "₫500" bid, "₫5.000" budget, "₫0" spent, "₫5.000" remaining, 0 impressions, 0 clicks and "Đang chạy"

  Scenario: Guest and non-seller are not authorized to open the page
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 1000 and budget 50000 on that listing
    When a visitor without a session opens the ads page in the seller app
    Then the visitor is redirected to login with "/seller/ads" as the return path
    When a signed-in buyer opens the ads page in the seller app
    Then the page shows no campaign row and no "₫50.000"
