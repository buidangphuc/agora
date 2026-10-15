## Decisions

- Shared helper, not hot files: the new names reach `GA4EventName`/`InternalEventType`/`map.ts` through frontend-loop-events. This slice only adds `src/lib/analytics/ai.ts` (a typed wrapper) so the call sites cannot pass free text.
- `listingId` is sent as `items: [{ itemId }]`, the same way `share`/`favorite` do, so the beacon `listingId` field carries it. When absent, no `items`.
- `shown` on `/assistant`: a `useEffect` over `messages` with a `useRef<Set<string>>` of message ids already reported; a message qualifies when `sender === "ai"` and `product_cards.length > 0`. The streaming placeholder has no cards, so streamed text alone never fires. The ref survives React strict-mode remount simulation, so dev double effects do not duplicate.
- `shown` on the review summary: the card is rendered by an async server component, so a client leaf with a `useRef` guard fires once on mount. The leaf is not rendered when the summary is null.
- Chat copilot `edited`: on insert the component records the inserted string in a ref; the composer `onChange` fires `edited` once if the new value differs from it, then clears the ref. Sending, a thread switch or a new insert resets the ref. Inserting a suggestion again counts as a new `accepted`.
- Magic Listing `edited`: `MagicListingAssist` exposes `onApplied(fields)` bookkeeping through a small callback prop; `ListingForm` keeps the set of applied fields and fires `edited` (once per draft) on the first `onChange` of an applied field. A new generated draft resets it.
- `dismissed` for Magic Listing is dropped: the draft card has no close control, so there is nothing observable. Adding a control is a UI change outside this instrumentation slice. Allowed by the initiative ("Slice specs may drop a cell the UI cannot observe").
- E2E runs with team-ai in its default mock mode; no scenario configures a real LLM provider.

## Clarifications

### Round 1 — 2026-10-15

- (a) `magic_listing` `dismissed` is dropped. Accepted by the human; no UI change.
- (b) Assistant `accepted` fires on both "Xem Ngay" and "Mua Ngay", one event per click, `listingId` = the card.
- (c) The gateway rejects any `ai_interaction` property key other than `surface` and `action`, so the helper sends exactly those two keys. `trackEcommerce` forwards `params.properties` verbatim and adds no property key of its own (checked in `dispatcher.ts`: extra fields go to top-level beacon fields only).
- E2E exemptions: failure, empty and text-only paths cannot be produced with mock team-ai and are unit-only (`not-testable`). Second reply, second draft and the buyer-without-copilot case are producible and have e2e coverage.

### Round 2 — 2026-10-15
- Q1 [data] tasks.md prerequisite: frontend-loop-events was told not to add `ai_interaction`, and this slice assumed it existed, so nobody added it (beacons would map to type `view`) → **this slice adds `ai_interaction` to `GA4EventName`, `InternalEventType` and `map.ts` (both directions) with a `map.test.ts` case; nothing else in those files changes** (human).
