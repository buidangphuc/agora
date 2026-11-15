## ADDED Requirements

### Requirement: The assistant page shows the grounded unary answer only

`/assistant` SHALL send each question as exactly one `ShoppingAssistant` call through the gateway and SHALL render its `replyText`, product cards and follow-ups. It SHALL NOT call `ChatService/StreamChat` or `/api/assistant/stream`, and the route handler SHALL NOT exist. A card SHALL show its rating line only when `ratingText` is non-empty. When the call fails the page SHALL show the apology text and no cards.

#### Scenario: One call per question and no stream
- **WHEN** a buyer asks "tai nghe dưới 500k" on `/assistant` and the mock backend answers with 2 cards
- **THEN** the browser makes exactly 1 request to the assistant Server Action, 0 requests to `/api/assistant/stream`, and the bubble text equals the `replyText` of that response
- **VERIFIED BY**: `team-frontend/src/features/assistant/AssistantChatView.test.tsx` `asks once and never streams`; `platform-e2e/tests/e2e/features/frontend/buyer_ai_surfaces.feature` `One call per question and no stream`

#### Scenario: The stream route is gone
- **WHEN** a client POSTs `/api/assistant/stream`
- **THEN** the response status is 404
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/buyer_ai_surfaces.feature` `The stream route is gone`

#### Scenario: A card without a rating shows no rating line
- **WHEN** a reply card has `ratingText` equal to ""
- **THEN** the card renders its title and price and no element with the rating text, no star glyph and no "đánh giá"
- **VERIFIED BY**: `team-frontend/src/features/assistant/AssistantChatView.test.tsx` `hides the rating line when empty`

#### Scenario: A rated card shows exactly the returned rating text
- **WHEN** a reply card has `ratingText` equal to "4.5/5 (12 đánh giá)"
- **THEN** the card shows "4.5/5 (12 đánh giá)" verbatim
- **VERIFIED BY**: `team-frontend/src/features/assistant/AssistantChatView.test.tsx` `shows the returned rating text`

#### Scenario: A failed call shows the apology and no cards
- **WHEN** `askAssistantAction` returns `ok = false` with no message
- **THEN** the bubble shows "Xin lỗi, trợ lý AI đang tạm thời không phản hồi. Vui lòng thử lại." and renders 0 cards
- **VERIFIED BY**: `team-frontend/src/features/assistant/AssistantChatView.test.tsx` `shows the apology on failure`

#### Scenario: A reply without cards is shown as text only
- **WHEN** the response has `replyText` "Mình chưa tra cứu được sản phẩm phù hợp" and 0 cards
- **THEN** the bubble shows that text, renders 0 cards and still renders the follow-up chips returned
- **VERIFIED BY**: `team-frontend/src/features/assistant/AssistantChatView.test.tsx` `renders a text-only reply`

### Requirement: Assistant cards report impressions with their placement and provenance

Each product card rendered for a reply SHALL be wrapped in `TrackImpression` with `placementId = "assistant.suggestions"`, `impressionId` = the reply's `request_id`, `modelVersion` = the reply's `model_version` and `position` = the 1-based card index. When the reply has no `request_id`, the card SHALL send no `impressionId`.

#### Scenario: Card impressions carry the placement and the request id
- **WHEN** a reply with `request_id` "r1", `model_version` "mock:v1|search-v2" and 2 cards is visible
- **THEN** the queued `impression` beacons number 2, each has `placementId = "assistant.suggestions"`, `impressionId = "r1"`, `modelVersion = "mock:v1|search-v2"`, and positions 1 and 2
- **VERIFIED BY**: `team-frontend/src/features/assistant/AssistantChatView.test.tsx` `cards report impressions with provenance`; `platform-e2e/tests/e2e/features/frontend/buyer_ai_surfaces.feature` `Card impressions carry the placement and the request id`

#### Scenario: A reply without a request id sends no impression id
- **WHEN** the reply has `request_id` equal to "" and 1 card
- **THEN** the queued impression beacon has no `impressionId` key and no `modelVersion` key
- **VERIFIED BY**: `team-frontend/src/features/assistant/AssistantChatView.test.tsx` `omits empty provenance`

#### Scenario: A reply without cards queues no impression
- **WHEN** the reply has 0 cards
- **THEN** 0 `impression` beacons are queued
- **VERIFIED BY**: `team-frontend/src/features/assistant/AssistantChatView.test.tsx` `reports nothing without cards`

### Requirement: AI interaction beacons carry the response provenance in wire fields

`trackAiInteraction` SHALL accept an optional `{ requestId, modelVersion }`. When `requestId` is non-empty the beacon SHALL carry `impressionId = requestId`, and when `modelVersion` is non-empty it SHALL carry `modelVersion`. The beacon `properties` SHALL stay exactly `{surface, action}`. The assistant, the seller chat copilot and the AI review summary SHALL pass the provenance of the response that produced the interaction on every `shown`, `accepted` and `edited` event. The dispatcher SHALL forward the two fields on events with no items.

#### Scenario: The helper puts provenance in wire fields only
- **WHEN** `trackAiInteraction("assistant", "shown", undefined, { requestId: "r1", modelVersion: "mock:v1" })` is called
- **THEN** one beacon is queued with `impressionId = "r1"`, `modelVersion = "mock:v1"` and `properties` equal to `{ surface: "assistant", action: "shown" }`
- **VERIFIED BY**: `team-frontend/src/lib/analytics/ai.test.ts` `sends provenance in wire fields only`

#### Scenario: Empty provenance sends nothing extra
- **WHEN** `trackAiInteraction("review_summary", "shown", "lst_1", { requestId: "", modelVersion: "" })` is called
- **THEN** the queued beacon has no `impressionId` key and no `modelVersion` key
- **VERIFIED BY**: `team-frontend/src/lib/analytics/ai.test.ts` `omits empty provenance`

#### Scenario: The assistant accepted click reuses the reply provenance
- **WHEN** the buyer clicks "Xem Ngay" on a card of the reply with `request_id` "r1"
- **THEN** the queued `ai_interaction` has `impressionId = "r1"`, `listingId` = the card listing and `modelVersion` of that reply
- **VERIFIED BY**: `team-frontend/src/features/assistant/AssistantChatView.test.tsx` `accepted carries reply provenance`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `The assistant accepted click reuses the reply provenance`

#### Scenario: Copilot events carry the copilot response provenance
- **WHEN** the seller loads suggestions whose response has `request_id` "c1" and then inserts and edits one
- **THEN** the `shown`, `accepted` and `edited` events each carry `impressionId = "c1"`
- **VERIFIED BY**: `team-frontend/src/features/chat/ChatView.test.tsx` `copilot events carry provenance`

#### Scenario: The review summary shown event carries the summary provenance
- **WHEN** a PDP renders the AI summary whose response has `request_id` "s1" and `model_version` "mock:v1"
- **THEN** the single `review_summary` `shown` has `impressionId = "s1"` and `modelVersion = "mock:v1"`
- **VERIFIED BY**: `team-frontend/src/features/review/AiReviewSummary.test.tsx` `shown carries provenance`

#### Scenario: Provenance never enters properties or free text
- **WHEN** a buyer asks a question containing "SECRET-PROMPT-7" and the reply cards are shown and clicked
- **THEN** no captured `/api/track` body contains "SECRET-PROMPT-7" or the reply text, every `ai_interaction` `properties` has exactly the keys `surface` and `action`, and the gateway answers each batch 2xx
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `AI events are accepted by the collector and carry no free text`

#### Scenario: A viewer without the seller role emits no copilot provenance
- **WHEN** a buyer who is not the thread's seller sends a message in a thread (no copilot role)
- **THEN** 0 `ai_interaction` events with `surface = chat_copilot` are queued
- **VERIFIED BY**: `team-frontend/src/features/chat/ChatView.test.tsx` `buyer quick replies emit nothing`

#### Scenario: A tracker failure still leaves the AI action working
- **WHEN** the tracker throws while the assistant renders a reply with provenance
- **THEN** the reply and cards still render and no error is shown
- **VERIFIED BY**: `team-frontend/src/lib/analytics/ai.test.ts` `swallows tracker failure`

### Requirement: The home page links to the assistant and invents no match

`AssistantBlock` SHALL render a link to `/assistant` and SHALL NOT compute, show or send any match score, canned advice or locally ranked product list. `AiAssistantModal` keyword matching SHALL be deleted.

#### Scenario: The home entry points to the assistant page
- **WHEN** a visitor opens `/`
- **THEN** the assistant entry is a link whose `href` is `/assistant` and activating it loads `/assistant`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/buyer_ai_surfaces.feature` `The home entry points to the assistant page`; `team-frontend/src/features/home/AssistantBlock.test.tsx` `links to the assistant page`

#### Scenario: No invented score is rendered
- **WHEN** the home page renders the assistant entry
- **THEN** the markup contains no "%" match badge and no text matching "Phù hợp \d+%"
- **VERIFIED BY**: `team-frontend/src/features/home/AssistantBlock.test.tsx` `renders no match score`

#### Scenario: The entry does not load the feed
- **WHEN** the feed loader is unavailable (it throws)
- **THEN** `AssistantBlock` still renders the link
- **VERIFIED BY**: `team-frontend/src/features/home/AssistantBlock.test.tsx` `does not depend on the feed`

### Requirement: The copilot request names the thread listing and keeps seller-only access

`chatCopilotAction` SHALL pass the active thread's `listingId` to `ChatCopilot` and SHALL return the response `quickReplies`, `requestId` and `modelVersion`. The "Gợi ý AI" control SHALL stay visible only to the thread's seller.

#### Scenario: The listing id reaches the copilot call
- **WHEN** the seller clicks "Gợi ý AI" in a thread about listing `lst_7`
- **THEN** the gateway `chatCopilot` is called once with `listingId = "lst_7"`
- **VERIFIED BY**: `team-frontend/src/features/chat/actions.test.ts` `passes the thread listing`

#### Scenario: A failed copilot call shows no suggestions
- **WHEN** the gateway `chatCopilot` call fails
- **THEN** the action returns `ok = false`, the previous suggestions stay unchanged and 0 `ai_interaction` events are queued
- **VERIFIED BY**: `team-frontend/src/features/chat/ChatView.test.tsx` `reports nothing for empty or failed copilot`

#### Scenario: A buyer has no copilot
- **WHEN** a buyer who is not the thread's seller (not the owner of the thread) opens the thread
- **THEN** the "Gợi ý AI" button is absent and `chatCopilotAction` is never called
- **VERIFIED BY**: `team-frontend/src/features/chat/ChatView.test.tsx` `buyer quick replies emit nothing`; `platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature` `A buyer has no copilot`

### Requirement: Flag variants are evaluated on the server and exposure is reported once

`getVariant(flag, entityId)` SHALL be server-only and SHALL evaluate a Flipt string variant flag with the targeting key SHA-256(`FLAG_ENTITY_SALT` + entityId) as lowercase hex, never the raw id. It SHALL return `null` when flags are disabled, `entityId` is empty, the evaluation errors or takes longer than 50 ms. The `ExperimentExposure` client leaf SHALL send one `experiment_exposure` beacon with `properties` exactly `{flag, variant}` per mount, and SHALL send nothing when `variant` is `null` or empty.

#### Scenario: A resolved variant is returned and the id is hashed
- **WHEN** `getVariant("home-feed-ranker", "u-1")` runs and Flipt returns variant "b"
- **THEN** it returns "b" and the targeting key passed to Flipt equals the SHA-256 hex of the salt plus "u-1", with no occurrence of "u-1"
- **VERIFIED BY**: `team-frontend/src/lib/flags/variant.test.ts` `hashes the entity id`

#### Scenario: A slow or failing Flipt yields no variant
- **WHEN** Flipt takes 80 ms or throws
- **THEN** `getVariant` returns `null` within 60 ms and logs no entity id
- **VERIFIED BY**: `team-frontend/src/lib/flags/variant.test.ts` `times out to null`

#### Scenario: Disabled flags or an empty entity yield no variant
- **WHEN** `FEATURE_FLAGS_ENABLED` is "false", or `entityId` is ""
- **THEN** `getVariant` returns `null` without calling Flipt
- **VERIFIED BY**: `team-frontend/src/lib/flags/variant.test.ts` `returns null when disabled or without entity`

#### Scenario: Exposure is sent once per mount with only flag and variant
- **WHEN** `<ExperimentExposure flag="home-feed-ranker" variant="b" />` mounts and the parent re-renders twice
- **THEN** exactly one beacon is queued, with `type = "experiment_exposure"` and `properties` equal to `{ flag: "home-feed-ranker", variant: "b" }`
- **VERIFIED BY**: `team-frontend/src/features/tracking/ExperimentExposure.test.tsx` `sends one exposure per mount` (unit-only: no page calls `getVariant` while the flags are off, so no browser journey can produce it, AGENTS.md 9b; its FEATURES entry is `not-testable`; storage of type 22 is covered by `analytics-experiment-reports`)

#### Scenario: No variant sends no exposure
- **WHEN** `<ExperimentExposure flag="home-feed-ranker" variant={null} />` mounts
- **THEN** 0 beacons are queued
- **VERIFIED BY**: `team-frontend/src/features/tracking/ExperimentExposure.test.tsx` `sends nothing without a variant`

#### Scenario: A browser bundle never contains the flag client
- **WHEN** the production client bundle is built
- **THEN** no client chunk imports `@openfeature/server-sdk` or contains the Flipt address
- **VERIFIED BY**: `team-frontend/src/lib/flags/variant.test.ts` `is server-only`
