## Why

The recommendation contract (`platform.recommendation.v1.RecommendationService/Recommend`,
added by **add-recommendation-contract**) and the ALS training pipeline
(**build-als-training-job**, which writes item vectors to a Qdrant collection and
pre-computed per-user Top-N lists to Redis) will exist, but nothing serves the RPC
online. `team-ai` already runs a gRPC transport (SearchService, AIService, ChatService)
over a Qdrant-backed RAG stack (`app/modules/ai/rag`, `app/core/config/ai.py`) and a Redis
client (`app/core/redis.py`) — the same building blocks a recommender needs. This change adds
a `team-ai` **recommend module** that implements `Recommend` online: a two-stage
retrieval→ranking pipeline over the Qdrant collection the training job populates, with a Redis
pre-computed fast path, gated behind a `RECS_ENABLED` flag exactly like `RAG_ENABLED`. No proto
change (the contract is owned by add-recommendation-contract) and no model training here (owned
by build-als-training-job) — this is the online serving half.

## What Changes

- **team-ai** — add a recommend module (`app/modules/business/recommend/` for ranking +
  business rules, reusing `app/modules/ai/rag`'s Qdrant client for candidate retrieval) and a
  thin `RecommendationServicer` (`app/transport/grpc/servicers/recommend.py`) implementing
  `platform.recommendation.v1.RecommendationService/Recommend` — mirroring how `search.py` is a
  thin transport over the RAG machinery. Two-stage: Qdrant ANN Top-100 candidate retrieval →
  ranking + business-rule filtering (in-stock, dedupe, drop the seed listing) → Top-10, with a
  Redis pre-computed-list fast path. Register it in `build_grpc_server`
  (`app/transport/grpc/server.py`) next to the existing servicers.
- **team-ai** — add a `RecommendationSettingsMixin` (`app/core/config/`) with `RECS_ENABLED`,
  the Qdrant collection/backend knobs, candidate/result Top-K, Redis cache key prefix + TTL, and
  the latency budget — following the `RAG_*` settings precedent. When `RECS_ENABLED=false` the
  servicer aborts `UNAVAILABLE` (mirrors SearchServicer's `RAG_ENABLED=false` behavior).
- **team-ai** — expose the recommend module on the gRPC listen port already wired for team-ai
  (`GRPC_PORT`); no new port, no HTTP endpoint.
- **E2E (platform-e2e)** — team-ai has no browser surface of its own; the user-facing scenario is
  owned by **surface-recommendations**. This change only asserts the serving contract via the
  spec scenarios below (backend behavior: gating, cache fast path, filtering) and updates
  `team-ai/FEATURES.yaml` with the recommendation capability entry, `status: planned` until
  surface-recommendations automates the end-to-end flow.

**No proto change.** The RPC and messages are defined solely in
`platform-core/packages/proto` by **add-recommendation-contract** (architecture Rule 4). This
change consumes the generated `recommendation_pb2*` stubs; it must not hand-edit generated code.

## Non-goals

- **Model training** — computing ALS item/user vectors and the pre-computed Top-N lists is
  **build-als-training-job** (Spark). This change only reads what that job writes.
- **Real LLM reranking** — ranking here is a lightweight scoring pass over ANN candidates, not an
  LLM.
- **Per-user online learning** — no online model updates from live events; the model is refreshed
  only by the offline training job.
- **The proto contract** — owned by add-recommendation-contract.
- **Gateway forwarder / frontend UI** — owned by surface-recommendations.
