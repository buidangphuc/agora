# team-ai

The AI service of the agora platform. It serves the platform-core gRPC contract on `GRPC_PORT` (production `50060`) and a small FastAPI surface on `:8000`.

## Scope

- **gRPC** (always on once the app opens its resources): `AIService` (shopping assistant, magic listing, chat copilot, `ClassifyTags`, `ScreenListing`), `ChatService` (`StreamChat`), `RecommendationService` (`Recommend`) and `SearchService` (`SearchListings` only).
- The chat copilot is rules-based by default (`COPILOT_BACKEND=mock`); `llm_router` (needs the `ai` extra) drafts with the LLM within `COPILOT_TIMEOUT_SECONDS` and falls back to the rules when the output is unusable, contradicts the listing facts or promises a delivery time.
- **HTTP**: `/api/v1/ai/*` (assistant and tag routes), `/api/v1/completions` (sync and SSE), `/healthz`, `/readyz` and `/metrics` (`teamai_grpc_requests_total`, which KEDA scales on).
- **Always on**: Redis, recommendations (Qdrant), the per-principal rate limit (Redis), the chat reply quota (Redis), taxonomy persistence (Redis database `TAXONOMY_REDIS_DATABASE`), gRPC reflection, graceful drain and the request timeout. Settings fail fast without `REDIS_HOST`.
- **Listing screen**: the `ScreenListing` RPC screens a listing for trust enforcement. `SCREEN_BACKEND` selects the backend (`mock` by default, or `llm_router`). The Kafka listing indexer indexes only listings in the `PUBLISHED` state (and an empty status from legacy producers).
- **Four switches**: `CHAT_BACKEND` (`mock` or `llm_router`), `RAG_ENABLED` (Qdrant, model_server embeddings and the Kafka listing indexer), `AI_USE_SCOPE_REQUIRED` and `DOCS_ENABLED`. OpenTelemetry starts when `OTEL_EXPORTER_OTLP_ENDPOINT` is set, Langfuse when both Langfuse keys are set.
- API-key Bearer auth for HTTP, gateway-forwarded principal metadata for gRPC, and the app shell: request IDs, error envelope, graceful drain.

Deployment manifests live in `platform-gitops`, not here.

## Project layout

```text
app/
  api/                  Versioned FastAPI routers (ai, completions, health)
  bootstrap/            App factory, platform resources, service composition, gRPC startup
  core/                 Settings, Redis, errors, logging, health, middleware, gRPC metrics
  modules/
    ai/                 llm/ (LangChain + Langfuse), rag/ (LlamaIndex), evals/
    business/           ai_assistant/, chat/, completions/, listing_screen/, recommendation/, tag_classifier/
    messaging/          kafka/, listing_indexer/
    platform/           identity/, rate_limit/, quota/, model_server/
  transport/grpc/       Servicers, interceptors, error mapping, scopes
proto/                  Vendored platform-core proto contract
docs/                   usage.md, adr/, agents/
evals/                  Eval cases for the eval gate
ops/
  compose/              Docker Compose stacks: local.yaml, langfuse.yaml, langfuse.env.example
  mlops/                MLflow local env example
  observability/        OpenTelemetry collector debug config
scripts/                gen_proto, check_env_example, run_eval, hash_api_key, seed_flipt_flags, smoke runners
tests/                  Unit and integration tests
```

## Quickstart

```bash
cp .env.example .env
uv sync --dev --all-extras
make test
make dev
curl http://localhost:8000/healthz
```

`make dev` needs a Redis at `REDIS_HOST` and a Qdrant at `RECS_QDRANT_URL`. The compose stack in `ops/compose/local.yaml` brings up the API and Redis. For Docker, configuration, auth and gRPC, see [docs/usage.md](docs/usage.md).

Placements with `in_stock_only` read live stock from team-search (`SearchService.GetListingStock`) at `RECS_SEARCH_ADDR` (default `localhost:50052`), as the service principal `RECS_SEARCH_PRINCIPAL_ID` (default `team-ai`) with the comma-separated scopes `RECS_SEARCH_PRINCIPAL_SCOPES` (default `search:read`), within `RECS_RETRIEVE_TIMEOUT_MS`. The first `limit` x 2 ranked ids (at most 20, at most 10 per call) are looked up and refilled from the next ranked ids; an absent stock drops only that listing, and a failed, rejected, slow or malformed lookup returns an empty placement.

A placement with an `experiment.flag` in `placements.yaml` (`home_feed`: `recs-home-feed-ranker`) asks Flipt at `RECS_FLIPT_URL` (empty = off) for the requester's arm within `RECS_FLIPT_TIMEOUT_MS` (default 50), sending the user or anonymous id only as a SHA-256 hash salted with `RECS_EXPERIMENT_SALT` (required with `RECS_FLIPT_URL` outside dev/local/test). A variant attachment `{"ranking": {"model": "cosine_rank" | "gbdt"}}` overrides the placement's ranker; any other attachment is ignored, and a Flipt failure serves the default ranker. `Recommend` returns `model_version` as `<model_version>/<tier>/<ranker>/<arm>` (tier the candidate-ladder strategy that served, `popular` for `global_popular` and the floor, or `serving-fallback`; ranker `similarity`, `fixed_weights` or `gbdt`; arm `none` or `<flag>:<variant>`, `<flag>:unassigned` on a Flipt failure), and the served log line carries `tier`, `ranker` and `arm`. `python -m scripts.seed_flipt_flags --url http://localhost:8080` creates the flag disabled with `control` and `similarity` variants and never touches an existing flag.

## Documentation

- [docs/usage.md](docs/usage.md): requirements, quickstart, services and routes, Docker, commands, env defaults, bearer auth.
- [docs/agents/project.md](docs/agents/project.md): feedback loops, checks, and an index of the architecture and AI-engineering docs.
- [docs/adr/](docs/adr/): architecture decision records.
- [../openspec/](../openspec/): specs and change proposals (repo root).
- [ops/compose/local.yaml](ops/compose/local.yaml) and [ops/compose/langfuse.yaml](ops/compose/langfuse.yaml): local Docker stacks.
