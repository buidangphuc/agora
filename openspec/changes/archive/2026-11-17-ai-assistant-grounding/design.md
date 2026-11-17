## Context

Today the assistant retrieves through `ProductRetriever` (RAG or demo catalog) and renders a fixed template (`shopping.py`). `ShoppingAssistantRequest.top_k` exists in the Python schema but not on the wire. team-ai already has a gRPC client pattern for team-search (`GrpcStockLookup`, service principal metadata) and a `mock|llm_router` backend pattern (`build_screen_service`).

## Decisions

- Retrieval order: `SearchListings` first (hybrid, `filters={"in_stock": "true"}`, `max_price` parsed from the message, page size 5), then `GetListing` per hit in parallel with `asyncio.gather`. Why: search owns ranking, stock and rating projections; domain owns the authoritative title, price and status. Grounding is in the data path, not only in the prompt.
- Price parsing is a small deterministic function over the accent-stripped message: "dưới|không quá|tối đa|under|<" followed by a number and an optional unit (`k`, `nghìn`, `ngàn`, `tr`, `triệu`, `đ`). No match means no `max_price`. Why: testable, no LLM dependency in retrieval. "dưới 500k" gives `max_price=500000`.
- A card is kept only when `GetListing` returns status `PUBLISHED`. Search can lag domain (read-model), so domain wins; a hit whose listing is unpublished or missing is dropped. If every hydration fails the reply is the existing retrieval-failed reply. Stock is trusted from the search hit filter, not re-derived from `Listing.stock` (variants make base stock unreliable).
- Ratings: `rating_text = f"{rating_avg:.1f}/5 ({review_count} đánh giá)"` when `review_count > 0`, else empty. `ProductCard.rating` and `rating_text` defaults (4.8) are removed. `discount_rate` is 0.
- Backends: `ASSISTANT_BACKEND` selects `MockAssistantReplier` (deterministic template over cards, today's wording rebuilt from retrieved facts) or `LLMAssistantReplier` (LLM chain via `build_llm_instance(instance_id="grpc-assistant")`, boot-time check for the `ai` extra like the screen backend). The LLM prompt carries the redacted message and a JSON list of `{title, price, currency, rating_text}`, in data tags. The post-check extracts price numbers and bolded titles from the reply and requires each to match a card; otherwise the template is used and `model_version` is stamped with the `mock` template label, because that is what ran. LLM timeout or error also falls back to the template, labelled `mock`.
- `model_version`: `<backend>:assistant-v1|<search model_version>`. Search model_version is the `SearchListingsResponse.model_version` string (e.g. `hybrid@v1`). On search failure there are no cards and the value is `<backend>:assistant-v1` alone.
- `request_id`: `uuid.uuid4().hex`, minted in the service per call, written to the response and the log line. It is the value the frontend passes as `impressionId` on card impressions.
- Settings added to `AISettingsMixin`: `ASSISTANT_BACKEND`, `ASSISTANT_TIMEOUT_SECONDS` (8.0), `ASSISTANT_SEARCH_TIMEOUT_SECONDS` (0.8), `ASSISTANT_LISTING_TIMEOUT_SECONDS` (0.5), `ASSISTANT_LISTING_ADDR`, `ASSISTANT_LISTING_PRINCIPAL_ID` (`team-ai`), `ASSISTANT_LISTING_PRINCIPAL_SCOPES` (`listing.read`). The search channel reuses `RECS_SEARCH_ADDR` and `RECS_SEARCH_PRINCIPAL_*`. `ASSISTANT_RAG_MIN_SCORE` is removed with its only consumer. These files are hot (shared with `recs-experiment-arms` and `ai-copilot-grounding`); this slice builds after `recs-experiment-arms`.
- gRPC channels are opened in the assistant bootstrap addon and closed on shutdown, like `bootstrap/addons/recommendation.py`.

## Risks

- team-search or team-domain down: the assistant still answers with the retrieval-failed reply; e2e destructive scenario stops team-search.
- Search read-model lag shows a listing domain already unpublished: handled by hydration status check; a listing with stock 0 in domain but still stock>0 in search can show for a short window (accepted, same lag as the PDP grid).
- The `ai-assistant` main spec still has "reply streams token-by-token"; `buyer-ai-surfaces-ui` retires it (U9). Until then the `StreamChat` echo is unaffected server-side.

## Migration

No data migration. Rollout order: contract, then this slice (compose adds the listing address), then the frontend. Old clients ignore the new fields.
