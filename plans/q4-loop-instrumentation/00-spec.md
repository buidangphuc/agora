# 00-spec — Activation, loop and AI interaction events

initiative: q4-loop-instrumentation

Source: backlog Q4 (`plans/pm-review-2026-10/BACKLOG.md`), evidence `plans/pm-review-2026-10/growth-retention.md` #4 and `plans/pm-review-2026-10/ai-data.md` #10.
Base: stacked on `asdlc/q3-audit-log` (Q2 → Q3 → Q4 chain). Q1 (`asdlc/q1-session-continuity`) also edits `team-frontend/src/features/auth/actions.ts`; that conflict is resolved after Q1 merges.

## Goal
Every activation and loop action (sign up, log in, share, favorite, follow a shop, redeem a referral code, daily check-in, subscribe to an alert) and every interaction with an AI surface (shopping assistant, chat copilot, Magic Listing, AI review summary) produces exactly one first-party tracking event that lands in the analytics warehouse with a stable event type. When the visitor has analytics consent, the same action is also pushed to the GTM `dataLayer` under its GA4 name. Signup → first purchase and AI usage → conversion become answerable with a warehouse query; no new report UI is built here.

## User stories
1. **Activation is measured.**
   - Given a visitor on `/register` (or `/login`).
   - When registration (login) succeeds.
   - Then exactly one `sign_up` (`login`) tracking event with `properties.method = "password"` reaches `analytics.events`. It carries the new user's principal, and the dataLayer receives `sign_up` (`login`) `{method: "password"}` when analytics consent is granted.
   - Edge cases:
     - A failed attempt (validation error, wrong password, duplicate username) emits nothing.
     - A reload of the landing page after the event fired does not emit a second event.
2. **Loop actions are measured.**
   - Given a signed-in buyer.
   - When they add a favorite, share a listing, follow a shop, redeem a referral code, check in, or subscribe to an alert, and the server action succeeds.
   - Then exactly one event of the matching type (table below) reaches `analytics.events`.
   - Edge cases:
     - Un-favorite, unfollow and unsubscribe emit nothing.
     - A failed server action emits nothing; the optimistic UI rolling back emits nothing.
     - Copying an already-created share link again does emit `share` again (each copy is a share).
3. **AI surfaces are measured.**
   - Given any user on an AI surface.
   - When an AI result is shown to them, or they accept, edit or dismiss it.
   - Then one `ai_interaction` event is emitted with `properties.surface` and `properties.action` from the allowlists below, and no free text (no prompt, reply, title or review text).
4. **The collector rejects malformed new events.**
   - Given a beacon to `/api/track` with type `ai_interaction` and a `surface` or `action` outside the allowlist, or missing.
   - Then the gateway answers 400 and publishes nothing, exactly as for an unknown type today.

## Event catalogue
| action (UI trigger, after success) | beacon `type` (first-party) | `EventType` | GA4 dataLayer name | properties |
|---|---|---|---|---|
| registration succeeded | `sign_up` | `EVENT_TYPE_SIGN_UP` | `sign_up` (recommended) | `method=password` |
| login succeeded | `login` | `EVENT_TYPE_LOGIN` | `login` (recommended) | `method=password` |
| share link created or copied (listing) | `share` (exists) | `EVENT_TYPE_SHARE` (exists) | `share` (recommended) | `method=copy_link`, `content_type=listing`; `listingId` = the listing |
| favorite added | `favorite` (exists) | `EVENT_TYPE_FAVORITE` (exists) | `add_to_wishlist` (recommended; replaces the unused `favorite` name) | `listingId` = the listing |
| shop followed | `follow_seller` | `EVENT_TYPE_FOLLOW_SELLER` | `follow_seller` (custom) | `seller_id` |
| referral code redeemed | `referral_redeem` | `EVENT_TYPE_REFERRAL_REDEEM` | `referral_redeem` (custom) | none (the code is never sent) |
| daily check-in succeeded | `daily_check_in` | `EVENT_TYPE_DAILY_CHECK_IN` | `daily_check_in` (custom) | `streak` (integer as string) when the response carries it |
| alert subscribed | `alert_subscribe` | `EVENT_TYPE_ALERT_SUBSCRIBE` | `alert_subscribe` (custom) | `alert_type` (the AlertToggle type); `listingId` = the listing |
| AI surface interaction | `ai_interaction` | `EVENT_TYPE_AI_INTERACTION` | `ai_interaction` (custom) | `surface`, `action`; `listingId` when the interaction is about one listing |

AI allowlists (gateway-enforced):
- `surface` ∈ {`assistant`, `chat_copilot`, `magic_listing`, `review_summary`}
- `action` ∈ {`shown`, `accepted`, `edited`, `dismissed`}
- Mapping per surface:

  | surface | `shown` | `accepted` | `edited` | `dismissed` |
  |---|---|---|---|---|
  | `assistant` (`/assistant`) | a reply with product cards renders | a product card link is clicked (`listingId` = card) | – | – |
  | `chat_copilot` (seller chat) | suggestions render | a suggestion is inserted into the composer | the inserted suggestion is changed before sending | – |
  | `magic_listing` (seller new/edit listing) | a generated draft renders | the draft is applied | applied, then a generated field is changed before save | the draft is closed without applying |
  | `review_summary` (PDP) | the summary card renders | – | – | – |

  Slice specs may drop a cell the UI cannot observe, but never add a new `surface`/`action` value.

## Non-goals
- Server-side GA4 delivery (Measurement Protocol) of `sign_up` or any new event; no new identity event. The first-party beacon is the warehouse source of truth.
- Reports: no new team-analytics RPC, gateway route or admin panel (signup → first purchase, AI readouts and cohorts belong to Q16).
- `request_id` / `model_version` on AI events (needs ai.proto + team-ai; Q16).
- Changing GA4 reconciliation (`ga4_reconcile.go`) or the GA4 import.
- Seller-side loop events beyond the AI surfaces (listing created, order shipped).
- Unfollow / unfavorite / unsubscribe events.

## Architecture decisions
- New `EventType` enum values, not overloaded `click` + properties — Why: the warehouse `event_type` column and the gateway allowlist already key on the enum; a new value is additive and `buf breaking` safe.
- All browser events go through `trackEcommerce` (dispatcher) — Why: it already applies the consent gate to the dataLayer, fans out to `/api/track`, dedupes by `eventId`; no second path.
- Login and sign-up are fired on the client after the redirect: the server action, on success only, sets a one-shot non-HttpOnly cookie `agora_auth_evt` (`sign_up` | `login`, `Max-Age=60`, `SameSite=Lax`, `Path=/`) that a client component mounted in the shop layout reads once, fires the event, and deletes — Why: the server action ends in `redirect()`, and the beacon must carry the new session's principal; a cookie survives the redirect without touching URLs.
- Events fire only after the server action reports success (not on click) — Why: the metric is completed actions; optimistic UI rollbacks must not count.
- The gateway validates `ai_interaction` properties against the allowlists and rejects (400) otherwise; other new types use the existing generic bounds — Why: keep free text out of the warehouse at the edge.
- team-analytics stores the new types under their lowercase beacon names (`sign_up`, `login`, `follow_seller`, `referral_redeem`, `daily_check_in`, `alert_subscribe`, `ai_interaction`) — Why: `eventTypeName` is the stable warehouse vocabulary; an unknown enum today lands as `unspecified`.
- No new dependencies.

## Contracts
```proto
// platform/analytics/v1/analytics.proto — append to enum EventType (additive)
// Registration completed (GA4 sign_up). properties: method.
EVENT_TYPE_SIGN_UP = 15;
// Login completed (GA4 login). properties: method.
EVENT_TYPE_LOGIN = 16;
// A buyer followed a shop. properties: seller_id.
EVENT_TYPE_FOLLOW_SELLER = 17;
// A referral code was redeemed. No properties (the code is never sent).
EVENT_TYPE_REFERRAL_REDEEM = 18;
// Daily loyalty check-in completed. properties: streak (optional).
EVENT_TYPE_DAILY_CHECK_IN = 19;
// A price/stock alert was subscribed. properties: alert_type; listing_id set.
EVENT_TYPE_ALERT_SUBSCRIBE = 20;
// An interaction with an AI surface. properties: surface, action (allowlisted, no free text).
EVENT_TYPE_AI_INTERACTION = 21;
```
Beacon wire format (`POST /api/track`, `WireTrackBeacon`) is unchanged; the new `type` strings are the beacon names in the catalogue.
Existing contracts consumed unchanged: `analytics.events/TrackingEvent`, `EngagementService.FollowSeller`, `EngagementService.CheckIn`, `ReferralService.RedeemReferral`, `IdentityService.Register`, `IdentityService.Login`.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| New event types in the tracking contract | q4-loop-events-contract | platform-core | – | provides: analytics.events/TrackingEvent (EventType 15-21) | platform-core (+ proto copies) |
| Collector accepts the new types; AI allowlist | gateway-loop-event-intake | team-gateway | q4-loop-events-contract | uses: analytics.events/TrackingEvent | team-gateway, platform-e2e |
| Warehouse stores the new types by name | analytics-loop-event-storage | team-analytics | q4-loop-events-contract | uses: analytics.events/TrackingEvent | team-analytics, platform-e2e |
| Activation + loop events fired from the shop UI | frontend-loop-events | team-frontend | gateway-loop-event-intake | uses: analytics.events/TrackingEvent | team-frontend, platform-e2e |
| AI surface interaction events | frontend-ai-interaction-events | team-frontend | gateway-loop-event-intake | uses: analytics.events/TrackingEvent | team-frontend, platform-e2e |

Hot files edited by more than one slice (initiative-build serialises those slices):
- `team-frontend/src/lib/analytics/schema.ts`, `team-frontend/src/lib/analytics/map.ts` — frontend-loop-events, frontend-ai-interaction-events (frontend-loop-events first).
- `platform-e2e/tests/e2e/support/` tracking helpers — any slice that adds a helper.

## Out of initiative
- Wiring `placementId="assistant.suggestions"` and the LLM stream overwrite fix on `/assistant` (ai-data quick win 1) — separate item.

## NFR and data
- NFR: tracking never blocks or delays the user action (fire-and-forget, as today); the beacon reaches `analytics.events` within the existing collector budget. No new network call on the server action path.
- Personal data: events carry the principal id (already in the envelope) and the existing anonymous/session ids; new properties carry no free text and no referral code. `seller_id` and `listing_id` are business identifiers. Consent: dataLayer only with analytics consent (unchanged); the first-party beacon path follows the existing consent snapshot rules.
- Risk routing expected: proto (contract slice), pii (privacy-reviewer on frontend slices: auth event cookie, AI events).

## Conventions
- Mirror the existing tracking tests: `team-frontend/src/lib/analytics/*.test.ts`, `team-frontend/src/app/(shop)/listing/[id]/tracking.test.tsx`; gateway `team-gateway/internal/edge/collector_test.go`; analytics `team-analytics/internal/consumer/tracking_test.go`.
- E2E: extend `platform-e2e/tests/e2e/features/tracking/` (beacon intercept / analytics.events consumer helpers already exist); do not add new GA4 DebugView dependencies.
- Do not touch `ga4relay`, `ga4import`, or `ga4_reconcile.go`.

## Locked
2026-10-15 — Confirmed by Bùi Đăng Phúc. Client-only sign_up/login via one-shot agora_auth_evt cookie; events after server success only; EventType 15-21 additive; gateway AI allowlist; warehouse storage only (reports Q16); AI events surface+action only, no free text; favorite → GA4 add_to_wishlist; 5 slices, FE slices serialised; stacked on asdlc/q3-audit-log.
