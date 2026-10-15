@buyer
Feature: Review integrity in the browser (frontend-review-integrity)
  Reviews come only from a completed order: the PDP has no write button, only a hint to the
  orders page; legacy unverified reviews are labelled and not counted; an admin can hide a
  review with a reason. Orders are completed through the gateway, legacy rows are inserted by
  the engagement fixture, and every assertion is on the rendered page or on stored reviews.

  Scenario: PDP offers no write button and links to the orders page
    Given a listing with one verified review and a signed-in buyer on its page
    Then the reviews section has no write-review button and shows the hint linking to the orders page
    When the buyer follows the orders hint
    Then the browser is on the orders page
    When a guest follows the orders hint on the same listing
    Then the guest is on the login page and no review dialog is open

  Scenario: Reviewing a completed order item shows a verified review
    Given a listing and a signed-in buyer with a completed order for it
    When the buyer reviews the order item with 5 stars and the comment "Giao nhanh, đúng mô tả"
    Then a success toast "Đánh giá sản phẩm thành công!" shows and the review modal closes
    And the listing page shows that review with the verified purchase badge and no write-review button

  Scenario: A second review for the same order item is refused
    Given a listing and a signed-in buyer with a completed order for it
    And the buyer already reviewed that order item through the API
    When the buyer reviews the order item with 4 stars and the comment "Đánh giá lần hai"
    Then the review modal stays open showing "Bạn đã đánh giá sản phẩm này cho đơn hàng này."
    And only one review by the buyer is stored for the listing

  Scenario: Legacy unverified review is listed but not counted
    Given a listing with verified reviews of 5 and 3 stars and one legacy unverified review of 1 star
    When a guest opens that listing page
    Then the summary shows "4.0" and "từ 2 đánh giá đã mua hàng"
    And the list shows 3 reviews and exactly one carries "Chưa xác minh mua hàng"

  Scenario: Admin hides a review and it disappears for everyone
    Given a listing with verified reviews of 5 and 3 stars and one legacy unverified review of 1 star
    And the seeded admin is signed in on that listing page
    Then every review shows a hide control
    When the admin hides the review "Review 3 sao" with the reason "Spam quảng cáo"
    Then the dialog closes, the toast "Đã ẩn đánh giá." shows and the review is gone without a reload
    And the hidden review is no longer returned by the reviews API
    When a guest opens that listing page
    Then the hidden review is absent for the guest and the other reviews remain

  Scenario: Buyers and guests do not see the hide control
    Given a listing with one verified review and a signed-in buyer on its page
    Then no hide control is in the page
    When a guest opens that listing page
    Then no hide control is in the page
    When the seeded admin opens that listing page
    Then every review shows a hide control
