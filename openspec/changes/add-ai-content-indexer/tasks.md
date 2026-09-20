# Tasks

## 1. Code — team-ai
- [x] Implement `app/modules/messaging/indexer/handler.py` (`ListingEventIndexer` mapping listing events to documents).
- [x] Implement `app/modules/messaging/indexer/__init__.py`.
- [x] Write unit tests in `tests/unit/modules/test_listing_indexer.py`.

## 2. Verification
- [x] Validate OpenSpec change (`openspec validate add-ai-content-indexer --strict`).
- [x] Run `pytest -v tests/unit/modules/test_listing_indexer.py` in `team-ai/`.
