# ai-interaction-events Specification

## Purpose
Frontend AI surfaces (shopping assistant, seller chat copilot, Magic Listing, review summary) report shown, accepted and edited interactions with a minimal payload.

## Requirements

### Requirement: AI interaction events carry only surface, action and an optional listing id

Every `ai_interaction` beacon SHALL be sent through `trackEcommerce("ai_interaction", ...)` with `properties` containing exactly the keys `surface` and `action`, `surface` in {`assistant`, `chat_copilot`, `magic_listing`, `review_summary`}, `action` in {`shown`, `accepted`, `edited`, `dismissed`}, and `listingId` only when the interaction concerns one listing. No prompt, reply, suggestion, title, description, review text, `request_id` or `model_version` SHALL be sent. Tracking SHALL NOT block or alter the AI UI.

#### Scenario: The helper sends only allowlisted keys
- **WHEN** `trackAiInteraction("magic_listing", "accepted", "lst_1")` is called
- **THEN** exactly one beacon is queued with `type = "ai_interaction"`, `listingId = "lst_1"` and `properties` equal to `{ surface: "magic_listing", action: "accepted" }`, with no other property key
- **AND** `trackEcommerce` adds no other key to `properties` (it forwards `params.properties` verbatim and sets no `placementId`, `modelVersion` or `impressionId` because no item carries them)
- **VERIFIED BY**: `team-frontend/src/lib/analytics/ai.test.ts` `sends only surface and action` (unit-only: cannot be produced with mock team-ai or without a browser, AGENTS.md 9b; its FEATURES entry is `not-testable`)

#### Scenario: A failing tracker never breaks the AI action
- **WHEN** `trackEcommerce` throws or `navigator.sendBeacon` is unavailable
- **THEN** the AI action completes, the result still renders and no error is shown
- **VERIFIED BY**: `team-frontend/src/lib/analytics/ai.test.ts` `swallows tracker failure` (unit-only: cannot be produced with mock team-ai or without a browser, AGENTS.md 9b; its FEATURES entry is `not-testable`)

#### Scenario: AI events are accepted by the collector and carry no free text
- **WHEN** a buyer asks the assistant a question containing the marker text "SECRET-PROMPT-7" and the mock AI answers with product cards
- **THEN** every `/api/track` batch captured in the browser that contains an `ai_interaction` beacon was answered 2xx by the gateway (not 400)
- **AND** no captured beacon body contains "SECRET-PROMPT-7" or the reply text, and each `ai_interaction` `properties` has exactly the keys `surface` and `action`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `AI events are accepted by the collector and carry no free text`

### Requirement: The shopping assistant reports shown and accepted

`AssistantChatView` SHALL emit `ai_interaction {surface=assistant, action=shown}` once for each AI message that renders at least one product card, and `{surface=assistant, action=accepted}` with `listingId` = the card's listing on each click of that card's "Xem Ngay" or "Mua Ngay" link. It SHALL NOT emit for the greeting, for streamed text without cards, or for a reply without cards. No `edited` or `dismissed` event exists on this surface.

#### Scenario: A reply with product cards reports shown once
- **WHEN** the assistant reply renders with 2 product cards and the component then re-renders (the visitor types in the input box)
- **THEN** exactly one `ai_interaction` with `surface=assistant`, `action=shown` is queued for that reply, and none for the greeting
- **VERIFIED BY**: `team-frontend/src/features/assistant/AssistantChatView.test.tsx` `reports shown once per reply with cards`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `A reply with product cards reports shown once`

#### Scenario: A reply without cards reports nothing
- **WHEN** the assistant answers with text only (zero product cards) or the unary call fails
- **THEN** zero `ai_interaction` events are queued
- **VERIFIED BY**: `team-frontend/src/features/assistant/AssistantChatView.test.tsx` `reports nothing without cards` (unit-only: cannot be produced with mock team-ai or without a browser, AGENTS.md 9b; its FEATURES entry is `not-testable`)

#### Scenario: Clicking a product card reports accepted with the listing
- **WHEN** the visitor clicks "Xem Ngay" or "Mua Ngay" on the card for listing `lst_9` (one click each, separately)
- **THEN** each click queues exactly one `ai_interaction` with `surface=assistant`, `action=accepted`, `listingId=lst_9`, and the navigation (`/listing/lst_9` or `/checkout`) proceeds
- **VERIFIED BY**: `team-frontend/src/features/assistant/AssistantChatView.test.tsx` `reports accepted with the card listing`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `Clicking a product card reports accepted with the listing`

#### Scenario: Two replies report two shown events
- **WHEN** the visitor gets a second reply that also has cards
- **THEN** exactly two `shown` events exist for the conversation, one per reply, and the first reply is not re-reported
- **VERIFIED BY**: `team-frontend/src/features/assistant/AssistantChatView.test.tsx` `reports shown once per reply with cards`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `Two replies report two shown events`

#### Scenario: Streamed text whose final message has no cards reports nothing
- **WHEN** `/api/assistant/stream` streams reply text and the unary call then returns a reply with zero product cards (or fails), so the final AI message has no cards
- **THEN** zero `ai_interaction` events are queued, including during the streaming patches
- **VERIFIED BY**: `team-frontend/src/features/assistant/AssistantChatView.test.tsx` `streamed text without cards reports nothing` (unit-only: cannot be produced with mock team-ai or without a browser, AGENTS.md 9b; its FEATURES entry is `not-testable`)

### Requirement: The seller chat copilot reports shown, accepted and edited

`ChatView` SHALL emit `{surface=chat_copilot, action=shown}` once per copilot load that returns at least one suggestion, `{action=accepted}` when a copilot chip is inserted into the composer, and `{action=edited}` once per inserted suggestion when the composer text is first changed to differ from it before sending; `listingId` = the active thread's listing when it has one. Buyer quick-reply chips and a copilot load with zero suggestions or an error SHALL NOT emit. A viewer who is not the thread's seller has no copilot and SHALL NOT emit `chat_copilot` events.

#### Scenario: Suggestions render and are reported once
- **WHEN** the seller clicks "Gợi ý AI" and the mock AI returns 3 suggestions
- **THEN** exactly one `chat_copilot` `shown` is queued, even after the seller types or the thread list re-renders
- **VERIFIED BY**: `team-frontend/src/features/chat/ChatView.test.tsx` `reports shown once per copilot load`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `Suggestions render and are reported once`

#### Scenario: Inserting a suggestion reports accepted
- **WHEN** the seller clicks a suggestion chip
- **THEN** the composer holds that suggestion and exactly one `chat_copilot` `accepted` is queued
- **VERIFIED BY**: `team-frontend/src/features/chat/ChatView.test.tsx` `reports accepted on insert`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `Inserting a suggestion reports accepted`

#### Scenario: Inserting a second suggestion reports a second accepted and re-arms edited
- **WHEN** after inserting suggestion A and editing it (one `edited`), the seller clicks suggestion B and edits again
- **THEN** a second `chat_copilot` `accepted` is queued and exactly one more `edited` follows the second edit (two `accepted`, two `edited` in total)
- **VERIFIED BY**: `team-frontend/src/features/chat/ChatView.test.tsx` `second insert re-arms edited`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `Inserting a second suggestion reports a second accepted and re-arms edited`

#### Scenario: Changing the inserted suggestion reports edited once
- **WHEN** after inserting a suggestion the seller types 3 more characters
- **THEN** exactly one `chat_copilot` `edited` is queued (not one per keystroke); sending the message as-is without any change queues no `edited`
- **VERIFIED BY**: `team-frontend/src/features/chat/ChatView.test.tsx` `reports edited once and only when changed`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `Changing the inserted suggestion reports edited once`

#### Scenario: No suggestions or a failed load reports nothing
- **WHEN** the copilot call returns an empty list or fails
- **THEN** zero `ai_interaction` events are queued
- **VERIFIED BY**: `team-frontend/src/features/chat/ChatView.test.tsx` `reports nothing for empty or failed copilot` (unit-only: cannot be produced with mock team-ai or without a browser, AGENTS.md 9b; its FEATURES entry is `not-testable`)

#### Scenario: Not the owner of the thread, no copilot and no copilot events
- **WHEN** a buyer opens a thread (and, in the unit test, clicks a seller-configured quick-reply chip)
- **THEN** the "Gợi ý AI" button is absent and zero `ai_interaction` events are queued
- **VERIFIED BY**: `team-frontend/src/features/chat/ChatView.test.tsx` `buyer quick replies emit nothing`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `Not the owner of the thread, no copilot and no copilot events`

### Requirement: Magic Listing reports shown, accepted and edited

`MagicListingAssist` SHALL emit `{surface=magic_listing, action=shown}` once per generated draft that renders, `{action=accepted}` on each click of a field "Áp dụng" or "Áp dụng tất cả", and `ListingForm` SHALL emit `{action=edited}` once per draft when, after an apply, the seller changes an applied field (title, description or price) before save. `listingId` is set when editing an existing listing. A failed or empty generation SHALL emit nothing. `dismissed` SHALL NOT be emitted on this surface.

#### Scenario: A generated draft reports shown once
- **WHEN** the seller clicks "Tạo gợi ý" and the mock AI returns a draft
- **THEN** exactly one `magic_listing` `shown` is queued, and none when the form re-renders after typing elsewhere
- **VERIFIED BY**: `team-frontend/src/features/listing/MagicListingAssist.test.tsx` `reports shown once per draft`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `A generated draft reports shown once`

#### Scenario: Applying reports accepted
- **WHEN** the seller clicks "Áp dụng tất cả"
- **THEN** the form fields are filled and exactly one `magic_listing` `accepted` is queued
- **VERIFIED BY**: `team-frontend/src/features/listing/MagicListingAssist.test.tsx` `reports accepted on apply`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `Applying reports accepted`

#### Scenario: Changing an applied field reports edited once
- **WHEN** after applying the draft the seller edits the title and then the description
- **THEN** exactly one `magic_listing` `edited` is queued for that draft; editing a field that was never applied, or editing before any apply, queues none
- **VERIFIED BY**: `team-frontend/src/features/listing/ListingForm.test.tsx` `reports edited once after apply`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `Changing an applied field reports edited once`

#### Scenario: A failed or empty generation reports nothing
- **WHEN** `magicListingAction` returns an error or no data
- **THEN** the error alert shows and zero `ai_interaction` events are queued
- **VERIFIED BY**: `team-frontend/src/features/listing/MagicListingAssist.test.tsx` `reports nothing on failed generation` (unit-only: cannot be produced with mock team-ai or without a browser, AGENTS.md 9b; its FEATURES entry is `not-testable`)

#### Scenario: A guest cannot reach Magic Listing and emits nothing
- **WHEN** a guest (no session) requests `/seller/new`
- **THEN** the guest is not authorised (unauthenticated, redirected to `/login`), the Magic Listing card never renders and zero `ai_interaction` events are queued
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `A guest cannot reach Magic Listing and emits nothing`

#### Scenario: A new draft starts a new cycle
- **WHEN** the seller generates a second draft after editing the first
- **THEN** a second `shown` is queued and a later change of an applied field of the second draft queues a second `edited`
- **VERIFIED BY**: `team-frontend/src/features/listing/ListingForm.test.tsx` `reports edited once after apply`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `A new draft starts a new cycle`

### Requirement: The AI review summary reports shown

The summary card SHALL contain a client leaf that emits `{surface=review_summary, action=shown, listingId}` once on mount. When the summary is not rendered (no reviews, service unavailable, error) no event SHALL be emitted. No other action exists on this surface.

#### Scenario: The summary card reports shown once
- **WHEN** a PDP with reviews renders the AI summary card and the page re-renders on the client in any way
- **THEN** exactly one `review_summary` `shown` with the listing id is queued per mounted card
- **VERIFIED BY**: `team-frontend/src/features/review/AiReviewSummary.test.tsx` `reports shown once on mount`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `The summary card reports shown once`

#### Scenario: No summary, no event
- **WHEN** the listing has no reviews or `summarizeReviews` fails
- **THEN** the card is absent and zero `ai_interaction` events are queued
- **VERIFIED BY**: `team-frontend/src/features/review/AiReviewSummary.test.tsx` `reports nothing without a summary`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `No summary, no event`
