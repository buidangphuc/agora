## Context

See proposal.md. `AIAssistantService` already receives the RAG service (`rag_service`) from the gRPC server and the
HTTP dependency but never uses it. The indexer stores one document per listing id (chunked), metadata `listing_id`,
`title`, `category`, `price`, `currency`, `seller_id`, `status`, and replaces/deletes by listing id. `ProductCardSnippet`
(`listing_id`, `title`, `price`, `currency`, `image_url`, `discount_rate`, `rating_text`) needs no proto change.

## Decisions

- **Retrieval, not generation.** `shopping_assistant` calls `rag_service.search(message, top_k=top_k * 3)` (over-fetch,
  because several chunks of one listing consume slots), groups hits by `document_id`/`listing_id`, keeps the best score
  per listing, drops hits below `ASSISTANT_RAG_MIN_SCORE`, and keeps `top_k`. The reply text is the existing template with
  the grounded facts (title, price) only: no rating or discount claims for real listings (`rating_text` and
  `discount_rate` stay empty/0 since the index has neither).
- **Fail-open without fabrication.** Any exception or timeout from `search` is logged (`shopping_assistant.rag_failed`)
  and yields an empty card list with a reply stating the catalog could not be searched right now. The demo catalog is
  used only when `rag_service is None` (RAG disabled), because its ids are not real listings and must not appear when a
  real index exists but is unreachable.
- **Relevance floor.** A vector store always returns its nearest `k`, however unrelated. `ASSISTANT_RAG_MIN_SCORE`
  (default 0, off: scores are model specific) lets a deployment drop weak hits. The e2e overlay leaves it unset: the
  indexed text embeds the listing metadata too, so with the fake TEI a unique-word query scores about 0.3 or less against
  its listing; the scenarios rely on rank (the unique word makes the listing the nearest hit), not on a floor.
- **Why the redaction still holds.** The RAG service redacts text and metadata on index; cards read that metadata.

## Why two scenarios are not end-to-end tests

Chunk-level behaviour and scores are not exposed by the gateway, and the stack runs with a RAG store, so the
RAG-disabled path cannot be reached. Both have `VERIFIED BY` lines and `not-testable` FEATURES entries.

## Deployment needs (team-ai, local compose)

Image build: `UV_EXTRAS: "ai kafka recs"` (llama-index + qdrant store live in `ai`, aiokafka in `kafka`, the recs reader in
`recs`). The `kafka` extra also carries `cramjam`: the Go producers compress `listing.events` with snappy and
aiokafka cannot decode it without. Qdrant specifics: the store needs both a sync and an async client, point ids must be
UUIDs (chunk ids are UUIDv5 of `<listing>:chunk:<n>`), and a collection that does not exist yet (nothing indexed) is an
empty result, not an error.

team-ai environment:

| Variable | Value | Why |
|---|---|---|
| `RAG_ENABLED` | `true` | opens the RAG store |
| `RAG_BACKEND` | `qdrant` | survives a team-ai restart (the memory store would be empty while the consumer group has already committed) |
| `RAG_QDRANT_URL` | `http://qdrant:6333` | |
| `RAG_QDRANT_COLLECTION` | `rag_documents` | not `item_als_vectors` (recsys) |
| `RAG_EMBED_BACKEND` | `model_server` | |
| `RAG_EMBED_SERVER_URL` | `http://modelserve-router:8100` | the modelserve overlay router |
| `RAG_EMBED_SERVER_PATH` | `/embed` (default) | confirm the router path serves `{"texts": [...]}` |
| `RAG_EMBED_DIM` | `384` (default) | the TEI fake dimension |
| `LISTING_INDEXER_ENABLED` | `true` | |
| `KAFKA_BROKERS` | `redpanda:9092` | |
| `ASSISTANT_RAG_MIN_SCORE` | unset (0) | optional floor; the e2e scenarios rank by nearest hit |

and `depends_on: modelserve-router` (healthy) in the overlay. The e2e scenarios need the modelserve overlay
(`platform-e2e/compose/modelserve.override.yaml`) plus a team-ai entry in it carrying the table above (task 3.1), tagged
`@needsModelserveOverlay`.

## Risks / Trade-offs

- Listings published before the indexer was enabled are not in the store until `listing.events` is replayed (the group
  starts at `earliest`, so a first start backfills) → documented in the README.
- With a real embedding model, scores differ from the fake → `ASSISTANT_RAG_MIN_SCORE` is per deployment.
