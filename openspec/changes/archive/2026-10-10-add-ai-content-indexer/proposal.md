## Why

Currently `team-ai`'s RAG knowledge store relies on static or mocked documents. When listings are created, updated, or deleted in `team-domain`, `team-ai` does not index them in real-time, leaving AI RAG and semantic search out of date.

Following **P2-T3**, this change introduces a real-time Kafka `listing.events` consumer in `team-ai` that indexes listing content into Qdrant/RAG via `platform-modelserve` embeddings within seconds of change publication.

## What Changes

- **team-ai** (`app/modules/messaging/indexer/`):
  - `ListingEventIndexer`: Consumer and handler for `platform.listing.v1.ListingChanged` events.
  - Formats listing metadata (title, description, category, price, attributes) into searchable RAG documents.
  - Automatically vectorizes and indexes active listings into `KnowledgeRetrievalService` / Qdrant.
  - Deletes/archives listings when `ListingChanged` indicates deletion or unpublishing.
  - Unit and integration tests in `team-ai/tests/test_listing_indexer.py`.

## Non-goals

- No direct SQL queries to `team-domain`'s database (Rule 3) — all updates are driven strictly through Kafka `listing.events`.
