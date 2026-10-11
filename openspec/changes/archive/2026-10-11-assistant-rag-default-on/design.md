## Context

Today only `platform-e2e/compose/modelserve.override.yaml` carries the team-ai RAG environment and the
`modelserve-router`/`tei-fake` services (see archived `assistant-rag-grounding`, "Deployment needs"). The base team-ai
service builds with `UV_EXTRAS: recs` and has no RAG settings.

## Decisions

- **Move the embedding path into the base compose.** Needed services: `tei-fake` (stdlib Python, dim 384, no model
  download, builds from `platform-e2e/fakes/tei_fake`) and `modelserve-router` (real `platform-modelserve` router,
  Redis DB 4 cache). Both are small; unreasonable would be a real TEI image (hundreds of MB + model weights), which we
  do not add. The fake is a documented local stand-in; replacing `TEI_*_URL` on the router switches to real TEI.
  Cross-directory build context (`./platform-e2e/fakes/...`) is acceptable: the repo is one monorepo checkout.
- **team-ai env in base**, same values as the archived design table: `RAG_ENABLED=true`, `RAG_BACKEND=qdrant`
  (survives restarts), `RAG_QDRANT_URL=http://qdrant:6333`, `RAG_QDRANT_COLLECTION=rag_documents`,
  `RAG_EMBED_BACKEND=model_server`, `RAG_EMBED_SERVER_URL=http://modelserve-router:8100`, `RAG_EMBED_DIM=384`,
  `LISTING_INDEXER_ENABLED=true`, `KAFKA_BROKERS=redpanda:9092`; build extras `"recs ai kafka"`. No secrets are
  involved (no key, token or password is added).
- **depends_on** `modelserve-router` (healthy), `qdrant` (healthy), `redpanda` (healthy), so the first boot finds its
  stores. A failing embedding backend later is still fail-open (assistant answers without cards).
- **Test-only settings stay in the overlay.** Published host ports (`18100`, `18110`), `agora-*` image names (the
  agora isolation convention) and `MAX_QUEUE_DEPTH=10` (the backpressure scenario) remain in
  `modelserve.override.yaml`; compose merges `ports` and `environment` with the base. Base publishes no ports.
- **RAG-off variant: an overlay, not an env var in `.env`.** `platform-e2e/compose/rag-off.override.yaml` sets
  `RAG_ENABLED=false` and `LISTING_INDEXER_ENABLED=false`. The demo-catalog scenario applies it with
  `up -d --no-deps team-ai` and restores the standing config in teardown, exactly like `ai-path-resilience`'s Langfuse
  variant. Scenario becomes e2e-covered (`@destructive`), the unit test stays.
- **e2e wiring.** The stack wrapper (`dc-agora-ov.sh`, outside the repo) keeps listing `modelserve.override.yaml`, which now
  only layers on the base services, so no wrapper change is needed; `rag-off.override.yaml` is passed by the scenario.
  Anyone composing the base stack without the overlay gets the grounded assistant with the in-network services.
- **Side effect, accepted.** `SearchListings` (team-ai) is now served in the base stack (it was `UNAVAILABLE`). It is
  scope-gated and routed internally only (no public route), so no new exposure.

## Risks / Trade-offs

- Slower first `up` (two more images, heavier team-ai image) and one more start-order dependency chain for team-ai.
- team-ai now needs Kafka reachable to index; indexer failures park on `listing.events.dlq` and do not affect serving.
- Embedding quality is the fake's (bag of concepts): good for plumbing and demos, not relevance. Documented in the README.
