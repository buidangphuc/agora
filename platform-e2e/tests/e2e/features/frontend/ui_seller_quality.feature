@seller
Feature: Seller pages - layout shift, images, primary action, tokens and tracking
  OpenSpec change ui-phase-seller. Server markup is read over HTTP with the seller's session;
  layout shift comes from the browser's layout-shift entries; the funnel is read through the
  gateway as the seller.

  Scenario: Skeleton matches the page footprint
    Given a d2 seller has 3 published listings
    When the d2 seller opens the workplace from the wallet page while the data is slow
    Then a skeleton with the KPI row and table rows shows first and the page then replaces it without a layout shift

  Scenario: Thumbnails reserve their space
    Given a d2 seller has 5 published listings with thumbnails
    When the d2 seller opens the workplace with the thumbnails loading slowly
    Then each thumbnail box is already 1:1 and the row height does not change when the picture fails or loads

  Scenario: Below-the-fold images are lazy
    Given a d2 seller has 20 published listings with thumbnails
    Then the thumbnails after the first screen are lazy

  Scenario: Token lint passes on seller code
    Then the token lint reports no violation under "src/app/(shop)/seller,src/app/(shop)/sell,src/features/seller"

  Scenario: One primary CTA per page
    Given a d2 seller has 1 published listings
    When the d2 seller opens "/seller/orders"
    Then at most one brand-filled primary button is visible in the page header and the other actions are outline or ghost

  Scenario: Impression, click and attribution events still fire
    Given a d2 seller has a listing and a buyer views it in the browser
    When the buyer opens the listing page and adds the product to the cart
    Then the page emitted view and add to cart events for that listing and the seller funnel counts the view
