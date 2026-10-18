## Why

Sign-up, login and the loop actions (favorite, share, follow a shop, redeem a referral code, daily check-in, alert subscribe) leave no trace in `analytics.events`, so signup to first purchase and the retention loop cannot be queried (backlog Q4, `growth-retention.md` #4). The tracking pipeline already exists (`trackEcommerce` to `/api/track` to the gateway to Kafka) but these actions never call it, and `favorite`/`share` exist in the vocabulary without any caller. This slice makes the shop UI fire exactly one event per completed action, after server success only. AI surface events are a separate slice (`frontend-ai-interaction-events`).

initiative: q4-loop-instrumentation

depends_on: gateway-loop-event-intake (the collector must accept the new beacon types; until it is merged the new beacons are answered 400 and dropped, which the best-effort queue tolerates). Contract: `EVENT_TYPE_SIGN_UP..EVENT_TYPE_ALERT_SUBSCRIBE` from q4-loop-events-contract; warehouse names from analytics-loop-event-storage.

## What Changes

- `src/lib/analytics/schema.ts`, `map.ts` (hot files, this slice edits them first): `GA4EventName` gains `sign_up`, `login`, `add_to_wishlist`, `follow_seller`, `referral_redeem`, `daily_check_in`, `alert_subscribe` and loses the unused `favorite`; `InternalEventType` gains `sign_up`, `login`, `follow_seller`, `referral_redeem`, `daily_check_in`, `alert_subscribe`. `add_to_wishlist` maps to internal `favorite` (beacon `type: "favorite"`), the other new names map to themselves. The AI slice adds `ai_interaction` after this one; do not add it here.
- `src/lib/analytics/dataLayer.ts`: the caller-property allowlist (`ALLOWED_PROPERTIES`, today only `valid`) gains `method`, `content_type`, `seller_id`, `streak`, `alert_type`. Values still pass `scrubText`. Without this the dataLayer entry could not carry `method`.
- `src/features/auth/actions.ts`: on success only, after `setSession` and before `redirect("/")`, `loginAction` sets cookie `agora_auth_evt=login` and `registerAction` sets `agora_auth_evt=sign_up` (`httpOnly:false`, `sameSite:"lax"`, `path:"/"`, `maxAge:60`, `secure` exactly as the session cookie config in `setSession` sets it). No cookie on any failure branch.
- New client component `AuthEventBeacon` (`src/features/auth/AuthEventBeacon.tsx`) mounted once in `src/app/(shop)/layout.tsx`. On mount it reads `agora_auth_evt`, deletes it (Max-Age 0, same path) before firing, and if the value is exactly `sign_up` or `login` calls `trackEcommerce(name, { properties: { method: "password" } })`. Any other value is deleted and ignored.
- `FavoriteButton`: after `await addFavoriteAction(id)` resolves (the action throws on failure), `trackEcommerce("add_to_wishlist", { items: [{ itemId: id }] })`. Nothing on remove, nothing in the `catch`.
- `ShareButton`: after `createShareLinkAction` returns `ok` with a `shortCode`, and on every later click that copies the existing link, `trackEcommerce("share", { items: [{ itemId: id }], properties: { method: "copy_link", content_type: "listing" } })`.
- `FollowSellerButton`: on `res.ok` of `followSellerAction` only, `trackEcommerce("follow_seller", { properties: { seller_id: sellerId } })`.
- `RedeemReferralForm`: on `res.ok`, `trackEcommerce("referral_redeem")` with no properties; the code is never passed to tracking.
- `LoyaltyWidget`: on `res.ok && res.result`, `trackEcommerce("daily_check_in", ...)` with `properties.streak = String(res.result.streak)` when the streak is a positive integer, no `streak` property otherwise.
- `AlertToggle` (`ToggleButton`): on subscribe `res.ok`, `trackEcommerce("alert_subscribe", { items: [{ itemId: listingId }], properties: { alert_type: kind } })` where `kind` is `price_drop` or `back_in_stock`. Unsubscribe and failure emit nothing.
- Tests (colocated, vitest) and e2e scenarios in `platform-e2e/tests/e2e/features/tracking/loop_events.feature`.

No proto, gateway, analytics or server-side change; no new dependency; no new network call on any server action path (the beacon is the existing queued `/api/track` POST).

## NFR

- Tracking never blocks or delays the user action: every call is the existing synchronous, fire-and-forget `trackEcommerce` (never throws), invoked after the action has already resolved. Toasts, optimistic state and redirects are unchanged.
- The login/sign-up event is queued within the first client render after the redirect (one effect run) and reaches `analytics.events` within the existing collector flush window (2 s queue idle flush plus collector budget); the e2e allows 10 s.
- At-most-once per action: one `trackEcommerce` call per success; the auth cookie is deleted before the event fires, so a reload, a back navigation or React StrictMode double-effect cannot repeat it.
- Beacon loss is accepted (best effort, as for all browser tracking today); no retry beyond the queue's existing one.
- Payload bounds: properties are short enums or ids, well under the gateway generic limits.

## Data classification

- Cookie `agora_auth_evt` (value `sign_up` or `login`). Purpose: carry "this redirect follows a successful registration/login" across the server-action redirect so the client can fire the event. Not personal data: the value is one of two fixed words, with no user id, token, name or timestamp. Not HttpOnly by design (the client must read and delete it). Retention: at most 60 s, deleted by the client on first read. Consent basis: functional cookie set in direct response to the user's own submit; the first-party beacon is sent regardless of analytics consent, carrying the consent snapshot the gateway attaches from the `agora_consent` cookie (`collector.go` `parseConsentCookie`/`newConsentState`), exactly as for existing events; only the dataLayer push is gated by analytics consent. Classified functional and non-personal, with no consent prompt; DPO confirmation of this classification remains an open human item.
- Principal id (Consent basis: unchanged from existing analytics.events collection): not sent by the frontend. The gateway resolves it from the session cookie that the same response set, and writes it in the envelope (existing behaviour). Purpose: link signup to first purchase. Retention: as `analytics.events` today.
- `listingId` (favorite, share, alert) and `seller_id` (follow): business identifiers, not personal. `streak` (integer string): loyalty counter of the signed-in user, low sensitivity, tied to the principal like any event. `method` (`password`, `copy_link`), `content_type` (`listing`), `alert_type`: fixed enums.
- Never sent: the referral code, the share short code or URL, the username, the password, any free text.
- Existing anonymous/session ids ride on the beacon as for every event (unchanged).

## Threat model

- Assets: integrity of the funnel data (events must reflect completed actions); the session and credentials (must not leak through the cookie or the event); the referral code (secret-ish, must not reach analytics).
- Abuse cases:
  - A visitor hand-sets `agora_auth_evt=login` in the browser to inflate logins: the beacon is a self-reported client event like every other beacon (the gateway attributes it to the session principal or anonymous); no privilege or data is gained, and warehouse readers must treat these as client-reported. Accepted; server-side delivery is a non-goal of the initiative.
  - A forged cookie with another value (script, long string): only the two exact values are honoured; anything else is deleted and ignored, never put in the beacon or the DOM.
  - Replay by reload or back button: the cookie is deleted before firing and has Max-Age 60.
  - Stolen cookie value reveals nothing (two fixed words).
  - Referral code or share URL leaking into properties or `path`: tracking calls take no code or short code argument; `path` is `location.pathname`, and `/s/<code>` is not tracked on this path. Verified by tests that assert the serialized beacon contains neither.
- Authz rule: every loop action is already gated by its server action and the gateway session (a guest cannot favorite, follow, check in, subscribe or redeem; `AlertToggle` blocks guests before the action). Events fire only from the success branch of those calls, so a guest or an unauthorised call produces no event. No new endpoint, scope or role.
