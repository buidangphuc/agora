## Decisions

- Provenance on the wire: the collector already forwards `impressionId` and `modelVersion` on any event (initiative spec), so no new beacon field exists. `properties` stays `{surface, action}` so the earlier "no free text" rule and its e2e check keep holding. The single-event branch of `dispatcher.ts` (no items) previously dropped these fields, so `EcommerceParams` gains optional `impressionId` and `modelVersion` used there. `ai_interaction` for the assistant `shown` has no listing, so it needs this branch.
- Empty provenance (old team-ai, error) sends neither field. It never sends a placeholder.
- Assistant cards use the existing `TrackImpression` leaf (coalesces into `view_item_list`), not a new tracker. `impressionId` is the reply's `request_id`, so impressions, `shown` and `accepted` of one reply share one id.
- `/assistant` keeps its message list shape. The streaming patch code and the `streamed` variable are deleted. The apology text stays for a failed call and no `shown` is emitted.
- `getVariant(flag, entityId)`: `entityId` is supplied by the caller, hashed as SHA-256(salt + id) with `FLAG_ENTITY_SALT`, evaluated with the existing OpenFeature client `getStringValue`. No new dependency. Returns `string | null`. `null` means unassigned: no exposure.
- `ExperimentExposure` is a `"use client"` leaf like `AiReviewSummaryShown`. The server component that called `getVariant` renders it with the resolved variant. One beacon per mount, not per re-render.
- The server cannot read the anonymous id (it lives in `localStorage` under `bds_anonymous_id`). This slice therefore evaluates only for a signed-in user. Anonymous visitors get `null`. See open issue 1.
- No page calls `getVariant` here: the flags are created switched off and no live experiment starts (U4). The helper and the leaf are built and unit-tested. They are verified by unit tests only; storage of the event is covered end to end by `analytics-experiment-reports`.
- Removing the stream route leaves `platform.chat.v1.ChatService` in place on the gateway and in team-ai. Other callers and e2e features (`ai/llm_*`) use it directly.

## Risks

- `chatCopilot` changes return type. Callers: `features/chat/actions.ts` and its tests only.
- The `ai-assistant` requirement removal may also be done by `ai-assistant-grounding`. Whichever archives second drops its copy.
