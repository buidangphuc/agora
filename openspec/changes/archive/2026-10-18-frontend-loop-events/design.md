## Decisions

- One cookie, two values, consumed in the shop layout. `loginAction` and `registerAction` end in `redirect("/")`, so the browser never sees a success value. The cookie is written before `redirect()` (which throws), after `setSession`, never on a failure branch. The consumer is a client component in `(shop)/layout.tsx` because `/login`, `/register` and `/` all live under `(shop)`; checkout has its own layout and is never a post-login landing.
- Delete-then-fire order in `AuthEventBeacon` (`document.cookie = "agora_auth_evt=; Max-Age=0; Path=/; SameSite=Lax"` first). A module-level `consumed` flag is not needed because the cookie deletion is synchronous and StrictMode's second effect run finds no cookie.
- `add_to_wishlist` carries the listing as an `items` entry so the dispatcher fans out one beacon with `listingId` set (internal type `favorite`) and the GA4 `ecommerce.items` shape is correct. `share` and `alert_subscribe` do the same. `follow_seller`, `referral_redeem`, `daily_check_in`, `sign_up`, `login` have no item, so `listingId` is `""` (generic bounds allow it).
- DataLayer property allowlist is extended rather than bypassed so the existing privacy test (`dataLayer.privacy.test.ts`) keeps its meaning: only named non-personal keys pass, still scrubbed.
- Favorite success is "the action promise resolved": `addFavoriteAction` returns `void` and throws on failure, so `await` followed by the track call inside the same `try` but after the await; the catch path rolls back and tracks nothing. No change to the action itself.
- Share fires on a successful create (first click) and on each later copy click; a rejected `navigator.clipboard.writeText` does not suppress it (the link exists; the copy is best effort already `void`ed).
- Streak: `CheckInResult.streak` is a number, and proto3 omits 0, so "carries it" is implemented as "a positive integer".
- Auth and AI slices both edit `schema.ts`/`map.ts`; this slice lands first, the AI slice appends `ai_interaction`.

## Clarifications

### Round 1 — 2026-10-15

- The `agora_auth_evt` cookie uses the same options as the session cookie: SameSite=Lax, Path=/, Secure exactly as the session cookie config sets it, not HttpOnly, Max-Age 60. Classified functional and non-personal, no consent prompt; DPO confirmation stays an open human item.
- `streak` is sent only when it is a positive integer (accepted).
- Consent, verified from code: the dispatcher always queues the first-party beacon and the gateway attaches the consent snapshot from the `agora_consent` cookie (`team-gateway/internal/edge/collector.go` `parseConsentCookie`/`newConsentState`). The beacon is sent regardless of analytics consent, as for existing events; only the dataLayer push is gated.
