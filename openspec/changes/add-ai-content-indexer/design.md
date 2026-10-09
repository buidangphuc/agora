## Context

The change was ticked done with a handler and unit tests, but nothing consumed Kafka: `team-ai` had no Kafka
client and `ListingEventIndexer` was not hooked to anything. This design records the wiring built to make the
requirement true. Test path in the proposal (`tests/test_listing_indexer.py`) is stale: the tests are
`tests/unit/modules/test_listing_indexer.py` (handler) and `tests/unit/modules/test_listing_indexer_consumer.py`
(consumer, decode, idempotency, failure handling, lifecycle).

## Decisions

- **Where it runs.** A `ListingIndexerAddon` (bootstrap addon, after `RagAddon`) runs the consumer as a supervised
  background task of the API process, gated by `LISTING_INDEXER_ENABLED` and requiring `RAG_ENABLED` (startup fails
  otherwise). A separate process would need its own RAG store handle; with `RAG_BACKEND=memory` it would not share
  the index, so one process it is. If the task dies it is rebuilt after 5 s.
- **Client.** `aiokafka`, in the optional `kafka` extra (image build arg `UV_EXTRAS` must include `kafka`).
- **Wire format.** `EventEnvelope` (ADR-0002); `type == platform.listing.v1.ListingChanged` is consumed, every other
  listing event type is committed and ignored. `proto/` is re-vendored from platform-core (events + current listing).
- **Delivery and idempotency.** Manual commit after the record is applied or parked. Group `team-ai-indexer`,
  `earliest` on first start. Applying an event is idempotent: update = delete then index (replace), delete of an unknown
  id is a no-op, an already applied `event_id` is skipped, and an event older than the last one applied to that listing
  (envelope `occurred_at`) is skipped. The bookkeeping is bounded in-memory (restart loses it; replaying in order still
  converges).
- **Failure handling.** Same shape as team-search AD1: up to `LISTING_INDEXER_MAX_ATTEMPTS` (5) with doubling backoff,
  then the record is produced to `listing.events.dlq` and the offset advances. An undecodable record goes to the DLQ
  without retries. If the DLQ write fails the consumer stops without committing, so the record is redelivered after the
  supervisor restarts it.
- **"Active" listings.** The proposal says active listings are indexed. A created/updated listing whose status is
  `DRAFT` or `REJECTED` is removed from RAG instead of indexed (team-search filters by status at query time, RAG
  retrieval has no such filter). `PUBLISHED` and unset are indexed.

## Why the scenarios are not end-to-end tests

Re-checked 2026-10-09 against the code: nothing reachable through the gateway reads the RAG store.
- `AIService.ShoppingAssistant` is routed to team-ai, but `AIAssistantService.shopping_assistant` matches against a static
  in-code `CATALOG` (`_match_products`); `rag_service` is stored on the object and never called, and the response
  (`reply_text`, `product_cards`, `suggested_followups`) has no sources or citations field.
- `ChatService.StreamChat` goes to the LLM router with no retrieval step. The `llm-fake` overlay records the chat requests the
  provider receives (`GET /_requests`), but no retrieved context is ever placed in them, so there is nothing to observe.
- `SearchService.SearchListings` is the only caller of `rag.search`, and the gateway routes it to team-search (OpenSearch), not
  team-ai.
The only observation of the index is the vector store itself, which is not the public edge. The scenarios therefore keep their
`VERIFIED BY` lines (real `KnowledgeRetrievalService`, mock embeddings, scripted Kafka records). They become end-to-end testable
when a gateway-reachable path retrieves from RAG (for example `ShoppingAssistant` calling `rag.search` and returning the matched
listing ids as `product_cards`); that is a product change outside this change.

## Deployment needs

`UV_EXTRAS` must include `ai kafka` (llama-index lives in `ai`), and the service needs `RAG_ENABLED=true`,
`LISTING_INDEXER_ENABLED=true`, `KAFKA_BROKERS=redpanda:9092` (plus the RAG backend/embedding settings).
