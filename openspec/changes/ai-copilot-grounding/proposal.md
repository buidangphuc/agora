## Why

The seller chat copilot (`AIService.ChatCopilot`) answers every stock question with "còn sẵn hàng", promises same-day dispatch and delivery windows nobody checked, and ignores `listing_id` (`team-ai/app/modules/business/ai_assistant/copilot.py`). A seller who clicks one of these drafts tells a buyer a false thing in the shop's name. Separately, none of the seller-side AI responses (`ChatCopilot`, `MagicListing`, `SummarizeReviews`) says which request or model version produced it, so the `ai_interaction` events the frontends send cannot be tied to a version (Q16, Q15).

This slice grounds the copilot in the live listing (stock, status, owner) read from team-domain, and stamps `request_id` and `model_version` on the three responses. The shopping assistant half of Q15 is `ai-assistant-grounding`; photo-aware Magic Listing is out of scope (decision U1), so Magic Listing only gains provenance here.

initiative: docs-ai-data

## What Changes

- `ChatCopilotService` reads the discussed listing through `ListingService.GetListing(listing_id)` and builds its drafts from the facts: stock 0 yields drafts that say the item is out of stock and never claim availability; stock above 0 may say the item is in stock; an unknown listing, a lookup failure, a listing owned by another seller or a listing that is not published yields drafts with no stock claim. The invented delivery times, same-day dispatch, "đặt trước 16h" and "luôn có sẵn" sentences are removed from every draft; delivery-time questions get drafts that ask for the buyer's address and make no time promise.
- New selector `COPILOT_BACKEND` = `mock|llm_router`, default `mock`, mirroring `SCREEN_BACKEND` (`app/core/config/ai.py`, `build_screen_service`). `mock` is a deterministic template over the retrieved facts; `llm_router` sends the redacted buyer message plus the facts through team-ai's existing LLM chain and falls back to the mock template on timeout or invalid output.
- `ChatCopilot`, `MagicListing` and `SummarizeReviews` fill `request_id` (UUID hex minted per call) and `model_version`: `rules-v1` for rule output, `llm_router:<prompt_version>` when the LLM produced the copilot drafts. The proto fields come from `docs-ai-data-contract`.
- team-ai evals gain a copilot groundedness set (`evals/`, `run_eval --min-score 0.8`): no draft contradicts the listing facts.
- `team-ai/FEATURES.yaml` entries for `ai.chat-copilot`, `ai.magic-listing`, `ai.summarize-reviews` gain the new acceptance lines (written by the e2e-author, not this slice's code track).

Uses, unchanged: `AIService.ChatCopilot`, `AIService.MagicListing`, `AIService.SummarizeReviews`, `ListingService.GetListing`. Depends on `docs-ai-data-contract` (provenance fields) and `ai-assistant-grounding` (hot files `app/core/config/ai.py`, `app/bootstrap/services.py`, `servicers/ai.py`, and the team-domain listing client and address it adds).

## Non-goals

- Photo-aware Magic Listing, attributes, variants, sold-price comparables (decision U1).
- The shopping assistant (`ai-assistant-grounding`) and the frontend beacons (`buyer-ai-surfaces-ui`, `seller-forecast-card`).
- Grounding claims about warranty, authenticity or return policy: the copilot has no listing data for them (see OPEN_ISSUES in the hand-off).
- Order-status questions.

## Non-functional requirements

- Latency: copilot p95 at most 300 ms with `COPILOT_BACKEND=mock`, including one `GetListing` call with a 200 ms timeout. `llm_router` is bounded by `COPILOT_TIMEOUT_SECONDS` (default 3 s), then falls back to the mock template.
- Availability: a failing or slow team-domain never fails `ChatCopilot`; the response degrades to drafts with no stock claim. `MagicListing` and `SummarizeReviews` gain no new dependency.
- Limits: the buyer message sent to the LLM is the redacted text, truncated to 1000 characters; at most 3 drafts are returned, each at most 300 characters.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `buyer_message` (request) | personal, free text | pick the intent and draft replies | contract with the seller, on behalf of the buyer who wrote it | not stored; logged only after `RedactionPolicy.redact_text` |
| `seller_id` (request) | personal id | authz check against the caller and the listing owner | legitimate interest, security | not stored; never logged raw |
| `listing_id`, listing stock, status, title, price | public catalog data | ground the drafts | n/a | not stored |
| `request_id` | pseudonymous correlation id, no user data | tie an `ai_interaction` event to the response | legitimate interest, measurement | lives in the analytics warehouse, same retention as the events |
| `model_version` | non-personal | name the version that answered | n/a | same as above |

The LLM prompt carries the redacted message and catalog facts only: no buyer id, no seller id.

## Threat model

- Assets: seller trust (a false stock claim sent in the shop's name), listing data of other sellers, buyer message text.
- Abuse 1: a seller passes another seller's `listing_id` to read its stock. Mitigation: the listing's `seller_id` must equal the caller; otherwise the copilot treats the listing as unknown and returns no stock claim, and nothing from that listing reaches the response or the prompt.
- Abuse 2: a seller passes another `seller_id` in the request. Mitigation: existing check, `PERMISSION_DENIED` unless the caller is that seller or admin.
- Abuse 3: prompt injection in `buyer_message` steers the LLM into a stock or delivery promise. Mitigation: the LLM output is validated against the facts (no in-stock claim when stock is 0 or unknown, no delivery-time claim) and replaced by the mock template when it fails.
- Abuse 4: log leakage of ids or message text. Mitigation: no raw id in any log line, message only after redaction.
- Authz rule: `ChatCopilot` needs the seller scope; the request `seller_id` equals the caller or the caller is admin; listing facts are used only when the listing owner equals the effective seller.
