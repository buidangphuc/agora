Feature: Unpublished listings are private to their seller
  Drafts must not leak through listing reads or search. Probed through the gateway
  with two real users (a seller who owns the draft, a buyer who does not).

  Scenario: A stranger cannot read a draft
    Given a seller with a draft listing
    And a logged-in buyer
    When the buyer reads that draft through GetListing
    Then the gateway answers HTTP 404

  Scenario: The owner can read their draft
    Given a seller with a draft listing
    When the seller reads that draft through GetListing
    Then the gateway answers HTTP 200
    And the returned listing has status draft

  Scenario: Listing without a status returns only published listings
    Given a seller with a draft listing
    And no one is logged in
    When the client lists listings without a status filter
    Then the gateway answers HTTP 200
    And every returned listing has status published
    And the seller's draft is not among them

  @search
  Scenario: Search hides drafts by default
    Given a seller with a draft and a published listing sharing a unique title token
    And no one is logged in
    When the published listing is indexed in search
    And the client searches for the unique title token
    Then the search returns the published listing
    And the search returns no hit for the draft
    And the owner can find the draft by asking for their own drafts

  Scenario: A user cannot search another seller's drafts
    Given a seller with a draft listing
    And a logged-in buyer
    When the buyer searches with status draft and the seller's seller_id
    Then the gateway answers HTTP 403

  Scenario: Saved searches require a signed-in user
    Given no one is logged in
    When the client calls SaveSearch
    Then the gateway answers HTTP 401
