## Why

`AIService.ShoppingAssistant` matches a static in-code `CATALOG` and never calls `rag.search`: the listing indexer
(`add-ai-content-indexer`) feeds a RAG store that nothing on the public edge reads (see that change's design). The
assistant therefore recommends products that do not exist and ignores every real listing.

## What Changes

- **team-ai**: when a RAG store is configured, `ShoppingAssistant` retrieves listings from it (`KnowledgeRetrievalService.search`)
  and returns one `product_cards` entry per distinct retrieved listing (`listing_id` = the indexed listing id; title,
  price and currency from the indexed metadata). No proto change: `ProductCardSnippet` already carries all of this.
- Fail-open: if retrieval fails or times out the assistant still answers, with no cards and a reply that says the
  catalog could not be searched. It never falls back to the demo catalog in that case (those ids are not real listings).
- Without a RAG store (`RAG_ENABLED=false`) behaviour is unchanged: the demo catalog answers (local default).
- New setting `ASSISTANT_RAG_MIN_SCORE` (default `0`, off) drops weak matches.
- Deploy/e2e: the compose for team-ai must run the RAG store and the indexer (listed in design.md).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `ai-assistant`: ADDED requirement "Shopping Assistant is grounded in indexed listings".

## Impact

- Code: `team-ai/app/modules/business/ai_assistant/`, `app/core/config/ai.py`, `.env.example`, README.
- Compose: `docker-compose.services.yaml` team-ai env/build args, plus the modelserve overlay (design.md).
- E2E: `platform-e2e` new feature `ai/assistant_grounding.feature`; `team-ai/FEATURES.yaml`.

## Non-goals

- No change to `ChatService.StreamChat` or the LLM-backed chat; no generated (LLM) reply text.
- No proto change, no new events, no image URL in cards (the indexer does not carry one yet).
- No ranking beyond the vector score; no price-constraint parsing against the RAG hits.
