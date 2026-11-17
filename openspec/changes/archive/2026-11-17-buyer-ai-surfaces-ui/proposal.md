## Why

The buyer app shows AI output as if it were verified and does not tell the platform which model produced it. `/assistant` streams text and then overwrites it with the unary reply (`AssistantChatView.tsx:126`). It renders `ratingText` unconditionally. The home `AiAssistantModal` invents a `matchScore` between 75 and 99 and a canned advice sentence in the browser. `ai_interaction` beacons cannot be tied to the response that caused them, and no feature-flag exposure is recorded. After `ai-assistant-grounding`, `ai-copilot-grounding` and `analytics-experiment-reports` land, team-ai answers from live catalog data and returns `request_id` and `model_version`, and the collector accepts `experiment_exposure` (event type 22). This slice makes the buyer UI show only that grounded answer and pass the provenance on to analytics.

initiative: docs-ai-data

## What Changes

- `/assistant` (`AssistantChatView`, `features/assistant/actions.ts`): one unary `ShoppingAssistant` call per question (decision U9). The `fetch("/api/assistant/stream")` call and the route handler `src/app/api/assistant/stream/route.ts` are deleted. The reply text, cards and follow-ups come from that one response. A card shows its rating line only when `ratingText` is non-empty. A failed call shows the existing apology text and no cards.
- Assistant cards are wrapped in `TrackImpression` with `placementId = assistant.suggestions`, `impressionId = request_id` and `modelVersion = model_version` of the reply that produced them.
- `lib/gateway/ai.ts`: `shoppingAssistant`, `chatCopilot` and `summarizeReviews` also return `requestId` and `modelVersion` (proto fields from `docs-ai-data-contract`). `chatCopilot` returns `{ quickReplies, requestId, modelVersion }` instead of a bare list.
- `lib/analytics/ai.ts`: `trackAiInteraction(surface, action, listingId?, provenance?)`. When `provenance` is given the beacon carries `impressionId = requestId` and `modelVersion` as wire fields. `properties` stay exactly `{surface, action}`. `AiAction` gains `"dismissed"` so the type matches the collector allowlist. No surface emits `dismissed` yet. `dispatcher.ts` forwards `impressionId` and `modelVersion` from `EcommerceParams` on the single-event path (no items) and as a fallback on the item path. The assistant, the seller chat copilot (`ChatView`) and the AI review summary pass the provenance of their response on every `shown`, `accepted` and `edited`.
- Home: `AiAssistantModal` loses its keyword matcher, `matchScore` and canned advice. `AssistantBlock` renders a link to `/assistant`, and no longer loads the feed.
- Flags: `lib/flags` gains `getVariant(flag, entityId)` (server-only, OpenFeature + Flipt, string variant flag) and a client leaf `ExperimentExposure` that sends one `experiment_exposure` beacon `{flag, variant}` per mount. `schema.ts` and `map.ts` gain the `experiment_exposure` event name.
- `openspec/specs/ai-assistant`: the requirement "Shopping Assistant reply streams token-by-token" is removed. `openspec/specs/ai-interaction-events`: the payload requirement and the assistant requirement are modified for provenance and for the removed stream path.

## Capabilities

### New Capabilities
- `buyer-ai-surfaces-ui`: grounded, measured buyer AI surfaces and the flag exposure beacon.

### Modified Capabilities
- `ai-interaction-events`: beacons carry provenance in wire fields; the streaming scenario goes away.
- `ai-assistant`: the streaming requirement is removed.

## Impact

- team-frontend: `src/features/assistant/*`, `src/features/ai/AiAssistantModal.tsx`, `src/features/home/AssistantBlock.tsx`, `src/features/chat/{ChatView.tsx,actions.ts}`, `src/features/review/*`, `src/lib/gateway/ai.ts`, `src/lib/analytics/{ai.ts,dispatcher.ts,schema.ts,map.ts}`, `src/lib/flags/*`, `src/features/tracking/ExperimentExposure.tsx` (new), removal of `src/app/api/assistant/stream/route.ts`.
- team-frontend `proto/` and generated stubs are already refreshed by `docs-ai-data-contract`.
- platform-e2e: new feature file `frontend/buyer_ai_surfaces.feature`; `tracking/ai_interaction_events.feature` is extended; team-frontend FEATURES.yaml entries are added by the e2e-author.
- No gateway or team-ai change here: the collector allowlist for `experiment_exposure` is `analytics-experiment-reports`; the answers are `ai-assistant-grounding` and `ai-copilot-grounding`.

## Non-goals

- Photo-aware Magic Listing and the seller app (`seller-forecast-card`).
- Streaming the grounded answer, order-status questions, personalised rows.
- Starting a live experiment: the flags stay off, and no page calls `getVariant` in this slice (see design.md).
- Emitting `dismissed` on any surface.

## Non-functional requirements

- `/assistant` answer: one gateway round trip; p95 at most 1.5 s end to end on the mock backend (budget set by `ai-assistant-grounding`); the UI adds no second call.
- `getVariant`: p95 added latency at most 20 ms, hard timeout 50 ms; on timeout, error, disabled flags or missing entity it returns `null` and no exposure is sent. A Flipt outage never fails or delays a page by more than the timeout.
- Tracking never blocks or alters the AI UI: a tracker failure is swallowed (existing rule kept).
- Availability: team-ai down shows the apology text and no cards; the page stays usable.
- Accessibility and copy: Vietnamese, existing design tokens.

## Data classification

| field | personal data | purpose | consent basis | retention |
|---|---|---|---|---|
| `request_id` as beacon `impressionId` | no: random UUID per AI call, no buyer id | tie shown/accepted/edited events to the response | first-party analytics as for other beacons (decision U6; DPO sign-off is a human item) | as the `analytics.events` warehouse |
| `model_version` as beacon `modelVersion` | no: model label | AI-surface report per model | as above | as above |
| `experiment_exposure` `{flag, variant}` | no: flag key and variant key; visitor identity only via the standard envelope `anonymousId`/`sessionId` | experiment readout | as above (U6) | as above |
| Flipt entity id (server to Flipt) | pseudonymous: salted SHA-256 of the signed-in user id, never the raw id | deterministic variant assignment | legitimate interest in running first-party experiments; DPO sign-off is a human item | not stored by the frontend; Flipt keeps no entity data for local evaluation |
| assistant prompt text | yes, free text typed by the buyer | answer the question | contract (the buyer asks the assistant) | not stored by the frontend; never put in a beacon (existing rule kept) |

## Threat model

- Assets: buyer privacy (free-text prompts, identity), integrity of the AI-surface report, trust in what the assistant shows.
- Abuse cases: free text or reply text leaking into beacon `properties`, `impressionId` or `modelVersion`; a forged provenance value polluting reports (accepted: provenance is a first-party client claim, as every beacon field is, and reports are aggregate only); raw user id sent to Flipt or logged; an invented rating or match score shown as fact; an exposure sent for an arm the visitor was not assigned.
- Authorisation rule: no new endpoint. `/assistant` and the copilot keep their existing gateway checks (buyer session, copilot only for the thread's seller). The frontend never calls team-ai or Flipt from the browser: Server Actions and Server Components do (Rule 1). `getVariant` is `server-only`.
