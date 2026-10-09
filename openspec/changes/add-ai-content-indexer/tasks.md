# Tasks

## 1. Code — team-ai
- [x] Implement `app/modules/messaging/indexer/handler.py` (`ListingEventIndexer` mapping listing events to documents).
- [x] Implement `app/modules/messaging/indexer/__init__.py`.
- [x] Write unit tests in `tests/unit/modules/test_listing_indexer.py`.

## 2. Verification
- [x] Validate OpenSpec change (`openspec validate add-ai-content-indexer --strict`).
- [x] Run `pytest -v tests/unit/modules/test_listing_indexer.py` in `team-ai/`.

## 3. Wiring (reconciliation 2026-10-09: the consumer was never started)
- [ ] Kafka consumer (`indexer/consumer.py`, `indexer/decode.py`) with manual commit, retry and DLQ.
- [ ] `ListingIndexerAddon` in the application lifecycle, gated by `LISTING_INDEXER_ENABLED` (+ `RAG_ENABLED`).
- [ ] Idempotency (event_id, per-listing order, replace-on-update) in `ListingEventIndexer`.
- [ ] Re-vendor `events` + `listing` proto; `kafka` extra; settings + `.env.example`.
- [ ] Tests in `tests/unit/modules/test_listing_indexer_consumer.py`.
- [ ] Compose: team-ai needs `UV_EXTRAS` incl. `ai kafka`, `RAG_ENABLED`, `LISTING_INDEXER_ENABLED`, `KAFKA_BROKERS` (integrator).
