# Usage

How to run, configure and operate team-ai. For an overview of the repo see the [README](../README.md); for conventions see [agents/architecture.md](agents/architecture.md).

## Requirements

- Python 3.12+
- uv
- Redis (mandatory: `REDIS_HOST` has no default)
- Qdrant for recommendations (always on; `RECS_QDRANT_URL` defaults to `http://localhost:6333`)
- Docker and Docker Compose for local container checks

## Quickstart

```bash
cp .env.example .env
uv sync --dev --all-extras
make test
make dev
```

`make dev` generates the gRPC stubs and starts the API on `http://localhost:8000` and gRPC on `GRPC_PORT` (`.env.example` ships `50051`; production runs `50060`). `.env.example` points `REDIS_HOST` at the compose service name `redis`, so for a host-run server change it to `localhost` and run a Redis. Recommendations always read Qdrant at `RECS_QDRANT_URL`.

## gRPC services

Served by the platform-core contract (vendored in `proto/`), always started once the app opens its resources. Interceptors run Tracing, then Auth, then RateLimit.

| Service | RPCs served |
| --- | --- |
| `platform.ai.v1.AIService` | `ShoppingAssistant`, `MagicListing`, `ChatCopilot`, `SummarizeReviews`, `ClassifyTags` and `ScreenListing` (internal: service principals with `ai.classify`) |
| `platform.chat.v1.ChatService` | `StreamChat` (server stream) |
| `platform.recommendation.v1.RecommendationService` | `Recommend` |
| `platform.search.v1.SearchService` | `SearchListings` only (scope `search:read`, answers unavailable unless `RAG_ENABLED`); the other search RPCs return unimplemented |

Auth is the gateway's: the service reads forwarded `x-principal-*` metadata and enforces scopes. A call without that metadata is `UNAUTHENTICATED`; with `grpcurl`, send the `x-principal-*` headers yourself. `AI_USE_SCOPE_REQUIRED` adds the `ai:use` scope to the shopper LLM RPCs and chat. `ShoppingAssistant` and `StreamChat` are always throttled per principal on Redis (`RATE_LIMIT_PRINCIPAL_PER_MINUTE` per `RATE_LIMIT_WINDOW_SECONDS`), and reflection is always served.

## HTTP endpoints

- `GET /healthz`: liveness (always 200 while the process is alive).
- `GET /readyz`: readiness. 503 when Redis is unreachable or while draining.
- `GET /metrics`: Prometheus text with `teamai_grpc_requests_total`, the counter KEDA scales on.
- `POST /api/v1/ai/assistant`, `/magic-listing`, `/chat-copilot`: the same assistants as `AIService`; no principal, the gateway authenticates.
- `POST /api/v1/ai/tags/classify`, `/tags/classify-sku-hierarchy`, `GET /api/v1/ai/tags`: tag classification, scope `ai.classify`.
- `POST /api/v1/ai/tags/explore`, `/tags/promote`: taxonomy curation, scope `admin`.
- `POST /api/v1/completions`, `POST /api/v1/completions/stream`: completions (see below).

All `/api/v1/completions*` routes and the tag routes require `Authorization: Bearer <api-key>` (see [Bearer Auth](#bearer-auth)).

## Switches and features

Four settings switch behaviour: `CHAT_BACKEND`, `RAG_ENABLED`, `AI_USE_SCOPE_REQUIRED` and `DOCS_ENABLED` (default on only in dev/local/test). Everything else below is always on or follows its configuration. The names are the settings in `.env.example`.

| Feature | Setting | Notes |
| --- | --- | --- |
| RAG | `RAG_ENABLED` | Needs the `ai` and `kafka` extras. On, it uses Qdrant (`RAG_QDRANT_*`), model_server embeddings (`RAG_EMBED_SERVER_URL`) and the listing indexer (`KAFKA_BROKERS`); all three are required. Feeds `ShoppingAssistant` and `SearchListings`. |
| Chat backend | `CHAT_BACKEND` | `mock` echoes offline; `llm_router` streams from the LLM chain (`CHAT_MODEL`, `CHAT_FALLBACK_MODELS`) and needs the `ai` extra. |
| Listing indexer | (with `RAG_ENABLED`) | Consumes `listing.events` from `KAFKA_BROKERS` into RAG. Indexes `PUBLISHED` listings only (and an empty status from legacy producers). |
| Listing screen | `SCREEN_BACKEND` | `mock` (default) flags text containing `SCREEN_MOCK_MARKER`; `llm_router` runs the LLM chain and needs the `ai` extra. Served as `AIService/ScreenListing`. |
| Taxonomy persistence | always on | Curated tags survive restarts, in Redis database `TAXONOMY_REDIS_DATABASE` (must differ from `REDIS_DATABASE`). |
| Quota | always on | Chat reply quota (`QUOTA_CHAT_*`), counted in Redis under `QUOTA_REDIS_PREFIX` so every pod shares it. |
| Rate limiting | always on | Per-principal limit on HTTP and gRPC, counted in Redis (`RATE_LIMIT_*`). There is no per-IP limit: edge limiting is the gateway's job. |
| Tracing | keys / endpoint | Langfuse starts when `LANGFUSE_PUBLIC_KEY` and `LANGFUSE_SECRET_KEY` are set; OpenTelemetry starts when `OTEL_EXPORTER_OTLP_ENDPOINT` is set (needs the `otel` extra, see [ops/observability](../ops/observability/README.md)). |

### Completions

`POST /api/v1/completions` returns one JSON completion; `POST /api/v1/completions/stream` returns server-sent events. They are thin transport: they call an injected `CompletionHandler` (the scaffold `EchoCompletionHandler` unless `create_app(completion_handler_factory=...)` supplies one) and know nothing about prompts, models or retrieval. There are no task routes.

### Streaming (`POST /api/v1/completions/stream`)

The response is `text/event-stream` with `Cache-Control: no-cache` and `X-Accel-Buffering: no`. Frames:

- `data: {"type":"content.delta","delta":"...","metadata":{}}` for each piece of text.
- `: ping` comment lines every `SSE_HEARTBEAT_SECONDS` while the model is quiet (clients ignore them).
- `data: {"type":"done", ...}` last, with `usage`, `finish_reason` and `model` when the handler reported them (plain `{"type":"done"}` otherwise).
- `event: error` + `data: {"type":"error","code":"..."}` if the stream fails after it started (codes: `stream_timeout`, `stream_error`, or an application error code); the stream then closes with no `done`. The frame never carries exception text.

Failures before the first chunk (guardrail violation, handler startup error, no chunk within `STREAM_FIRST_TOKEN_TIMEOUT_SECONDS`) return the normal JSON error envelope (422 / 500 / 504 `stream_timeout`) instead of a `200` stream. A gap longer than `STREAM_IDLE_TIMEOUT_SECONDS` between chunks ends the stream with `stream_timeout`.

Each sync and stream completion logs one `completion mode=... outcome=... model=... duration_ms=... ttft_ms=... finish_reason=... input_tokens=...` line (no content).

## Docker

```bash
cp .env.example .env
make docker-build
make docker-run
```

The Docker path is a local golden path only. It builds the API image and runs the API and Redis with `ops/compose/local.yaml`. The image exposes `8000` (HTTP) and `50060` (gRPC); set `GRPC_PORT=50060` to match. Optional extras are chosen at build time, e.g. `--build-arg UV_EXTRAS="ai kafka"` (the production set).

To run the API with a local self-hosted Langfuse stack for AI tracing, prompt management, and eval scores:

```bash
cp .env.example .env
cp ops/compose/langfuse.env.example .env.langfuse   # optional Langfuse overrides
make docker-run-langfuse
```

The Langfuse overlay is `ops/compose/langfuse.yaml`; `make docker-run-langfuse` layers it on `ops/compose/local.yaml` and loads `.env.langfuse` when present.

Langfuse is available at `http://localhost:3000`. The local project and API keys are bootstrapped from `.env` through Langfuse headless initialization. The API container uses `LANGFUSE_DOCKER_BASE_URL=http://langfuse-web:3000`; host Python processes can use `LANGFUSE_BASE_URL=http://localhost:3000`.

### Graceful shutdown and Kubernetes

The image runs `python -m app.serve` (also usable locally; `SERVER_HOST` / `SERVER_PORT`, default `0.0.0.0:8000`). uvicorn closes its listener as soon as it gets SIGTERM, which would hide the `/readyz` 503 from load balancers, so on the first SIGTERM/SIGINT `app.serve` marks the app draining (`/readyz` returns 503, `/healthz` stays 200), keeps accepting for `SHUTDOWN_READINESS_DELAY_SECONDS` (default 5, `0` disables), then runs the normal graceful shutdown. A second signal forces exit. The gRPC server stops first, within `GRPC_GRACE_SECONDS`, before the resources close.

- A `preStop: sleep` hook is optional with this delay; set the delay at least `periodSeconds * failureThreshold` of the readiness probe plus endpoint propagation time.
- `terminationGracePeriodSeconds` must exceed `SHUTDOWN_READINESS_DELAY_SECONDS` + `GRACEFUL_SHUTDOWN_TIMEOUT_SECONDS` (connection drain), plus another `GRACEFUL_SHUTDOWN_TIMEOUT_SECONDS` in the worst case (the lifespan then waits for in-flight requests): `5 + 2 * 30 + margin` with the defaults.

## Local commands

Run `make help` for every target. The main ones:

```bash
make dev                    # run local API with uvicorn reload
make proto                  # generate the gRPC stubs from proto/
make test                   # run full pytest suite
make eval                   # run the eval gate
make smoke-langfuse         # exercise the Langfuse callback against the local stack
make smoke-langfuse-prompt  # exercise the Langfuse prompt-management flow
make check-env              # verify .env.example matches Settings fields
make ci                     # ruff checks + env check + typecheck + tests
```

## Secrets

Copy `.env.example` to `.env` for local development. Keep real secrets in environment variables or your team's secret manager, not in Git. Environment variables that Settings does not know are ignored when they come from the OS environment (gitops keys that were removed are inert), but an unknown key in a `.env` file fails validation.

## Local `.env` defaults (from `.env.example`)

The values below are what `.env.example` ships for the local golden path; they are **not** all the code defaults. In code Langfuse and OpenTelemetry are off until their keys or endpoint are set, `AUTH_API_KEYS` defaults to `""` (no keys, no implicit admin), and `LANGFUSE_BASE_URL` defaults to `https://cloud.langfuse.com`.

- `REDIS_HOST=redis` (required; no code default)
- `CHAT_MODEL=` (empty: the local fake model) and `CHAT_BACKEND=mock`
- `AUTH_API_KEYS=local-user:admin:<sha256 of change-me-local-bearer-token>`
- `CORS_ALLOW_ORIGINS=*`
- `TRUSTED_HOSTS=*`
- `MAX_REQUEST_BODY_BYTES=10485760`
- `LANGFUSE_PUBLIC_KEY=` and `LANGFUSE_SECRET_KEY=` (blank: Langfuse off; `make docker-run-langfuse` passes the local stack's keys)
- `LANGFUSE_BASE_URL=http://localhost:3000`  *(example only; code default `https://cloud.langfuse.com`)*

To use a real model, install the `ai` extra, set the provider's standard environment variables in your runtime, then set `CHAT_MODEL` to a native LangChain target such as `openai:gpt-4.1-mini` or `anthropic:claude-sonnet-4-5`.

HTTP middleware provides CORS, trusted-host enforcement, a request body limit, standard error envelopes for FastAPI/Starlette HTTP errors, and access logs with method, path, status, duration, request id, and authenticated principal when available.

Endpoints reach the request boundary through small per-concern dependencies, not a single context object: `Depends(require_principal)` (`app.modules.platform.identity.auth`) for the authenticated principal (which also sets `request.state.principal`), `Depends(require_scopes(...))` for scope-gated routes, and `get_request_id()` (`app.core.request_context`) for the request id. No shared context object carries Redis, feature flags, or AI runtimes.

Application wiring is centralized in the bootstrap layer. API routers and gRPC servicers keep only transport concerns: parsing, response mapping, and a small call into a business service. Platform modules expose reusable capabilities such as quota, rate limit, identity and the model server client; `app/bootstrap/resources.py` opens those capabilities once for the app, and `app/bootstrap/services.py` composes the business services from them.

Business services receive explicit constructor dependencies and must not read FastAPI `Request`, `app.state`, or a global service locator. Each is built once per app and stored on a typed `ApplicationResources` field (e.g. `completion_service`, `ai_assistant`, `chat`, `tag_classifier`, `recommendation`). Views and servicers read it with `get_app_resources(request.app)`; no per-surface getter helpers.

Adding a new endpoint usually should not touch bootstrap. Add or extend the business service method, then call it from the API router or servicer. Update bootstrap only when the endpoint introduces a new runtime dependency or a new application service that must be created once per app lifecycle.

List endpoints can reuse `app.core.pagination.PaginationParams` and `build_list_response(...)` for offset-based responses: `{"items": [...], "pagination": {"limit": 50, "offset": 0, "total": 123}}`.

Langfuse is only wired at the AI execution boundary. Use `build_llm_instance(..., instance_id="...", service_name="...")` from `app.modules.ai.llm.runtime` to create a native LangChain `BaseChatModel` plus a per-instance Langfuse tracker. Pass `instance.trace_config(...)` into LangChain calls to keep parallel LLM services separated by instance, service, session, user, and request metadata.

To use the completions transport with a real model, pass a handler factory. It runs in lifespan with the opened resources, so the handler can take an LLM instance, RAG service or stores from them:

```python
from collections.abc import AsyncIterator

from app.modules.business.completions.schemas import (
    CompletionRequest,
    CompletionResult,
    CompletionStreamChunk,
)
from app.bootstrap.application import create_app
from app.bootstrap.resources import ApplicationResources
from app.core.config import Settings


class MyCompletionHandler:
    async def complete(self, request: CompletionRequest) -> CompletionResult:
        return CompletionResult(content="...")

    async def stream(
        self,
        request: CompletionRequest,
    ) -> AsyncIterator[CompletionStreamChunk]:
        yield CompletionStreamChunk(delta="...")


def build_handler(resources: ApplicationResources, settings: Settings) -> MyCompletionHandler:
    return MyCompletionHandler()


app = create_app(completion_handler_factory=build_handler)
```

The app factory registers a FastAPI lifespan that calls `langfuse.get_client().flush()` on shutdown when the Langfuse keys are set and `init_resources=True`, so any buffered traces from per-instance trackers are drained before the process exits. The app factory also owns shared runtime resources on `app.state.resources` (an `ApplicationResources` holding `.redis` and the composed services); readiness checks reuse those handles, and the lifespan stops gRPC, then closes Redis and the other resources on shutdown. Tests construct the app with `init_resources=False` to skip resources and external readiness checks; with resources on, the test suite supplies a fakeredis client and `GRPC_PORT=0`.

## Bearer Auth

Clients authenticate HTTP requests with an API key sent as `Authorization: Bearer <token>`. The server stores only SHA-256 digests, in `AUTH_API_KEYS`: comma-separated entries of

```
subject:scope|scope:sha256hex      # e.g. billing-svc:ai.classify|admin:3f2a...c9
```

(`subject` is everything before the first `:`, the digest everything after the last; scopes may contain `:` and may be empty, which grants nothing.)

```bash
python -m scripts.hash_api_key --generate   # prints a random token and its digest
# AUTH_API_KEYS=billing-svc:admin:<digest>
curl -X POST http://localhost:8000/api/v1/completions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"messages": [{"role": "user", "content": "hi"}]}'
```

Tokens must be randomly generated (the digest is unsalted, so a guessable token is brute-forceable from a leaked config). The presented token's digest is compared in constant time against every entry.

**Rotation (overlap).** Several entries may share a subject. Add a second entry for the same subject, roll the client over to the new token, then remove the old entry. The principal (`subject`) is unchanged throughout, so rate limits carry over.

The subject is the principal id, so per-principal rate limiting is per-client. The `Principal` built by `require_principal` (`app/modules/platform/identity/auth.py`) is `{"id": subject, "type": "service", "scopes": [...]}` and guards every `/api/v1/completions*` route. The service intentionally does not create users, passwords, sessions, or key-management tables. There is no `/auth/me` endpoint; its absence is pinned by `tests/unit/core/test_application_wiring.py`.
