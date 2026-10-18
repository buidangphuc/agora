@seller
Feature: Seller sees the moderation status of their listings (frontend-listing-moderation)
  A flagged publish waits in review, a clean one sells, a rejected one returns to review after an
  edit, and the seller never reads the screen flags. Scenario names echo the spec scenarios of
  openspec/changes/frontend-listing-moderation. Each flagged case is paired with a clean listing of
  the same seller so the status tags cannot pass vacuously.

  Scenario: Moderation UI: Seller sees the pending status after publishing a flagged listing
    Given a moderation seller who published a clean and a flagged listing
    When the seller opens the seller center
    Then the seller list shows the flagged listing with the tag "Chờ duyệt"
    And the seller list shows the clean listing with the tag "Đang bán"
    When the seller opens the edit page of the flagged listing
    Then the edit page shows "Tin đăng đang chờ kiểm duyệt."

  Scenario: Moderation UI: Seller sees a clean listing as selling
    Given a moderation seller who published a clean and a flagged listing
    When the seller opens the seller center
    Then the seller list shows the clean listing with the tag "Đang bán"
    And the flagged listing has no tag "Đang bán" in the seller list
    And an anonymous visitor can open the clean listing page

  Scenario: Moderation UI: Seller edits a rejected listing and it returns to review
    Given a moderation seller who published a clean and a flagged listing
    And an admin has rejected the flagged listing with reason "counterfeit" through the gateway
    When the seller opens the seller center
    Then the seller list shows the flagged listing with the tag "Bị từ chối"
    When the seller saves an edit of the flagged listing in the browser
    Then the edit form shows "Tin đăng đã được gửi và đang chờ kiểm duyệt."
    When the seller opens the seller center
    Then the seller list shows the flagged listing with the tag "Chờ duyệt"
    And an anonymous reader gets not_found for the flagged listing

  Scenario: Moderation UI: The seller never sees the screen flags
    Given a moderation seller who published a clean and a flagged listing
    When the seller opens the edit page of the flagged listing
    Then the edit page shows "Tin đăng đang chờ kiểm duyệt."
    And the edit page text contains no screen flag
    And an admin reads a screen flag on the flagged listing through the gateway
