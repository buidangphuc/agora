@seller @promo
Feature: Seller getting-started checklist on /seller (seller-getting-started, initiative seller-centre)
  The "Bắt đầu bán hàng" card reflects real shop state: each step turns done after the seller
  completes it on its linked page, and the card disappears once all four are done. Scenario names
  equal the spec scenario titles of openspec/changes/seller-getting-started. Accounts are fresh per
  scenario and data is seeded through the real gateway API.

  Scenario: Getting started: a new seller completes the steps in the browser
    Given a seller is signed in the seller app
    When the seller opens the seller centre
    Then the getting-started card shows exactly four steps, all not done, with links "/seller/shop", "/seller/shop", "/seller/new", "/seller/marketing"
    And no getting-started step mentions KYC
    When the seller saves the shop name "Shop Khởi Đầu" and a banner on the shop page
    And the seller opens the seller centre
    Then getting-started steps 1 and 2 are done and steps 3 and 4 are not done
    When the seller creates a listing on the new-listing page
    And the seller opens the seller centre
    Then getting-started steps 1, 2 and 3 are done and step 4 is not done
    When the seller creates a shop voucher on the marketing hub
    And the seller opens the seller centre
    Then the getting-started card is absent
    And the seller centre KPI row is present

  Scenario: Getting started: a seller with a flash sale only sees the marketing step done
    Given a seller is signed in the seller app
    And the seller owns a published listing with a scheduled flash sale and nothing else
    When the seller opens the seller centre
    Then the getting-started card is shown
    And getting-started steps 3 and 4 are done and steps 1 and 2 are not done

  Scenario: Getting started: a buyer without the seller role does not see the card
    Given a buyer is signed in the seller app
    When the seller opens the seller centre
    Then the onboarding button "Đồng ý và trở thành Người Bán" is shown
    And no getting-started card text is shown
