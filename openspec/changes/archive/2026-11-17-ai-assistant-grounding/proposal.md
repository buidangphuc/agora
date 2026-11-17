## Why

`/assistant` answers from team-ai's own Qdrant RAG (`RAG_ENABLED`) or, when RAG is off, from `DemoCatalogRetriever`, whose cards carry invented ratings ("4.9/5 (1.2k đánh giá)"). The reply is a fixed template that never checks stock or price filters, and the query log prints the raw `user_id` (`shopping.py:32`). A buyer can be shown a product that is out of stock, over budget or not real. This slice grounds the assistant in live catalog data: team-search `SearchListings` for retrieval, team-domain `ListingService.GetListing` for hydration, and a reply that cites only the retrieved cards. Every response also carries `request_id` and `model_version`, so AI events can be tied to the response that produced them.

initiative: docs-ai-data

## What Changes

- New retrieval path in `team-ai/app/modules/business/ai_assistant`: a `CatalogGrounding` component calls `SearchService.SearchListings` (hybrid, `filters.in_stock=true`, `max_price` parsed from the message, top 5) then `ListingService.GetListing` for every hit in parallel. A hit whose listing is not `PUBLISHED`, or whose lookup fails, is dropped. Cards are built from the hydrated listing (title, price, currency, first image) and the hit (`rating_avg`, `review_count`).
- Ratings: `rating_text` is built only from `rating_avg` and `review_count`; empty when `review_count` is 0. `discount_rate` is 0 (no discount fact exists in these sources).
- New gRPC adapters in team-ai, like `recommendation/adapters/search_stock.py`: a search client (service principal, scope `search:read`, address `RECS_SEARCH_ADDR`) and a listing client (service principal, scope `listing.read`, new `ASSISTANT_LISTING_ADDR`).
- New selector `ASSISTANT_BACKEND` = `mock|llm_router`, default `mock`, mirroring `SCREEN_BACKEND`. `mock` is a deterministic template over the retrieved cards. `llm_router` writes the reply through the existing LLM chain from the cards only, and a post-check falls back to the template if the reply names a price or title not in the cards.
- `ShoppingAssistantResponse` gains `request_id` (UUID hex minted per call) and `model_version` (`<backend>:<prompt_version>|<search model_version>`, search part omitted when search failed), fields 4 and 5 from `docs-ai-data-contract`.
- Deleted: `DemoCatalogRetriever`, `catalog.py`, `RagProductRetriever` use in the assistant path and `ASSISTANT_RAG_MIN_SCORE`. team-ai's own RAG (`RAG_ENABLED`, `SearchListings` servicer, indexer) stays; removing it is a separate one-line follow-up.
- The query log stops printing `user_id`; it logs `request_id`, the redacted message and counts only.
- `docker-compose.services.yaml`: team-ai gets `ASSISTANT_LISTING_ADDR=team-domain-svc:50051`, `ASSISTANT_BACKEND=mock`, `depends_on` team-search and team-domain started.
- Evals: a groundedness set in `team-ai/evals` (reply prices and titles are a subset of the cards) gated by `run_eval --min-score 0.8`.

## Capabilities

### New Capabilities
- `ai-assistant-grounding`: the shopping assistant's cards, reply, stock and price filters, ratings, provenance and failure behaviour.

### Modified Capabilities
none (the streaming requirement of `ai-assistant` is retired by `buyer-ai-surfaces-ui`, which stops calling `StreamChat`; this slice does not touch it).

## Impact

- team-ai: `ai_assistant/{shopping,factory,retrieval,schemas}.py`, new `grounding.py`, new `adapters`, `core/config/ai.py` (hot file), `bootstrap/services.py` (hot file), `transport/grpc/servicers/ai.py` (hot file), `evals/`, `FEATURES.yaml` (by the e2e author).
- `docker-compose.services.yaml` (hot file): team-ai env block only.
- platform-e2e: `features/ai/assistant.feature` and `assistant_grounding.feature` (the RAG-indexer and "demo catalog" scenarios are replaced).
- No proto edit here: consumes the fields from `docs-ai-data-contract`. The card impression `placement_id=assistant.suggestions` is emitted by the frontend (`buyer-ai-surfaces-ui`) from the returned `request_id` and `model_version`.

## Non-goals

Streaming the grounded answer (decision U9); order-status questions; the copilot, Magic Listing and review summary (slice `ai-copilot-grounding`); removing team-ai's own RAG; photo-aware features (U1); a new ranking of cards beyond team-search order.

## Non-functional requirements

- Latency: assistant p95 at most 1.5 s with `ASSISTANT_BACKEND=mock` (one search call plus at most 5 parallel `GetListing` calls). Timeouts: search 800 ms, each `GetListing` 500 ms, `llm_router` generation 8 s (`ASSISTANT_TIMEOUT_SECONDS`), after which the template is used.
- Availability: the assistant never returns an RPC error because retrieval or the LLM failed; it degrades to the "chưa tra cứu được" reply with no cards (retrieval) or to the template (LLM). It does not fail on a search or domain outage.
- Limits: at most 5 cards; message length stays bounded by the existing request limits; existing per-principal rate limit and AI quota unchanged.

## Data classification

| field | personal data | purpose | consent basis | retention |
|---|---|---|---|---|
| `ShoppingAssistantRequest.message` | possible (free text) | build the search query and the reply | service delivery, buyer-initiated request | not stored; logged only after `RedactionPolicy` redaction |
| `ShoppingAssistantRequest.user_id` | yes (buyer id) | rate limit and quota by principal only | service delivery | never logged, never sent to search, domain or the LLM |
| `previous_context` | possible (free text) | conversation continuity | service delivery | not stored; redacted before any prompt |
| Search and listing calls | no buyer data: query text (redacted) and listing ids only | retrieval | not applicable | not stored by team-ai |
| `request_id`, `model_version` | no: random UUID and a model label | link AI events to responses | not applicable | carried on events, retention as the warehouse |
| LLM prompt (`llm_router` only) | no buyer id; redacted message plus catalog facts | write the reply | service delivery | provider and Langfuse rules unchanged |

## Threat model

- Assets: buyer trust in prices and availability shown by the assistant; buyer free text; seller listing content.
- Abuse cases: (1) prompt injection through a listing title or the buyer message telling the model to invent prices or links; (2) a draft, banned or out-of-stock listing surfaced as a recommendation; (3) the assistant used to amplify load on search and domain (fan-out of 6 calls per request); (4) a buyer id leaking through logs or the prompt.
- Mitigations: cards come from data, not the model; only title, price, rating and stock reach the prompt (never descriptions), wrapped in data tags with an instruction to ignore embedded commands; the reply post-check rejects any price or title outside the cards; non-`PUBLISHED` listings are dropped; fan-out is capped at 5 `GetListing` calls with deadlines and the existing per-principal rate limit and quota apply; `user_id` is never logged or forwarded.
- Authorisation rule: unchanged. `ShoppingAssistant` still requires the shopper scopes (`ai.use` when enforced) and the gateway route stays authenticated. team-ai reaches search and domain only with its own service principal (`search:read`, `listing.read`), never with the buyer's identity.
