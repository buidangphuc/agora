@buyer
Feature: Invite link, referral cookie, register attribution and xu rewards (frontend-referral-invite)
  A buyer shares an invite link from /account/referral. /s/<shortCode> keeps utm_*, a referral link
  sets the agora_ref cookie and lands on /register?ref=, register sends the code from the field or
  the cookie, and the referral page shows pending and released xu with reward statuses.
  Attribution is observed through the inviter's invited_count and the reward status through the
  gateway reads the page itself renders.

  Scenario: S1 Buyer copies the invite link
    Given a signed-in inviter with a referral code and clipboard access
    When the buyer opens the referral page and taps "Chia sẻ link mời"
    Then the toast "Đã sao chép link mời." is shown
    And the clipboard holds the site origin followed by "/s/" and a short code
    And that short code resolves to a referral link for the buyer's code with utm_source "referral" and utm_medium "invite"

  Scenario: S4 Guest is sent to login (permission)
    When an anonymous visitor opens the referral page
    Then the visitor is on the login page with returnUrl "/account/referral"
    And no referral code, invite button or xu total is rendered

  Scenario: S5 Referral link redirects to register with attribution
    Given an inviter with a referral code
    And the inviter has a referral share link with utm_source "referral" and utm_medium "invite"
    When the short link is requested without following redirects
    Then the answer is 307 to the register page with ref the inviter's code and utm_medium "invite" and utm_source "referral" in that order
    And the agora_ref cookie holds the inviter's code with HttpOnly, SameSite=Lax, Path=/ and Max-Age=604800 and no Secure attribute
    When the short link is requested without following redirects behind an https proxy
    Then the agora_ref cookie also carries the Secure attribute

  Scenario: S6 Listing and shop links keep utm and set no cookie
    Given a seller with a published listing
    And a listing share link with utm_source "share"
    And a shop share link with utm_source "share"
    And a seller share link with utm_source "share"
    When each short link is requested without following redirects
    Then the listing link answers 307 to the listing page with utm_source "share"
    And the shop link and the seller link answer 307 to the shop page with utm_source "share"
    And none of the three responses sets agora_ref

  Scenario: S7 Unknown short code (error)
    When a short link that does not exist is requested without following redirects
    Then the answer is 307 to "/" with no query
    And the response sets no agora_ref cookie

  Scenario: S8 Unknown target type (error)
    Given a share link of target type "coupon" with utm_source "share"
    When the short link is requested without following redirects
    Then the answer is 307 to "/" with no query
    And the response sets no agora_ref cookie

  Scenario: S9a Non-utm key dropped (boundary)
    Given a seller with a published listing
    And a listing share link carrying utm_source "share" plus the key "campaign" valued "x"
    When the short link is requested without following redirects
    Then the answer is 307 to the listing page with utm_source "share" only

  Scenario: S9b Unsafe referral id (error, permission)
    Given a referral share link whose target id is "bad code!"
    When the short link is requested without following redirects
    Then the answer is 307 to "/" with no query
    And the response sets no agora_ref cookie

  Scenario: S10 Register with ?ref
    Given an inviter with a referral code
    When a visitor registers on the register page opened with the inviter's code as ref
    Then the visitor lands on the home page
    And the inviter's invited count rises by 1

  Scenario: S11 Register with only the cookie, then cookie cleared
    Given an inviter with a referral code
    And the inviter has a referral share link with utm_source "referral" and utm_medium "invite"
    When a visitor follows the short link in the browser and then registers on the plain register page
    Then the visitor lands on the home page
    And the inviter's invited count rises by 1
    And the register response deleted agora_ref and the browser holds no agora_ref cookie

  Scenario: S12 Field wins over cookie (boundary)
    Given an inviter with a referral code
    And a second inviter with a referral code
    And the browser holds the agora_ref cookie of the second inviter
    When a visitor registers on the register page opened with the first inviter's code as ref
    Then the visitor lands on the home page
    And the first inviter's invited count rises by 1
    And the second inviter's invited count stays 0

  Scenario Outline: S13 Malformed code is ignored (error)
    Given an inviter with a referral code
    When a visitor registers on the register page opened with the malformed ref "<ref>"
    Then the visitor lands on the home page
    And the new account can sign in through the gateway
    And the inviter's invited count stays 0

    Examples:
      | ref                                   |
      | bad code!                             |
      | ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefg     |

  Scenario: S14 Failed register keeps the cookie (error)
    Given an inviter with a referral code
    And an existing account
    And the browser holds the agora_ref cookie of the inviter
    When a visitor submits the register form with the existing username
    Then the form shows "Tên đăng nhập đã tồn tại."
    And the browser still holds the agora_ref cookie of the inviter
    And the inviter's invited count stays 0

  Scenario: S15 No code, no field
    Given an inviter with a referral code
    When a visitor registers on the plain register page recording Set-Cookie headers
    Then the visitor lands on the home page
    And no response set an agora_ref cookie
    And the inviter's invited count stays 0

  Scenario: S16 returnUrl still honoured
    Given an inviter with a referral code
    When a visitor registers on the register page opened with returnUrl "/cart" and the inviter's code as ref
    Then the visitor lands on "/cart"
    And the inviter's invited count rises by 1

  Scenario: S17 Totals in xu
    Given an inviter whose invitee has a completed order with a pending reward
    When the inviter opens the referral page
    Then "Xu đang chờ" shows the pending xu from the gateway formatted with dots and the suffix " xu"
    And "Xu đã nhận" shows the released xu from the gateway formatted with dots and the suffix " xu"
    And the page contains no đ currency amount

  Scenario: S18 Reward timeline statuses
    Given an inviter with two released rewards, one pending reward held by the daily cap and one voided reward
    When the inviter opens the referral page
    Then the timeline shows "Đang chờ" once, "Đã nhận" once per released reward and no "Đã hủy"
    And the pending reward shows "Dự kiến nhận:" with its release date as dd/MM/yyyy in Asia/Ho_Chi_Minh
    And the listed rows are exactly the released and pending rewards, none struck through
    And "Xu đang chờ" counts only the pending reward and "Xu đã nhận" only the released rewards

  Scenario: S19 No rewards (boundary)
    Given a signed-in inviter with a referral code and clipboard access
    When the buyer opens the referral page
    Then "0 xu" is shown twice
    And the text "Chưa có phần thưởng nào." is shown

  Scenario: S21 Totals are the caller's own (permission)
    Given an inviter whose invitee has a completed order with a pending reward
    And another signed-in buyer with their own referral code
    When the other buyer opens the referral page
    Then the page shows the other buyer's own code and not the inviter's code
    And "Xu đang chờ" and "Xu đã nhận" both show "0 xu"
    And no reward is listed and "Chưa có phần thưởng nào." is shown

  Scenario: S22 Rule text shown
    Given a signed-in inviter with a referral code and clipboard access
    When the buyer opens the referral page
    Then the text "Chỉ nhập được trong 7 ngày đầu và khi bạn chưa có đơn hoàn thành." is shown under the redeem field

  Scenario Outline: S23 Redeem refused (error)
    Given a signed-in buyer on a referral page with <situation>
    When the buyer submits <code_kind> in the redeem field
    Then an error toast is shown and the redeem field is invalid with the same Vietnamese message as help
    And no referral was attributed for that attempt

    Examples:
      | situation               | code_kind             |
      | a code already redeemed | another friend's code |
      | no earlier redeem       | their own code        |

  Scenario: S24 Redeem accepted
    Given an inviter with a referral code
    And a signed-in buyer on a referral page with no earlier redeem
    When the buyer redeems the inviter's code on the referral page
    Then the redeem success toast "Đã ghi nhận mã giới thiệu." is shown and the redeem field is empty
    And the inviter's invited count rises by 1
