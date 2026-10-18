## Why

Four AI surfaces (shopping assistant, seller chat copilot, Magic Listing, AI review summary) emit no tracking events, so "AI usage -> conversion" cannot be answered from the warehouse (ai-data review #10). The tracking contract and the gateway allowlist for `ai_interaction` come from the sibling slices; this slice makes the browser emit it.

initiative: q4-loop-instrumentation

depends_on: gateway-loop-event-intake (collector accepts `ai_interaction` with the allowlist), frontend-loop-events (adds `ai_interaction` to `GA4EventName`, `InternalEventType` and `map.ts` first; assumed landed, this slice does not edit `schema.ts` or `map.ts`).

## What Changes

- New helper `trackAiInteraction(surface, action, listingId?)` in `src/lib/analytics/ai.ts`. It calls `trackEcommerce("ai_interaction", { properties: { surface, action }, items: listingId ? [{ itemId: listingId }] : undefined })`. Its TypeScript types are the two allowlists, so a value outside them does not compile. It never accepts free text.
- `/assistant` (`AssistantChatView.tsx`): `shown` once per AI message that has at least one product card (keyed by message id, so streaming patches and re-renders do not repeat it); `accepted` on a click of a product card link ("Xem Ngay" or "Mua Ngay") with `listingId` = that card.
- Seller chat copilot (`ChatView.tsx`): `shown` once per `loadCopilot` result that returns at least one suggestion; `accepted` when a copilot suggestion chip is inserted into the composer; `edited` once per inserted suggestion on the first composer change that makes the text differ from the inserted suggestion, before sending. `listingId` = the active thread's listing when set. The buyer quick-reply chips are not AI and emit nothing.
- Magic Listing (`MagicListingAssist.tsx`, `ListingForm.tsx`): `shown` once per generated draft that renders; `accepted` when "Áp dụng" (a field) or "Áp dụng tất cả" is clicked; `edited` once per draft when, after an apply, the seller changes an applied field in the form (title, description or price) before save. The `dismissed` cell is dropped: the card has no close control and this slice adds none (see design.md).
- AI review summary (`AiReviewSummary.tsx`): a tiny client leaf `AiReviewSummaryShown` rendered inside the card fires `shown` once on mount with the listing id. The summary stays a server component and renders nothing, with no event, when the service fails or returns nothing.
- No change to props that reach the beacon beyond `surface`, `action` and `listingId`. No prompt, reply, suggestion text, title, description, review text, `request_id` or `model_version` is ever sent.

No proto, gateway or analytics change. No new dependency. Dropped cells (never added): `assistant.edited|dismissed`, `chat_copilot.dismissed`, `magic_listing.dismissed`, `review_summary.accepted|edited|dismissed`.

## NFR

- Fire-and-forget through `trackEcommerce` (never throws into the caller, safe during SSR). No new network call on any AI action path; the AI call latency is unchanged. A tracking failure or a blocked beacon never changes the AI UI.
- Exactly one event per observable action: `shown` is not re-sent on re-render, state update, streaming patch or strict-mode double effect.
- Beacon reaches `analytics.events` within the existing collector budget (batching flush about 2 s idle, unchanged).
- Added client JS: one small leaf for the summary and one helper module, no new dependency.

## Data classification

- `surface` and `action`: closed enums, not personal. Purpose: AI feature usage and conversion analysis. Basis: legitimate interest (product analytics), first-party beacon follows the existing consent snapshot rules; the dataLayer copy only with analytics consent (unchanged). Retention: the `analytics.events` policy (unchanged).
- `listingId`: business identifier, not personal. Same purpose, basis and retention.
- Principal, anonymous and session ids: already in the envelope and beacon today; unchanged.
- Explicitly never sent: buyer chat message text and assistant query/reply, copilot suggestion text, Magic Listing title/description/price, review text, model id, request id.

## Threat model

- Assets: free text of buyers and sellers (chat, prompts, reviews) kept out of the warehouse; integrity of AI usage metrics.
- Abuse cases:
  1. A developer later passes content into `properties` -> the helper signature accepts only the two enums plus a listing id, and a unit test asserts the beacon `properties` keys equal exactly `surface` and `action`.
  2. A visitor forges beacons with other values -> gateway allowlist answers 400 (gateway slice, not this slice); the browser only emits allowlisted values.
  3. Inflating metrics by re-render or repeated clicks -> once-per-result guards for `shown`/`edited`; `accepted` is one per click (each click is a user act).
  4. A buyer triggering seller-only copilot events -> the copilot UI renders only when the viewer is the thread's seller (`isSeller`), so a buyer produces no `chat_copilot` event.
- Authz rule: no new authorization; events are fire-and-forget telemetry with the existing session identity. AI endpoints keep their own gateway authz.
