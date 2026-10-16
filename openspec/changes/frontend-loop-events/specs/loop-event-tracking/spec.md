## ADDED Requirements

### Requirement: Tracking vocabulary covers the loop events

The analytics schema SHALL map each new GA4 name to its first-party beacon type: `sign_up`, `login`, `follow_seller`, `referral_redeem`, `daily_check_in`, `alert_subscribe` to themselves and `add_to_wishlist` to `favorite`. `GA4EventName` SHALL NOT contain the unused `favorite` name. The dataLayer SHALL accept the caller properties `method`, `content_type`, `seller_id`, `streak` and `alert_type` (scrubbed) in addition to `valid`, and SHALL still push nothing without analytics consent.

#### Scenario: Loop events: GA4 names map to beacon types
- **WHEN** `ga4ToInternal` is called for each of `sign_up`, `login`, `add_to_wishlist`, `follow_seller`, `referral_redeem`, `daily_check_in`, `alert_subscribe`, `share`
- **THEN** it returns `sign_up`, `login`, `favorite`, `follow_seller`, `referral_redeem`, `daily_check_in`, `alert_subscribe`, `share` respectively
- **AND** `internalToGa4("favorite")` returns `add_to_wishlist`
- **VERIFIED BY**: `team-frontend/src/lib/analytics/map.test.ts` `maps loop events`

#### Scenario: Loop events: dataLayer carries the allowlisted properties
- **WHEN** `trackEcommerce("sign_up", { properties: { method: "password", secret: "x" } })` runs with analytics consent granted
- **THEN** `window.dataLayer` gets one entry with `event: "sign_up"` and `properties` equal to `{ method: "password" }`, and the key `secret` is absent
- **VERIFIED BY**: `team-frontend/src/lib/analytics/dataLayer.test.ts` `passes loop properties and drops others`

#### Scenario: Loop events: dataLayer stays empty without consent
- **WHEN** `trackEcommerce("add_to_wishlist", { items: [{ itemId: "L1" }] })` runs without analytics consent
- **THEN** `window.dataLayer` contains no `add_to_wishlist` entry
- **AND** the beacon queue still receives one beacon of type `favorite` with `listingId` `L1`
- **VERIFIED BY**: `team-frontend/src/lib/analytics/dataLayer.privacy.test.ts` `loop events respect consent`

### Requirement: Sign-up and login are reported once, after success, via a one-shot cookie

On success `registerAction` SHALL set the cookie `agora_auth_evt=sign_up` and `loginAction` SHALL set `agora_auth_evt=login`, each non-HttpOnly, `SameSite=Lax`, `Path=/`, `Max-Age=60`, after the session cookie is set and before the redirect. No failure branch SHALL set it. A client component in the shop layout SHALL, on mount, delete the cookie and then, only if the value is exactly `sign_up` or `login`, call `trackEcommerce` with that name and `properties.method = "password"`. The cookie SHALL carry no user data.

#### Scenario: Loop events: registration sets the sign_up cookie only on success
- **WHEN** `registerAction` completes with the gateway returning a token
- **THEN** the cookie store receives `agora_auth_evt=sign_up` with `maxAge` 60, `sameSite` `lax`, `path` `/`, `httpOnly` false, after the session cookie, and the action redirects to `/`
- **VERIFIED BY**: `team-frontend/src/features/auth/actions.test.ts` `register success sets the sign_up event cookie`

#### Scenario: Loop events: login sets the login cookie only on success
- **WHEN** `loginAction` completes with the gateway returning a token
- **THEN** the cookie store receives `agora_auth_evt=login` with the same attributes and the action redirects to `/`
- **VERIFIED BY**: `team-frontend/src/features/auth/actions.test.ts` `login success sets the login event cookie`

#### Scenario: Loop events: failed auth attempts set no event cookie
- **WHEN** `loginAction` fails with empty fields, `Unauthenticated`, a service error or an empty token, or `registerAction` fails with invalid fields, `AlreadyExists`, any other error or an empty token
- **THEN** the cookie store never receives `agora_auth_evt` and no redirect happens
- **VERIFIED BY**: `team-frontend/src/features/auth/actions.test.ts` `failures never set the event cookie`

#### Scenario: Loop events: the auth beacon fires once and consumes the cookie
- **WHEN** `AuthEventBeacon` mounts with `agora_auth_evt=login` in `document.cookie`, including under React StrictMode double effects
- **THEN** `trackEcommerce` is called exactly once with `("login", { properties: { method: "password" } })` and the cookie is expired before that call
- **VERIFIED BY**: `team-frontend/src/features/auth/AuthEventBeacon.test.tsx` `fires once and deletes the cookie`

#### Scenario: Loop events: an unknown or absent auth cookie fires nothing
- **WHEN** `AuthEventBeacon` mounts with no cookie, or with `agora_auth_evt` equal to `admin`, an empty string, or a string longer than 4 000 characters injected via a `document.cookie` stub
- **THEN** `trackEcommerce` is not called, and an existing cookie of any value is expired
- **VERIFIED BY**: `team-frontend/src/features/auth/AuthEventBeacon.test.tsx` `ignores unknown values`

#### Scenario: Loop events: registering emits one sign_up event with the new principal
- **WHEN** a visitor submits a valid new username and password on `/register` and lands on `/`
- **THEN** within 10 s exactly one TrackingEvent `EVENT_TYPE_SIGN_UP` with `properties.method = "password"` is on `analytics.events`, and its envelope principal is the new user's id
- **AND** after analytics consent was granted the page `window.dataLayer` holds exactly one `sign_up` entry with `properties.method = "password"`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/loop_events.feature` `Loop events: registering emits one sign_up event with the new principal`

#### Scenario: Loop events: logging in emits one login event and a reload does not repeat it
- **WHEN** an existing buyer logs in on `/login`, lands on `/`, and then reloads the page twice
- **THEN** exactly one `EVENT_TYPE_LOGIN` event with `properties.method = "password"` and the buyer's principal is on `analytics.events` for that session, and the `agora_auth_evt` cookie is absent after the first landing
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/loop_events.feature` `Loop events: logging in emits one login event and a reload does not repeat it`

#### Scenario: Loop events: failed login and duplicate registration emit nothing
- **WHEN** a visitor submits a wrong password on `/login`, and another visitor registers a username that already exists
- **THEN** both pages show their error, no `agora_auth_evt` cookie exists, and zero `EVENT_TYPE_LOGIN` and zero `EVENT_TYPE_SIGN_UP` events for that session are on `analytics.events` after 10 s
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/loop_events.feature` `Loop events: failed login and duplicate registration emit nothing`

### Requirement: Favorite, share and follow events fire only after server success

`FavoriteButton` SHALL call `trackEcommerce("add_to_wishlist", { items: [{ itemId: id }] })` only after `addFavoriteAction` resolves, and nothing on removal or failure. `ShareButton` SHALL call `trackEcommerce("share", ...)` with `properties { method: "copy_link", content_type: "listing" }` and the listing as item after each successful link creation and after each later copy click. `FollowSellerButton` SHALL call `trackEcommerce("follow_seller", { properties: { seller_id } })` only when `followSellerAction` returns `ok`, and nothing on unfollow or failure. A guest SHALL NOT produce these events, because the server actions reject unauthenticated callers.

#### Scenario: Loop events: favorite tracked after add succeeds
- **WHEN** a signed-in buyer clicks the heart on listing `L1` and `addFavoriteAction` resolves
- **THEN** `trackEcommerce` is called exactly once with `("add_to_wishlist", { items: [{ itemId: "L1" }] })`
- **VERIFIED BY**: `team-frontend/src/features/engagement/FavoriteButton.test.tsx` `tracks add_to_wishlist after success`

#### Scenario: Loop events: favorite failure or removal tracks nothing
- **WHEN** `addFavoriteAction` rejects, so the heart rolls back, or the buyer un-favorites an already favorite listing
- **THEN** `trackEcommerce` is called 0 times and the heart returns to its previous state on failure
- **VERIFIED BY**: `team-frontend/src/features/engagement/FavoriteButton.test.tsx` `tracks nothing on failure or removal`

#### Scenario: Loop events: share tracked on create and on every re-copy
Note: creating the link counts as the first copy.
- **WHEN** a buyer clicks "Chia sẻ" and `createShareLinkAction` returns a short code, then clicks "Sao chép liên kết" twice
- **THEN** `trackEcommerce` is called 3 times, each with `("share", { items: [{ itemId: id }], properties: { method: "copy_link", content_type: "listing" } })`
- **VERIFIED BY**: `team-frontend/src/features/listing/ShareButton.test.tsx` `tracks create and each copy`

#### Scenario: Loop events: failed share link creation tracks nothing
- **WHEN** `createShareLinkAction` returns `ok: false` or `ok: true` without a `shortCode`
- **THEN** `trackEcommerce` is not called and the error toast is shown
- **VERIFIED BY**: `team-frontend/src/features/listing/ShareButton.test.tsx` `tracks nothing when creation fails`

#### Scenario: Loop events: follow tracked only on follow success
- **WHEN** a buyer follows shop `S1` and `followSellerAction` returns `ok: true`
- **THEN** `trackEcommerce` is called exactly once with `("follow_seller", { properties: { seller_id: "S1" } })`
- **VERIFIED BY**: `team-frontend/src/features/engagement/FollowSellerButton.test.tsx` `tracks follow_seller on success`

#### Scenario: Loop events: unfollow or failed follow tracks nothing
- **WHEN** the buyer unfollows, or `followSellerAction` returns `ok: false` and the button rolls back
- **THEN** `trackEcommerce` is called 0 times
- **VERIFIED BY**: `team-frontend/src/features/engagement/FollowSellerButton.test.tsx` `tracks nothing on unfollow or failure`

#### Scenario: Loop events: favoriting emits one favorite event and the add_to_wishlist dataLayer entry
- **WHEN** a signed-in buyer with analytics consent clicks the heart on a seeded listing
- **THEN** within 10 s exactly one TrackingEvent `EVENT_TYPE_FAVORITE` with that listing id is on `analytics.events`, and `window.dataLayer` holds exactly one `add_to_wishlist` entry whose `ecommerce.items[0].item_id` is that listing
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/loop_events.feature` `Loop events: favoriting emits one favorite event and the add_to_wishlist dataLayer entry`

#### Scenario: Loop events: un-favoriting emits nothing
- **WHEN** a buyer who has just favorited a listing clicks the heart again to remove it
- **THEN** the total count of `EVENT_TYPE_FAVORITE` events for that listing and session on `analytics.events` stays 1 after 10 s
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/loop_events.feature` `Loop events: un-favoriting emits nothing`

#### Scenario: Loop events: sharing emits one share event per copy
- **WHEN** a signed-in buyer clicks "Chia sẻ" on a listing page and then clicks "Sao chép liên kết" once
- **THEN** exactly 2 `EVENT_TYPE_SHARE` events with that listing id and `properties.method = "copy_link"` are on `analytics.events`, and none contains the short code
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/loop_events.feature` `Loop events: sharing emits one share event per copy`

#### Scenario: Loop events: following a shop emits follow_seller and unfollowing emits nothing
- **WHEN** a signed-in buyer opens a shop page, clicks "+ Theo dõi", then clicks "✓ Đang theo dõi"
- **THEN** exactly one `EVENT_TYPE_FOLLOW_SELLER` event with `properties.seller_id` equal to the shop is on `analytics.events` after 10 s
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/loop_events.feature` `Loop events: following a shop emits follow_seller and unfollowing emits nothing`

#### Scenario: Loop events: a guest cannot produce loop events
- **WHEN** a guest opens a listing page and clicks the heart, or opens a shop page and clicks "+ Theo dõi", and the gateway answers `Unauthenticated`
- **THEN** zero `EVENT_TYPE_FAVORITE` and zero `EVENT_TYPE_FOLLOW_SELLER` events are on `analytics.events` for that session
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/loop_events.feature` `Loop events: a guest cannot produce loop events`

### Requirement: Referral, check-in and alert events fire only after server success

`RedeemReferralForm` SHALL call `trackEcommerce("referral_redeem")` with no properties only when `redeemReferralAction` returns `ok`; the code SHALL NOT appear in any tracking argument. `LoyaltyWidget` SHALL call `trackEcommerce("daily_check_in", ...)` only when `checkInAction` returns `ok` with a result, with `properties.streak = String(streak)` when `streak` is a positive integer and no `streak` otherwise. `AlertToggle` SHALL call `trackEcommerce("alert_subscribe", { items: [{ itemId: listingId }], properties: { alert_type } })` with `alert_type` `price_drop` or `back_in_stock` only when `subscribeAlertAction` returns `ok`, and nothing on unsubscribe or failure. A guest SHALL NOT produce these events.

#### Scenario: Loop events: referral redeem tracked without the code
- **WHEN** a buyer submits code `ABC123` and `redeemReferralAction` returns `ok`
- **THEN** `trackEcommerce` is called exactly once with the single argument `"referral_redeem"` and no argument contains `ABC123`
- **VERIFIED BY**: `team-frontend/src/features/account/referral/RedeemReferralForm.test.tsx` `tracks referral_redeem without the code`

#### Scenario: Loop events: rejected referral code tracks nothing
- **WHEN** `redeemReferralAction` returns `ok: false`, or the code is empty and the button is disabled
- **THEN** `trackEcommerce` is called 0 times and the error is shown
- **VERIFIED BY**: `team-frontend/src/features/account/referral/RedeemReferralForm.test.tsx` `tracks nothing on a rejected code`

#### Scenario: Loop events: check-in tracked with the streak
- **WHEN** `checkInAction` returns `ok` with `streak` 3
- **THEN** `trackEcommerce` is called exactly once with `("daily_check_in", { properties: { streak: "3" } })`
- **VERIFIED BY**: `team-frontend/src/features/engagement/LoyaltyWidget.test.tsx` `tracks daily_check_in with streak`

#### Scenario: Loop events: check-in without a streak omits the property
- **WHEN** `checkInAction` returns `ok` with `streak` 0
- **THEN** `trackEcommerce` is called exactly once with `("daily_check_in", {})`, so no `streak` key is present
- **VERIFIED BY**: `team-frontend/src/features/engagement/LoyaltyWidget.test.tsx` `omits streak when absent`

#### Scenario: Loop events: failed or repeated check-in tracks nothing
- **WHEN** `checkInAction` returns `ok: false` (for example already checked in today)
- **THEN** `trackEcommerce` is not called and the error toast is shown
- **VERIFIED BY**: `team-frontend/src/features/engagement/LoyaltyWidget.test.tsx` `tracks nothing on failure`

#### Scenario: Loop events: alert subscribe tracked with its type
- **WHEN** a signed-in buyer turns on "Giảm giá" for listing `L1` and `subscribeAlertAction` returns `ok`
- **THEN** `trackEcommerce` is called exactly once with `("alert_subscribe", { items: [{ itemId: "L1" }], properties: { alert_type: "price_drop" } })`
- **VERIFIED BY**: `team-frontend/src/components/alerts/AlertToggle.test.tsx` `tracks alert_subscribe with the type`

#### Scenario: Loop events: unsubscribe, failure and guest tracking is empty
- **WHEN** the buyer turns an alert off, or `subscribeAlertAction` returns `ok: false`, or a guest clicks the toggle
- **THEN** `trackEcommerce` is called 0 times
- **VERIFIED BY**: `team-frontend/src/components/alerts/AlertToggle.test.tsx` `tracks nothing off failure or guest`

#### Scenario: Loop events: redeeming a referral code emits referral_redeem without the code
- **WHEN** a buyer submits another user's valid referral code on `/account/referral`
- **THEN** exactly one `EVENT_TYPE_REFERRAL_REDEEM` event is on `analytics.events` after 10 s and the serialized event does not contain the code string
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/loop_events.feature` `Loop events: redeeming a referral code emits referral_redeem without the code`

#### Scenario: Loop events: an invalid referral code emits nothing
- **WHEN** a buyer submits a code that does not exist
- **THEN** the error is shown and zero `EVENT_TYPE_REFERRAL_REDEEM` events are on `analytics.events` after 10 s
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/loop_events.feature` `Loop events: an invalid referral code emits nothing`

#### Scenario: Loop events: daily check-in emits daily_check_in with the streak
- **WHEN** a buyer who has not checked in today clicks "Điểm danh"
- **THEN** exactly one `EVENT_TYPE_DAILY_CHECK_IN` event is on `analytics.events` after 10 s, with `properties.streak` equal to the streak shown on the widget
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/loop_events.feature` `Loop events: daily check-in emits daily_check_in with the streak`

#### Scenario: Loop events: subscribing to an alert emits alert_subscribe and unsubscribing emits nothing
- **WHEN** a signed-in buyer turns on "Giảm giá" on a listing page, then turns it off
- **THEN** exactly one `EVENT_TYPE_ALERT_SUBSCRIBE` event with `properties.alert_type = "price_drop"` and that listing id is on `analytics.events` after 10 s
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/loop_events.feature` `Loop events: subscribing to an alert emits alert_subscribe and unsubscribing emits nothing`
