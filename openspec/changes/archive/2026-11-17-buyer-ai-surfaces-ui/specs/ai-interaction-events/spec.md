## MODIFIED Requirements

### Requirement: AI interaction events carry only surface, action and an optional listing id

Every `ai_interaction` beacon SHALL be sent through `trackEcommerce("ai_interaction", ...)` with `properties` containing exactly the keys `surface` and `action`, `surface` in {`assistant`, `chat_copilot`, `magic_listing`, `review_summary`}, `action` in {`shown`, `accepted`, `edited`, `dismissed`}, and `listingId` only when the interaction concerns one listing. The response `request_id` and `model_version` SHALL travel only in the wire fields `impressionId` and `modelVersion`, never in `properties`. No prompt, reply, suggestion, title, description or review text SHALL be sent. Tracking SHALL NOT block or alter the AI UI.

#### Scenario: The helper sends only allowlisted keys
- **WHEN** `trackAiInteraction("magic_listing", "accepted", "lst_1")` is called
- **THEN** exactly one beacon is queued with `type = "ai_interaction"`, `listingId = "lst_1"` and `properties` equal to `{ surface: "magic_listing", action: "accepted" }`, with no other property key
- **AND** the beacon carries no `impressionId` and no `modelVersion` because no provenance was passed
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

## REMOVED Requirements

### Requirement: The shopping assistant reports shown and accepted
**Reason**: Replaced by "The shopping assistant reports shown and accepted per grounded reply": the streaming path and its scenario no longer exist (decision U9) and events now carry the reply provenance.
**Migration**: None; the unchanged scenarios are carried over to the replacement requirement.

## ADDED Requirements

### Requirement: The shopping assistant reports shown and accepted per grounded reply

`AssistantChatView` SHALL emit `ai_interaction {surface=assistant, action=shown}` once for each AI message that renders at least one product card, and `{surface=assistant, action=accepted}` with `listingId` = the card's listing on each click of that card's "Xem Ngay" or "Mua Ngay" link. Each event SHALL carry the `request_id` and `model_version` of the reply that holds the card. It SHALL NOT emit for the greeting or for a reply without cards. No `edited` or `dismissed` event exists on this surface.

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
