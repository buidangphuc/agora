@engagement
Feature: team-engagement anonymises reviews and Q&A and deletes activity data on UserErased
  A UserErased event on identity.events anonymises the author of reviews, questions and answers, keeps
  their text and ratings, clears moderation fields that name the user and deletes personal activity
  data. The event is produced to the real topic; everything is read back through gateway RPCs, the
  broker, and engagement_db where no RPC exposes the field (hide_reason, hidden_by, reports, votes).
  (change content-user-erasure)

  Scenario: A review keeps rating and text under an anonymised author
    Given a seller "S" with a listing "L"
    And a buyer "U" with a verified 4-star review "Giao nhanh" on "L"
    When a UserErased event for "U" is consumed
    Then the review of "U" on "L" reads rating 4, comment "Giao nhanh" and author name "Người dùng đã xóa" without the id of "U"

  Scenario: A question and its answers keep their text under an anonymised author
    Given a seller "S" with a listing "L"
    And a buyer "U" who asked "Còn hàng không?" on "L"
    And a buyer "W" asked "Bảo hành bao lâu?" on "L" and "U" answered it with "Một năm"
    When a UserErased event for "U" is consumed
    Then the questions of "L" still list "Còn hàng không?", "Bảo hành bao lâu?" and "Một năm" and no row carries the id of "U"

  Scenario: The listing rating is unchanged
    Given a seller "S" with a listing "L"
    And a buyer "U" with a verified 4-star review "Giao nhanh" on "L"
    And a buyer "V" with a verified 5-star review "Rất tốt" on "L"
    And a buyer "W" with a verified 3-star review "Tạm được" on "L"
    And the rating summary of "L" is recorded
    When a UserErased event for "U" is consumed
    Then the review of "U" on "L" reads rating 4, comment "Giao nhanh" and author name "Người dùng đã xóa" without the id of "U"
    And the review count of "L" is still 3 and its average rating is unchanged

  Scenario: The moderation reason of an erased author is cleared
    Given a seller "S" with a listing "L"
    And a buyer "U" with a verified 4-star review "Giao nhanh" on "L"
    And the admin hides the review of "U" with the reason "Quảng cáo"
    When a UserErased event for "U" is consumed
    Then the hide reason of the review of "U" is empty, it is still hidden and hidden_at is still set

  Scenario: An erased moderator no longer appears as hidden_by
    Given a seller "S" with a listing "L"
    And a buyer "U" with a verified 4-star review "Giao nhanh" on "L"
    And a buyer "W" with a verified 3-star review "Tạm được" on "L"
    And the admin hides the review of "U" with the reason "Quảng cáo"
    And the admin hides the review of "W" with the reason "Quảng cáo"
    And the reviews of "U" and "W" record moderator "M" as hidden_by
    When a UserErased event for the moderator "M" is consumed
    Then hidden_by is empty on the reviews of "U" and "W", their hide reason reads "Quảng cáo" and both are still hidden

  Scenario: Other users' content is untouched
    Given a seller "S" with a listing "L"
    And a buyer "U" with a verified 4-star review "Giao nhanh" on "L"
    And a buyer "B" with a verified 5-star review "Rất tốt" on "L"
    And "B" asked "Còn hàng không?" on "L" and favourited "L"
    And the content of "B" is recorded
    When a UserErased event for "U" is consumed
    Then the review of "U" on "L" reads rating 4, comment "Giao nhanh" and author name "Người dùng đã xóa" without the id of "U"
    And the content of "B" is unchanged

  Scenario: Erasure is reachable only through the identity topic
    Given a seller "S" with a listing "L"
    And a buyer "U" with a verified 4-star review "Giao nhanh" on "L"
    And "U" favourited "L"
    And a buyer "X"
    When a UserErased record for "U" is produced to a topic other than identity.events
    And "X" calls engagement RPCs naming "U" and the review of "U"
    Then nothing of "U" is erased or changed

  Scenario: Favourites, follows and reports filed by the user are deleted
    Given a seller "S" with listings "L" and "L2"
    And a buyer "W" with a verified 5-star review "Rất tốt" on "L"
    And a buyer "U" with 2 favourites, 1 follow of "S", 1 report filed on "L" and 1 helpful vote on the review of "W"
    When a UserErased event for "U" is consumed
    Then zero favourites, follows, filed reports and helpful votes remain for "U"

  Scenario: An erased seller loses their followers
    Given a seller "S" with a listing "L"
    And a seller "S2" with a listing "L2"
    And buyers "F1", "F2" and "F3" who follow "S" and "S2"
    When a UserErased event for the seller "S" is consumed
    Then zero follows with seller "S" remain and each of "F1", "F2" and "F3" still follows only "S2"

  Scenario: Collections, history, check-ins and loyalty are deleted
    Given a seller "S" with 5 listings "V1" to "V5"
    And a buyer "U" with a collection of 2 items, 5 viewed listings and a check-in
    When a UserErased event for "U" is consumed
    Then "U" has no collections, no viewed listings and an empty loyalty account

  Scenario: Reports about the erased user's content stay open
    Given a seller "S" with a listing "L"
    And a buyer "U" with a verified 4-star review "Giao nhanh" on "L"
    And a buyer "W" who reported "L"
    When a UserErased event for "U" is consumed
    Then the review of "U" on "L" reads rating 4, comment "Giao nhanh" and author name "Người dùng đã xóa" without the id of "U"
    And the report of "W" on "L" still exists with status "REPORT_STATUS_OPEN"

  Scenario: A replayed event changes nothing
    Given a seller "S" with a listing "L"
    And a buyer "U" with a verified 4-star review "Giao nhanh" on "L"
    And "U" favourited "L"
    When a UserErased event for "U" is consumed
    Then the review of "U" on "L" reads rating 4, comment "Giao nhanh" and author name "Người dùng đã xóa" without the id of "U"
    When the same UserErased event for "U" and then one with a new event id are consumed
    Then the engagement erasure consumer completes both without error and "U" is unchanged

  Scenario: A malformed event goes to the DLQ
    Given a seller "S" with a listing "L"
    And a buyer "U" with a verified 4-star review "Giao nhanh" on "L"
    When a UserErased event with an empty user_id and a record that does not decode are produced
    Then both records are written to the engagement DLQ and nothing is applied to "U"

  Scenario: Erasure completes within one hour
    Given a seller "S" with a listing "L"
    And a buyer "U" holding 1000 reviews on "L"
    When a UserErased event for "U" is consumed
    Then all 1000 rows of "U" are anonymised within 1 hour
