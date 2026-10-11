# Tasks

## 1. Code — team-ai
- [x] Implement `app/modules/messaging/indexer/handler.py` (`ListingEventIndexer` mapping listing events to documents).
- [x] Implement `app/modules/messaging/indexer/__init__.py`.
- [x] Write unit tests in `tests/unit/modules/test_listing_indexer.py`.

## 2. Verification
- [x] Validate OpenSpec change (`openspec validate add-ai-content-indexer --strict`).
- [x] Run `pytest -v tests/unit/modules/test_listing_indexer.py` in `team-ai/`.

## 3. Wiring (reconciliation 2026-10-09: the consumer was never started)
- [x] Kafka consumer (`indexer/consumer.py`, `indexer/decode.py`) with manual commit, retry and DLQ.
- [x] `ListingIndexerAddon` in the application lifecycle, gated by `LISTING_INDEXER_ENABLED` (+ `RAG_ENABLED`).
- [x] Idempotency (event_id, per-listing order, replace-on-update) in `ListingEventIndexer`.
- [x] Re-vendor `events` + `listing` proto; `kafka` extra; settings + `.env.example`.
- [x] Tests in `tests/unit/modules/test_listing_indexer_consumer.py`.

## Evidence (2026-10-10)

- Code and unit tests: each repo's `make check` / test suite was green at merge (see the commit bodies).
- e2e after rebuilding team-ai, team-search (server and indexer), gateway, frontend and the recsys image, with
  platform-recsys-nearline and the modelserve overlay (fake TEI + router) running:
  - ML scenarios: 23/23, twice;
  - modelserve, hybrid and taxonomy: 27/27, three times;
  - placement and serve-trained scenarios: green three times.
- Scenarios that cannot be produced end to end carry a VERIFIED BY line in the spec and a not-testable FEATURES
  entry.
- spec_sync --strict reports e2e-ready.

- Final gate (2026-10-10): parallel lane 775/775 (w10-par) and 775/776 (w9-par; its one failure was the gateway-wide denylist gauge scenario, moved to the serial lane in e8373e00). Destructive lane 100/101 (w9-dfull); its one failure, backpressure, was fixed in 7f4ae454 and 4d325f2f and then passed twice in the outage-then-backpressure order.

## Follow-ups (not done in this change)

- Compose: team-ai needs `UV_EXTRAS` incl. `ai kafka`, `RAG_ENABLED`, `LISTING_INDEXER_ENABLED`, `KAFKA_BROKERS` (integrator).

The local compose leaves the listing indexer off (`LISTING_INDEXER_ENABLED` unset, `UV_EXTRAS` without `kafka`). RAG retrieval is not observable through the edge, because ShoppingAssistant matches a static CATALOG and does not call `rag.search`, so turning the indexer on locally proves nothing end to end yet.
