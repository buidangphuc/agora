## Why

`team-ai` defines an ML decoupling seam where `RAG_EMBED_BACKEND=model_server` delegates vectorization over HTTP (`POST {path} {"texts":[…]}` → `{"embeddings":[[…]]}`). Currently, no server implements this contract and `team-ai` falls back to a deterministic `mock` backend.

Furthermore:
1. Retrieval is single-stage (top-k vector only, no reranker), leaving `RAG_DEFAULT_TOP_K` overloaded between recall and prompt token cost.
2. Inference workloads require GPU/VRAM-bound execution, high-throughput continuous batching, multi-GB models, and versioned rollouts distinct from microservice sprint lifecycles.
3. Model routing lacks capacity-driven admission control and Redis embedding caching.

Following **ADR-0011**, this change introduces `platform-modelserve`: an internal platform capability (following the `platform-recsys` precedent) that owns the model serving router, embedding caching, admission control, and proxying to vendor inference runtimes (Hugging Face TEI for embed/rerank, vLLM for generation).

## What Changes

- **platform-modelserve** (new capability):
  - Fast router service running on port `:8100`.
  - Implements `POST /embed` / `POST /v1/embeddings` adhering strictly to `team-ai`'s `_extract_vectors` contract.
  - Implements `POST /rerank` proxying to Hugging Face TEI reranker (`:8102`).
  - Implements `POST /v1/chat/completions` / `POST /generate` proxying to vLLM (`:8103`).
  - Redis vector caching keyed by `hash(model_version + text)` with configurable TTL.
  - Queue-depth tracking and admission control returning `429 Too Many Requests` + `Retry-After` header when overloaded.
  - Full test suite verifying config, env drift, Redis cache, admission control, router endpoints, and contract conformance against `team-ai`.
  - `docker-compose.local.yaml` for local development.
- **platform-gitops**:
  - `platform/infra/modelserve.yaml`: Kubernetes Deployment + Service + NetworkPolicy (limiting ingress to `team-ai`).
  - `argocd/apps/platform-modelserve.yaml`: ArgoCD Application manifest under infra-shared sync wave.
- **platform-core**:
  - `docs/ADR/0011-model-serving.md` documenting the architecture decision.

## Non-goals

- No proto changes or gRPC exposure (`platform-core/packages/proto` untouched) — `platform-modelserve` is an internal HTTP platform capability.
- No public gateway routing (`team-gateway` is untouched) — browsers never call `platform-modelserve`.
- No hand-rolled inference engines or custom PyTorch runtime — inference is delegated to vendor continuous-batching runtimes (TEI / vLLM).
- No real GPU requirements for local dev — CPU fallback and mock/stub runtime options are supported.
