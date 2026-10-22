@admin @engagement @order
Feature: Dispute escalation and the admin Khiếu nại tab in the browser (frontend-dispute-cases)
  A buyer escalates a rejected return from the order detail and an admin investigates and rules
  from /admin/cases?tab=disputes. Orders, returns and disputes are seeded through the gateway by
  real, distinct users; every outcome is read off the rendered page and, where the spec promises
  state, off the gateway. Refusals are paired with a positive control.

  Scenario: Buyer escalates a rejected return
    Given a buyer with a paid online order whose return the seller rejected
    When the buyer opens the returns tab of the order
    Then the return card offers the escalate button
    When the buyer escalates the return with the reason "Người bán từ chối vô lý"
    Then the order detail shows a dispute "Đang chờ xử lý" and no escalate button
    And the gateway lists exactly one OPEN dispute for the order linked to the return

  Scenario: Admin rules for the buyer and the return is refunded
    Given an OPEN dispute on that rejected return
    And the dispute admin is signed in the browser
    When the admin opens the dispute detail by deep link
    Then the dispute detail shows "Đang chờ xử lý"
    When the admin clicks "Bắt đầu điều tra" on the dispute detail
    Then the browser is on the dispute detail at "tab=disputes&dispute=<id>"
    And the dispute detail shows "Đang điều tra"
    And the dispute detail offers "Giải quyết"
    When the admin picks the ruling "Người mua thắng"
    And the admin clicks "Giải quyết" on the dispute detail
    Then the dispute detail shows the buyer-favour confirmation
    When the admin clicks "Xác nhận" on the dispute detail
    Then the dispute detail shows "Đã giải quyết"
    When the dispute buyer is signed in the browser
    And the buyer reloads the order after the refund settles
    Then the returns tab shows the return as "Đã hoàn tiền"
    And the order detail dispute shows the ruling "Người mua thắng"

  Scenario: Seller-favour ruling leaves the return unchanged
    Given an INVESTIGATING dispute on that rejected return
    And the dispute admin is signed in the browser
    When the admin opens the dispute detail by deep link
    Then the dispute detail shows "Đang điều tra"
    When the admin picks the ruling "Người bán thắng"
    And the admin clicks "Giải quyết" on the dispute detail
    Then the dispute detail shows "Đã giải quyết"
    When the dispute buyer is signed in the browser
    And the buyer opens the returns tab of the order
    Then the order detail dispute shows "Đã giải quyết"
    And the order detail dispute shows the ruling "Người bán thắng"
    And the returns tab shows the return as "Đã từ chối"
    And the return still reads REJECTED at the gateway after a quiet period

  Scenario: An open dispute cannot be resolved directly
    Given an OPEN dispute on that rejected return
    And the dispute admin is signed in the browser
    When the admin opens the dispute detail by deep link
    Then the dispute detail offers "Bắt đầu điều tra"
    And the dispute detail offers "Từ chối"
    And the dispute detail offers no "Giải quyết" button
    And the dispute reads OPEN at the gateway

  Scenario: A buyer cannot open the dispute tab
    Given an OPEN dispute on that rejected return
    And the dispute buyer is signed in the browser
    When the buyer opens the dispute tab in the browser
    Then the page shows the admin-required result and no dispute list
    When the dispute admin is signed in the browser
    And the admin opens the dispute tab in the browser
    Then the dispute tab shows the dispute list heading
