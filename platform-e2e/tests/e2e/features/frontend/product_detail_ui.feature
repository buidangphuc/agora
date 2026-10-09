@buyer
Feature: Product detail page - anatomy, real data, gallery, purchase, reviews, Q&A and shop card
  ui-phase-product-detail, beyond product_detail.feature: the page anatomy and token discipline,
  real data only, the image gallery, out-of-stock handling, reviews, Q&A, the shop card and the
  tracking hooks. Listings, reviews and shops are seeded through the gateway; every assertion is
  on the rendered page or on what the browser sent.

  # ── Anatomy ───────────────────────────────────────────────────────────
  Scenario: Desktop layout shows the header card above the sections
    Given a published listing with stock 12 and a buyer on its page at 1280px
    Then the page shows the breadcrumb, the gallery beside the info column, the shop card, the anchor nav and the three sections in that vertical order
    And exactly one h1 contains the listing title

  Scenario: Specs render in a Descriptions block
    Given a published listing with stock 12 and a buyer on its page at 1280px
    Then the Chi tiết section lists "Kho hàng" "12 sản phẩm" and the category, followed by the description

  Scenario: No raw values or off-scale type
    When the token lint runs on the product detail files
    Then the token lint reports no violation in the product detail files

  # ── Real data only ────────────────────────────────────────────────────
  Scenario: A listing with no reviews shows no rating stars and no sold count
    Given a published listing with stock 12 and a buyer on its page at 1280px
    Then no star rating is rendered for the listing, "Chưa có đánh giá" is shown and no sold count appears in the header or the shop card

  Scenario: Mall is never guessed
    Given a published listing priced 6000000 whose title has a brand keyword and a buyer on its page
    Then no Mall badge is rendered

  Scenario: A flash-sale listing shows the real discount
    Given a listing with an active flash-sale campaign
    When a visitor views that listing
    Then the strike-through shows the regular price, the price shows the campaign price and the discount badge is computed from them

  # ── Gallery ───────────────────────────────────────────────────────────
  Scenario: Clicking a thumbnail changes the stage without shifting layout
    Given a published listing with 4 images and a buyer on its page
    When the buyer clicks the third thumbnail
    Then the stage shows the third image and its bounding box is unchanged

  Scenario: A missing image uses the fallback
    Given a published listing with stock 12 and a buyer on its page at 1280px
    Then the stage shows the placeholder in a 1:1 box and no external image host was requested

  Scenario: Thumbnails are lazy and the stage is eager
    Given a published listing with 4 images and a buyer on its page
    Then the stage image is not lazy and each thumbnail image is lazy

  # ── Variants and purchase ─────────────────────────────────────────────
  Scenario: An out-of-stock variant cannot be chosen
    Given a published listing with variants and a buyer on its page
    Then the out-of-stock variant is disabled, tagged Hết hàng and cannot be selected by click or keyboard

  Scenario: Out of stock disables both actions
    Given a published listing with variants and a buyer on its page with the out-of-stock variant in the URL
    Then both purchase buttons and the quantity picker are disabled and a warning says the variant is out of stock

  Scenario: Add to cart failure is reported and recoverable
    Given a published listing with stock 12 and a buyer on its page at 1280px
    And the listing is deleted behind the page's back
    When the buyer clicks Thêm vào giỏ
    Then an error toast is shown, no add_to_cart event was fired and both purchase buttons are enabled again

  Scenario: Add to cart event is unchanged
    Given a published listing priced 100000 and a buyer on its page
    When the buyer adds quantity 2 to the cart
    Then one add_to_cart event carries value 200000, currency VND and the item id, name, price, quantity and category

  Scenario: One view event per PDP load
    Given a published listing with variants and a buyer on its page while recording beacons
    When the buyer selects the variant "256GB" and uses the anchor nav
    Then exactly one view_item event was sent and the variant change and the anchor navigation sent none

  Scenario: The buy bar is visible at 375px and tracks the variant
    Given a published listing with variants and a buyer on its page at 375px
    When the buyer scrolls to the reviews section and selects the variant "256GB"
    Then the buy bar is visible with the price "150.000"
    When the buyer clicks Thêm vào giỏ in the buy bar
    Then a toast "Đã thêm 1 sản phẩm vào giỏ hàng" appears and the cart counter shows 1

  # DEFECT (xfail in the binder): at 375px the global footer scrolls under the buy bar - the page
  # reserves bottom padding for the PDP body but not for the footer below it.
  Scenario: The bar covers no content
    Given a published listing with variants and a buyer on its page at 375px
    When the buyer scrolls to the bottom of the page
    Then the last element of the page is fully visible above the buy bar

  Scenario: The anchor nav is sticky on desktop only
    Given a published listing with stock 12 and a buyer on its page at 1280px
    Then the anchor nav is sticky
    When the viewport becomes 375 pixels wide
    Then the anchor nav is not sticky and scrolls horizontally without page overflow

  # ── Reviews ───────────────────────────────────────────────────────────
  # DEFECT (xfail in the binder): with 23 reviews the UI offers only 2 pages; ?rpage=3 clamps to page 2,
  # so reviews 21-23 are unreachable (only the first 20 are fetched).
  Scenario: Reviews paginate at 10
    Given a published listing with 23 reviews and a buyer on its page
    When the buyer opens the third page of reviews
    Then 3 reviews are listed and the pagination marks page 3 as current

  Scenario: Marking helpful is single-use and reports failure
    Given a published listing with one review by another buyer and a buyer on its page
    When the buyer marks that review as helpful
    Then the helpful count increases by one and the button is disabled

  Scenario: A successful mutation revalidates the page
    Given a published listing with one review by another buyer and a buyer on its page
    When the buyer marks that review as helpful
    Then the new count comes from the server without a reload and survives a reload

  Scenario: Submitting a review shows pending then success
    Given a published listing with stock 12 and a buyer on its page at 1280px
    And the form submission is slowed down
    When the buyer picks 5 stars, types a comment and submits the review modal
    Then the submit button is pending and the modal controls are disabled
    And a success toast shows, the modal closes and the new review is listed

  # ── Q&A ───────────────────────────────────────────────────────────────
  Scenario: Asking a question shows pending and a toast
    Given a published listing with stock 12 and a buyer on its page at 1280px
    And the form submission is slowed down
    When the buyer types a question and submits
    Then the submit button is pending, then a success toast appears, the textarea clears and the question is listed

  Scenario: Empty submit is blocked
    Given a published listing with stock 12 and a buyer on its page at 1280px
    Then the ask button is disabled while the textarea is empty and no request is made

  Scenario: A guest is prompted to log in
    Given a published listing with stock 12 and a guest on its page
    Then the login prompt links to the login page with the return URL and no ask form renders

  Scenario: A failed action returns a structured error
    Given a published listing with stock 12 and a buyer on its page at 1280px
    And the buyer's session token is corrupted behind the page's back
    When the buyer types a question and submits
    Then an error toast and an inline alert show a readable message and the page does not crash

  # ── Shop card ─────────────────────────────────────────────────────────
  Scenario: The shop card shows the real shop name
    Given a seller named "Cửa hàng Hoa Mai" with a published listing and a buyer on its page
    Then the PDP shop card shows "Cửa hàng Hoa Mai" and not "Shop #"
    When the buyer opens the shop
    Then the storefront header shows "Cửa hàng Hoa Mai"

  Scenario: An empty shop name falls back
    Given a seller without a shop name with a published listing and a buyer on its page
    Then the PDP shop card shows "Shop #" and the first 6 characters of the seller id

  Scenario: The PDP shop card links to the storefront
    Given a seller named "Tiệm Hoa Hồng" with a published listing and a buyer on its page
    When the buyer opens the shop
    Then the browser is on the seller's storefront and its header is the hero shop card

  Scenario: Storefront sort is in the URL
    Given a seller named "Tiem Hoa Mai" with two priced listings and a buyer on the shop page
    When the buyer sorts the shop by "Giá: Thấp đến Cao"
    Then the URL contains sort=price_asc and the cheaper listing comes first
    And reloading the shop keeps the ascending price order

  Scenario: A shop with no ratings shows an empty rating
    Given a seller named "Shop Chua Co Danh Gia" with a published listing and a buyer on its page
    Then the shop card shows "Chưa có đánh giá" instead of "0.0 / 5.0"

  # ── Not-found, loading, tracking ──────────────────────────────────────
  Scenario: An unknown listing renders Result 404
    When the visitor requests the unknown listing "does-not-exist"
    Then the response status is 404 and the Result offers "Về trang chủ" and "Tìm sản phẩm khác"

  Scenario: The loading skeleton matches the final layout
    Given a published listing with stock 12 and a buyer on its page at 1280px
    When the buyer loads the listing over a throttled network
    Then the measured layout shift of the page is 0

  Scenario: Similar-item impression and click keep attribution
    Given a published listing with stock 12 and a buyer on its page while recording beacons
    When the similar-items row scrolls into view and the buyer clicks the second card
    Then similar_items impressions for positions 1..n and a similar_items click for position 2 were sent
