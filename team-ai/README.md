# team-ai

AI serving service for the Agora marketplace. Python 3.12, FastAPI (HTTP) plus a `grpc.aio`
server that implements the platform-core contract. It is a deployed service (compose service
`team-ai`, container `team-ai-svc`).

**Bounded context / what it owns**

- The AI surface behind the gateway: shopping assistant, magic listing, chat copilot, review
  summary (`AIService`), RAG search (`SearchService`), streaming chat (`ChatService`) and
  recommendations (`RecommendationService`).
- The SPU/SKU tag classifier (HTTP only, in-memory state).
- It owns no business data. Its only database tables are platform plumbing (see Data), and the
  compose stack runs with `DATABASE_ENABLED=false`.

Most answer logic is deterministic. `ShoppingAssistant` matches against a catalog hard-coded in
`app/modules/business/ai_assistant/service.py` (it never calls the injected RAG service), and
`MagicListing`, `ChatCopilot` and `SummarizeReviews` are rule-based. An LLM or an embedding
server is only reached through opt-in backends (`CHAT_BACKEND=llm_router`,
`RAG_EMBED_BACKEND=model_server`).

## 1. Contract

Ports: HTTP `8000` (compose publishes `8001:8000`). gRPC is off unless `GRPC_ENABLED=true`; the
code default port is `50051`, compose sets `50060` (`50060:50060`). The gateway reaches it at
`team-ai-svc:50060` for `AIService` and `RecommendationService`.

### gRPC (`app/transport/grpc/servicers/`)

| Service / RPC | Authorization | Notes |
|---|---|---|
| `platform.ai.v1.AIService` `ClassifyTags` | scope `ai.classify` AND a `service` principal (team-search's indexer); no user role holds the scope; not routed by the gateway | SPU tags plus per-variant tags with facet group and confidence, from the same in-process tag registry as the REST routes. `INVALID_ARGUMENT` for a title under 2 characters. |
| `platform.ai.v1.AIService` `ShoppingAssistant`, `MagicListing`, `ChatCopilot`, `SummarizeReviews` | none beyond a resolved principal (no scope gate) | Deterministic. Any exception becomes `INTERNAL`. |
| `platform.search.v1.SearchService.SearchListings` | scope `search:read` | `UNAVAILABLE` when `RAG_ENABLED=false`. |
| `platform.chat.v1.ChatService.StreamChat` (server stream) | none beyond a resolved principal | `CHAT_BACKEND=mock` echoes the prompt; `llm_router` streams from the LLM router. Final chunk has `done=true`. |
| `platform.recommendation.v1.RecommendationService.Recommend` | scope `listing.read` | `UNAVAILABLE` when `RECS_ENABLED=false` or the Qdrant collection contract mismatches. A cache/Qdrant error answers `OK` with the popular list (or empty) and `model_version` `serving-fallback`. Every response carries `placement_id` and a fresh `request_id` (one `recs.served` log line each). `RECS_BACKEND=memory` is refused at boot outside dev/local/test. |
| `grpc.health.v1.Health`, server reflection | exempt from auth | Health always answers SERVING. Reflection is on unless `GRPC_REFLECTION_ENABLED=false`. |

Auth (`interceptors/auth.py`): if `x-principal-id` is present, the `x-principal-{id,type,scopes}`
metadata forwarded by team-gateway is trusted as is. Otherwise an `authorization: bearer` token
is checked against `AUTH_BEARER_TOKEN`.

### HTTP (`app/api/`)

| Route | Auth |
|---|---|
| `GET /healthz`, `GET /readyz` (also under `/api/v1`), `GET /metrics` (gRPC request counter) | none |
| `POST /api/v1/ai/assistant`, `/magic-listing`, `/chat-copilot` | none |
| `POST /api/v1/ai/tags/classify`, `/tags/classify-sku-hierarchy` (no longer used by team-search's indexer, which calls gRPC `ClassifyTags`), `/tags/explore`, `/tags/promote`; `GET /api/v1/ai/tags` | none |
| `POST /api/v1/completions`, `/completions/stream`, `/completions/tasks`; `GET /completions/tasks/{task_id}` | `require_principal` (bearer) |

`/docs` is served while `DOCS_ENABLED=true` (default). `/readyz` reports postgres, redis and
mongo checks only for the resources that are enabled.

### Consumes

| What | From | When |
|---|---|---|
| Qdrant collection `item_als_vectors` (name, dim 64, Cosine must match) | `platform-recsys` ALS job | `RECS_BACKEND=qdrant` |
| Redis keys `recs:v1:user:<id>`, `recs:v1:popular`, `recs:v1:model_version` | `platform-recsys` ALS job | `RECS_ENABLED=true` with Redis |
| Qdrant collection `RAG_QDRANT_COLLECTION` | its own RAG store | `RAG_ENABLED=true`, `RAG_BACKEND=qdrant` |
| `POST {RAG_EMBED_SERVER_URL}{RAG_EMBED_SERVER_PATH}` with `{"texts": [...]}` | embedding server (for example platform-modelserve) | `RAG_EMBED_BACKEND=model_server` |
| LLM via LangChain (`CHAT_MODEL`, `CHAT_FALLBACK_MODELS`) | external provider | `CHAT_BACKEND=llm_router`, `ai` extra |

Recommendation placements (`home_feed`, `similar_items`, `cart_cross_sell`) are defined in
`app/modules/business/recommend/config/placements.yaml` (platform ADR-0012). The ladder for
`home_feed` is precomputed user list, then seed-vector ANN, then category popular, then global
popular; the other placements are configured in the same file.

## 2. Events

None. There is no Kafka or RabbitMQ producer or consumer wired. The `outbox`, `queue`, `tasks` and
`webhooks` modules exist but are disabled by default, and `ListingEventIndexer`
(`app/modules/messaging/indexer`) is not hooked to any consumer.

## 3. Data

- Postgres is optional (`DATABASE_ENABLED`, default `false`; compose keeps it off). Alembic
  migrations in `alembic/versions/` create `audit_events`, `idempotency_keys`, `tasks`,
  `outbox_events`, `quota_counters` and `quota_reservations` (platform plumbing, no domain tables).
- Apply with `make migrate` (`alembic upgrade head`). The Docker image copies `alembic/` but its
  `CMD` does not run migrations, and the root compose has no migrate job for team-ai.
- Tag classifier state (canonical and candidate tags) is in process memory and is lost on
  restart. The recommendation feature and nearline stores are in-memory too.
- Redis (`REDIS_ENABLED`) holds the recs lists it reads, plus rate-limit or cache data if those
  features are enabled.

## 4. Configuration

All settings are in `app/core/config/*.py`, loaded from the environment or `.env`
(`extra="forbid"`: unknown keys in `.env` fail startup). Only the service-specific ones are
listed. The platform plumbing flags (`RATE_LIMIT_*`, `CACHE_*`, `QUEUE_*`, `TASKS_*`, `OUTBOX_*`,
`WEBHOOKS_*`, `OBJECT*`, `MONGO*`, `IDEMPOTENCY_*`, `QUOTA_*`, CORS and timeouts) are all off by
default and listed in `.env.example`.

| Variable | Default | Meaning |
|---|---|---|
| `ENVIRONMENT` | `dev` | `prod` or `production` rejects `DOCS_ENABLED=true`, wildcard CORS or hosts, and a weak `AUTH_BEARER_TOKEN`. |
| `AUTH_BEARER_TOKEN` | `""` | Bearer fallback for HTTP and direct gRPC calls. Required outside dev, local and test. |
| `GRPC_ENABLED` | `false` | Start gRPC inside the HTTP app lifespan. |
| `GRPC_HOST`, `GRPC_PORT` | `0.0.0.0`, `50051` | Bind address. Compose uses `50060`. |
| `GRPC_REFLECTION_ENABLED`, `GRPC_GRACE_SECONDS` | `true`, `10.0` | |
| `DATABASE_ENABLED` | `false` | |
| `POSTGRES_HOST`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | none (required) | No default: must be set even when the DB is off. |
| `REDIS_ENABLED` | `false` | |
| `REDIS_HOST` | none (required) | No default. |
| `CHAT_BACKEND` | `mock` | `mock` or `llm_router`. |
| `CHAT_MODEL`, `CHAT_FALLBACK_MODELS`, `JUDGE_CHAT_MODEL` | `""` | LangChain model ids such as `openai:gpt-4.1-mini`. Empty `CHAT_MODEL` means the local fake model. |
| `LLM_FIRST_TOKEN_TIMEOUT_SECONDS`, `LLM_MAX_ATTEMPTS` | `8.0`, `3` | `llm_router`: each attempt is cancelled when no chunk arrives in time; `LLM_MAX_ATTEMPTS` bounds all pre-first-chunk attempts across the chain (the same target is retried only in a single-target chain). The gRPC deadline caps every wait. |
| `LLM_BREAKER_THRESHOLD`, `LLM_BREAKER_COOLDOWN_SECONDS` | `3`, `30.0` | Per-target breaker: consecutive 429/5xx/timeout/connection failures open it; after the cooldown one probe request is let through. Request-caused 4xx never count. |
| `CHAT_SYSTEM_PROMPT` | `""` | System prompt used when Langfuse is off or unreachable; empty means the built-in prompt. |
| `CHAT_HISTORY_MAX_TURNS`, `CHAT_HISTORY_MAX_TOKENS`, `CHAT_HISTORY_TTL_SECONDS` | `10`, `1500`, `1800` | Earlier exchanges of the same `(principal, session_id)` sent to the model (a turn = one user+assistant exchange; tokens estimated as chars/4). Redis when `REDIS_ENABLED`, else per process. |
| `LLM_TRACE_CONTENT` | `redacted` | User text in logs and Langfuse traces: `redacted`, `off` or `full` (`full` refused outside dev/local/test). Text sent to the model is always redacted. |
| `QUOTA_ENABLED`, `QUOTA_BACKEND` | `false`, `memory` | With `llm_router`, one `chat.reply` unit is reserved per reply (server-minted id), finalized after delivery and refunded when nothing was delivered. |
| `QUOTA_CHAT_REPLIES_PER_WINDOW`, `QUOTA_CHAT_WINDOW_SECONDS` | `200`, `86400` | Chat reply quota per principal. |
| `GRPC_RATE_LIMIT_ENABLED` | `false` | Per-principal limit on `StreamChat` and `ShoppingAssistant` (`RATE_LIMIT_*`). Refused with `RATE_LIMIT_BACKEND=memory` outside dev/local/test. |
| `RAG_ENABLED` | `false` | Gates `SearchListings`. |
| `RAG_BACKEND` | `memory` | `memory` or `qdrant`. |
| `RAG_QDRANT_URL`, `RAG_QDRANT_COLLECTION` | `http://localhost:6333`, `rag_documents` | Used when `RAG_BACKEND=qdrant`. |
| `RAG_EMBED_BACKEND` | `mock` | `mock` or `model_server`. |
| `RAG_EMBED_SERVER_URL`, `RAG_EMBED_SERVER_PATH`, `RAG_EMBED_DIM`, `RAG_EMBED_TIMEOUT_SECONDS` | `""`, `/embed`, `384`, `10.0` | Embedding server seam. |
| `RAG_CHUNK_SIZE`, `RAG_CHUNK_OVERLAP`, `RAG_DEFAULT_TOP_K`, `RAG_MOCK_EMBED_DIM`, `RAG_RETRIEVE_TIMEOUT_SECONDS`, `RAG_EMBED_MODEL` | `512`, `50`, `5`, `16`, `10.0`, `""` | |
| `LANGFUSE_ENABLED` | `false` in code, `true` in `.env.example` | Pairs with `make docker-run-langfuse`. Needs the `ai` extra. |
| `LANGFUSE_PUBLIC_KEY`, `LANGFUSE_SECRET_KEY`, `LANGFUSE_BASE_URL`, `LANGFUSE_PROMPT_CACHE_TTL_SECONDS` | `""`, `""`, `https://cloud.langfuse.com`, `60` | |
| `RECS_ENABLED` | `false` | Gates `Recommend`. |
| `RECS_BACKEND` | `memory` | `memory` (offline fixture) or `qdrant`. |
| `RECS_QDRANT_URL`, `RECS_QDRANT_COLLECTION` | `http://localhost:6333`, `item_als_vectors` | Must match platform-recsys. |
| `RECS_VECTOR_DIM`, `RECS_QDRANT_DISTANCE` | `64`, `Cosine` | Collection contract check at startup. |
| `RECS_CANDIDATE_TOP_K`, `RECS_RESULT_TOP_K` | `100`, `10` | |
| `RECS_CACHE_PREFIX`, `RECS_CACHE_SCHEMA_VERSION`, `RECS_CACHE_TTL_SECONDS` | `recs`, `v1`, `86400` | Redis key layout `<prefix>:<schema>:user:<id>`. |
| `RECS_RETRIEVE_TIMEOUT_MS` | `15` | Qdrant fallback latency cap. |
| `RECS_FEATURESTORE_REDIS_URL` | empty | Redis holding `fs:item_popularity:*` (platform-featurestore). Set: ranking boosts by `ctr_7d` / `favorites_current`; empty: no online features. Local compose uses `redis://redis:6379/2`. |
| `RECS_MODEL_VERSION` | `serving-fallback` | Replaced by the `model_version` Redis key when present. |

There is no `HOST`, `PORT`, `MODELSERVE_URL` or `QDRANT_URL` setting. Uvicorn takes host and port
from its CLI (the Dockerfile and `make dev` use `8000`).

Drift gate: `make check-env` (`scripts/check_env_example.py`) fails when `.env.example` and
`Settings` differ. See Known gaps: it currently fails.

## 5. Run locally

Root compose (from the agora root; it includes the shared infra: Redis, Qdrant, Postgres):

```bash
docker compose up -d --build team-ai                       # HTTP :8001, gRPC :50060
docker compose --profile jobs run --rm platform-recsys     # fills Qdrant + Redis for Recommend
```

The compose service builds with `UV_EXTRAS=recs` and sets `GRPC_ENABLED=true`,
`REDIS_ENABLED=true`, `CHAT_BACKEND=mock`, `RECS_ENABLED=true` and `RECS_BACKEND=qdrant`. Until
the recsys job has run, `Recommend` returns empty results. It does not set `RAG_ENABLED`, so
`SearchListings` answers `UNAVAILABLE` there.

Standalone:

```bash
cp .env.example .env
uv sync --dev --all-extras
make proto                    # generate gRPC stubs first (see Gotchas)
make dev                      # HTTP :8000, docs at /docs, probe at /healthz
GRPC_ENABLED=true make dev    # HTTP and gRPC in one process
make grpc                     # gRPC-only entrypoint (see Known gaps)
make worker                   # async task worker
```

`docker-compose.local.yaml` (`make docker-run`) is a template-era standalone stack, and
`make docker-run-langfuse` adds a local Langfuse.

## 6. Build, test and lint

| Command | What it runs |
|---|---|
| `make test` | `pytest -v` (`tests/unit`, `tests/integration`) |
| `make check` | `ruff check .` and `ruff format --check .` |
| `make check-env` | `.env.example` vs `Settings` |
| `make typecheck` | `pyright` |
| `make eval` | `scripts.run_eval --min-score 0.8` (judge cases skip without `JUDGE_CHAT_MODEL`) |
| `make ci` | `check`, `check-env`, `typecheck`, `test` |
| `make migrate`, `make migration-new NAME=...` | Alembic |

`make` targets use `uv run --all-extras`. Production images pick extras with
`--build-arg UV_EXTRAS=...`: `ai` for LangChain, LlamaIndex and Langfuse, `recs` for the Qdrant
client.

CI: `.github/workflows/test.yml` runs `uv sync --dev --all-extras`, `pytest -q` and the eval
gate. `.github/workflows/ci.yaml` is a second job (see Known gaps). Run `make ci` and
`make eval` before pushing.

## 7. Spec and verification

- `FEATURES.yaml` at the repo root: `ai.shopping-assistant`, `ai.magic-listing`,
  `ai.chat-copilot`, `ai.tag-classifier-taxonomy`, `ai.summarize-reviews`, and three
  `recommendations.contract-*` entries.
- E2E coverage lives in `platform-e2e` (`tests/e2e/features/ai/`,
  `tests/e2e/features/recommendations/`). Gates, from the agora root:
  `make -C platform-e2e features-check` and `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC. Relevant
  changes include `serve-trained-recs-locally` and `add-tag-classifier-filter-enrichment`.

## 8. Gotchas

- `app/transport/grpc/_pb/` is generated and gitignored. A fresh checkout has no stubs and the
  server will not import until `make proto` runs (the Docker build generates them in its own
  stage). `RecommendationService` is skipped with a warning if only its stubs are missing.
- `proto/` is a vendored copy of platform-core `packages/proto`. Never edit it here; copy the new
  contract from platform-core and run `make proto`. It also contains `listing.v1`, which this
  service does not serve.
- Stubs are import-rewritten so that `platform.*` does not shadow Python's stdlib `platform`.
- In-memory fallbacks: `RAG_BACKEND=memory`, `RAG_EMBED_BACKEND=mock`, `CHAT_BACKEND=mock`,
  `RECS_BACKEND=memory`, plus the in-memory recommendation feature and nearline stores and the
  tag store. Defaults never reach a real model or index.
- The recommendation ranker is a fixed-weight linear scorer (`GBDTRankerAdapter`), not a trained
  GBDT.
- Without the `ai` extra, LLM and RAG entry points raise a clear error; the app still boots.
- `LANGFUSE_ENABLED=true` in `.env.example` differs from the code default (`false`).

## 9. Known gaps

- **`make check-env` fails.** `.env.example` lacks every `RECS_*` setting (13 keys), so
  `make ci` fails at that step.
- **`.github/workflows/ci.yaml` is stale.** It runs `pytest tests/test_env_drift.py`, which does
  not exist, and installs with pip on Python 3.11 while the project requires 3.12.
- **`ai:use` is not enforced yet.** `MagicListing` and `ChatCopilot` need `listing.write`
  (`ChatCopilot` also rejects another seller's `seller_id`), `SummarizeReviews` needs
  `listing.read`, and `AIService` errors are mapped to field names or fixed text. But
  `ShoppingAssistant` and `StreamChat` stay open to any principal until team-identity grants
  `ai:use` to buyer, seller and admin and `AI_USE_SCOPE_REQUIRED=true` is set. The HTTP AI and
  tag routes (including `/tags/promote`, which mutates state) still have no auth dependency.
- **`make grpc` (`scripts/run_grpc.py`) does not wire recommendations.** It builds the server
  without `recommendation_provider`, so `Recommend` answers `UNAVAILABLE` there. Use the HTTP app
  with `GRPC_ENABLED=true`, as compose does.
- **gRPC is plaintext** (`add_insecure_port`), and `Check` always reports SERVING regardless of
  dependency state.
- **`ShoppingAssistant` ignores RAG.** It matches the hard-coded `CATALOG`, whatever `RAG_ENABLED`
  is. `SearchListings` is the only RAG-backed RPC.
- **No gRPC `TagClassifier` service.** `FEATURES.yaml` lists `ai.v1.TagClassifier`, but the proto
  has no such service; tag classification is HTTP only.
- **Tag state is not persisted** and is per process, so `/tags/promote` results vanish on restart
  and differ between replicas.
- **The listing indexer has no input.** `ListingEventIndexer` is not connected to any event
  source.
- The root `AGENTS.md` service table lists team-ai as `:8000` and omits gRPC and recommendations.

## 10. Links

- Root `AGENTS.md` (rules) and `README.md` (ASDLC): `../AGENTS.md`, `../README.md`
- Repo-local agent guide: `AGENTS.md` (points to `.agents/fastapi-template-repo/SKILL.md`)
- Platform ADRs in `platform-core/docs/ADR/`: `0001-proto-distribution`, `0003-auth-model`,
  `0010-service-zero-trust`, `0011-model-serving`, `0012-placement-engine`
- Local ADR: `docs/adr/0001-dgl-listing-generator-migration.md`
