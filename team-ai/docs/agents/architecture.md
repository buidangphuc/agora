# Architecture

Where code lives and how it is wired. This repo is team-ai, the AI service of the agora platform, not a plugin platform: a new domain is wired into the composition root by hand.

## Runtime

Redis and recommendations are mandatory, and gRPC is always served once the app opens its resources. The rate limit, the chat quota and taxonomy persistence are always on (Redis-backed). Four settings switch behaviour: `CHAT_BACKEND`, `RAG_ENABLED` (RAG and the listing indexer), `AI_USE_SCOPE_REQUIRED` and `DOCS_ENABLED`; telemetry starts when its endpoint or keys are configured. Settings has no flag for the mandatory parts; `REDIS_HOST` has no default.

## Layers

| Path | Role |
| --- | --- |
| `app/api` | Transport/controller layer. |
| `app/bootstrap` | App factory, lifespan resources, central service composition. |
| `app/transport/grpc` | gRPC transport: servicers, interceptors, error mapping, scopes. |
| `app/modules/platform` | Reusable capabilities: quota, rate limit, identity, model_server. |
| `app/modules/messaging` | Kafka consumer (`kafka/`) and the listing indexer (`listing_indexer/`). |
| `app/modules/ai` | Optional AI extras (LLM, RAG, evals) behind lazy-import guards (`app/modules/ai/_deps.py`). |
| `app/modules/business` | Domain/product logic only: ai_assistant, chat, completions, listing_screen, recommendation, tag_classifier. |
| `app/core` | Settings, infra primitives, health, errors, middleware. |

Layer flow:

- API routers parse HTTP input and call a business service.
- Business services receive explicit constructor dependencies.
- Platform modules expose reusable capabilities and backend factories.
- Bootstrap opens platform resources and composes app services.

Prefer existing repo patterns over new abstractions.

### API (`app/api`)

- Owns route paths, request parsing, query/path/body parameters, response models/envelopes, auth dependencies.
- Calls one business service method per endpoint when possible.
- Preserves published contracts: path, method, query names, body shape, envelope, status behavior.
- Views read services from `get_app_resources(request.app)` (the deliberate `app.state` seam). An optional capability lives on `ApplicationResources` as `X | None` and is narrowed in the view with `require(...)` (`app/bootstrap/state.py`), which raises a 503 when the feature is off.
- Endpoint files never build stores, DB clients, HTTP clients, quota adapters, LLMs or generators, and never hold business workflow decisions.
- No generic `app/api/deps.py` that collects unrelated wiring.

### Bootstrap (`app/bootstrap`)

- Opens lifecycle-owned platform resources, composes application services from them, stores each composed business service on its typed `ApplicationResources` field (one instance per app), and closes resources in reverse order.
- Use it for app-lifetime dependencies: the Redis client, shared HTTP clients, LLM instances, the RAG service, quota service, rate limiters, the recommendation clients.
- Anchors:
  - `resources.py`: `ApplicationResources`, open/close lifecycle. It always opens Redis and registers the `redis` readiness check, then opens each enabled addon.
  - `addons/base.py`: `BootstrapAddon` protocol and `default_resource_addons()` (telemetry, rate_limit, quota, rag, listing_indexer, taxonomy, recommendation). Rate limit, quota, taxonomy and recommendation are always on; rag and listing_indexer follow `RAG_ENABLED`, telemetry follows `OTEL_EXPORTER_OTLP_ENDPOINT`.
  - `state.py`: typed helpers such as `get_app_resources(...)`, `get_quota_service(...)`, `require(...)`.
  - `services.py`: `compose_business_services`, handler factories.
  - `addons/<name>.py`: one addon per capability. Contract (`addons/base.py`): `open(resources, settings, stack)` builds the capability with factory functions that take required dependencies, stores it on `resources`, and pushes its close onto `stack`. No `close` method.
- `ListingIndexerAddon` (runs with `RAG_ENABLED`; `kafka` extra) runs `supervise(...)` over `messaging/kafka` as a background task, feeding `listing.events` through `decode_listing_event` → `ListingEventIndexer` into `resources.rag_service`; the stack cancels the task on close. `SearchService/SearchListings` reads the same `rag_service`.
- Cross-setting rules ("X requires Y") live in a `@model_validator` on the owning settings mixin, not in addons or factories.

### gRPC transport (`app/transport/grpc`)

- The gRPC equivalent of `app/api`: owns the wire contract (stubs generated from `proto/` into the ignored `_pb` dir), interceptors (Tracing → Auth → RateLimit), error mapping and scopes.
- Servicers read composed services from `ApplicationResources` and call one service method per RPC. They never build stores, clients or LLMs, and hold no business decisions.
- Adding a domain: write a servicer under `app/transport/grpc/servicers/<domain>.py` and a `<domain>_service_registration(services, ...)` returning one `GrpcServiceRegistration` (service name, `register`, optional `rate_limited_methods`), then add it to `default_grpc_services_factory` in `app/bootstrap/grpc.py`. AIService (`servicers/ai.py`) is the example: each RPC runs `ensure_scopes` first, then a `@map_errors`-wrapped method that builds the pydantic request, calls the composed `resources.ai_assistant` bundle and maps the result to proto; ClassifyTags is the internal exception: it also requires a `service` principal and a title of 2+ characters, and calls `resources.tag_classifier`. The taxonomy always lives in its own Redis database (`TaxonomyAddon`, restored once at startup). Tests inject their own `grpc_services_factory` into `create_app`.
- Streaming: the business service (`ChatService.stream`) owns the stream and its errors; the servicer (`servicers/chat.py`) only maps deltas to `StreamChatResponse` and `ChatError` subclasses to fixed status and message pairs.
- `serve_grpc` runs inside the API lifespan after `open_runtime` has composed the services, whenever `init_resources` is true (there is no flag; `GRPC_PORT=0` binds an ephemeral port in tests), and stops before the runtime closes. It is not a `BootstrapAddon`: addons open before the services exist, and servicers need the services. The four services are `AIService`, `ChatService`, `RecommendationService` and `SearchService` (`SearchListings` only; `servicers/search.py` reads `resources.rag_service`).
- Request counts are exposed at `GET /metrics` (`teamai_grpc_requests_total`).

### Platform modules (`app/modules/platform`)

- Reusable cross-product capabilities and backend adapters; factories choose concrete backends from settings; each adapter implements one backend.
- Examples: the quota service over the Redis store (`QUOTA_REDIS_PREFIX`); the Redis rate limiter; identity; the model server client.
- No product-specific setup here.

```text
factory chooses implementation
adapter implements one stack
service consumes interface/capability
```

### Business modules (`app/modules/business`)

- Domain/product services, schemas, models, stores, integrations, generation logic.
- Services receive explicit constructor dependencies. Stores own collection/table details; integrations own external API details; AI generation owns prompt building and model parsing.
- A business service never reads `Request` or `app.state`, never uses a global service locator, and never opens app-lifetime runtime resources itself.
- Formats owned by another system's producers (Redis keys, collection and point ids, artifact layouts) live in the module's `contract/` and are pinned by tests; change them only with the producer (see `recommendation/contract/`).

### Package `__init__.py`

`__init__.py` files stay empty (a docstring is allowed); no re-exports, no `__all__`. Import each symbol from the file that defines it, and only what you use. `tests/unit/test_package_inits.py` enforces this.

## Adding or changing a service

1. Define one `<Domain>Service` class in `app/modules/business/<domain>/service.py` (e.g. `CompletionService`); add schemas/types/models/stores/integrations under the same domain.
2. Keep stores, handlers and external integrations as constructor dependencies.
3. Decide scope: request-scoped uses an existing FastAPI dependency or session boundary; app-lifetime is built once per app in lifespan after resources open (`app/bootstrap/services.py`) and stored on a typed `ApplicationResources` field (e.g. `completion_service: CompletionService`). No module-level singletons.
4. A feature behind a switch (`RAG_ENABLED`-style) gets a `BootstrapAddon` that sets an optional field on `ApplicationResources` only when enabled; the view or servicer narrows it with `require(...)` and answers 503 when it is off. A mandatory capability (like recommendations) is a non-optional field composed in `compose_business_services`.
5. Endpoint functions read the service from `get_app_resources(request.app)`, parse HTTP input, call one service method, and return the existing response contract. No per-surface getter helpers.
6. Tests: unit test for orchestration and failure modes; bootstrap test that lifespan composes the service; integration test for the API contract (tests run the app lifespan, see `tests/factories.py:api_client_for`).

## Adding an endpoint

```text
API endpoint
  -> dependency adapter retrieves service
  -> service.method(...)
  -> response
```

Leave bootstrap alone when the service already has its dependencies. Touch bootstrap only when the endpoint introduces a new app-level service or a new lifecycle-owned runtime dependency.

## Wire contracts

The gRPC contract (`proto/`, vendored from platform-core) and the team-ai HTTP routes under `/api/v1/ai` are consumed by team-gateway and team-search. Keep paths, RPC names, message shapes and status behavior stable; improve internals only:

```text
existing RPC or route stays the same
  -> servicer/view calls service
  -> service owns business flow
  -> platform capabilities provide shared infrastructure
```

The `/api/v1/completions` surface is a scaffold pattern for thin transport, not a replacement for those contracts.

## Canonical Feature: completions

`app/api/v1/completions/` is the reference. For a new feature `<domain>`, copy its shape; reuse the service rules above (`<Domain>Service`, one bootstrap instance, `get_app_resources`).

- API package `app/api/v1/<domain>/`: an empty `__init__.py` and `router.py`, which declares `APIRouter(prefix=..., tags=..., dependencies=[Depends(require_principal)])` once. Routers declare their own tags (completions also its `/completions` prefix); `app/api/router.py` (`build_api_router`) mounts them: health at `/` and at `API_V1_PREFIX`, completions at `API_V1_PREFIX`, ai at `API_V1_PREFIX` + `/ai` (prefix added at include time). A route that needs the caller takes `principal: Principal = Depends(require_principal)` as a parameter; a scope-gated route uses `dependencies=[Depends(require_scopes(...))]` (see `app/api/v1/ai/router.py`).
- View: `get_app_resources(request.app).<domain>_service`, one service call, `<Response>.model_validate(result, from_attributes=True)`. Response ids default in the response schema (`Field(default_factory=...)`). A view does not time, log, record telemetry, apply deadlines or mint ids.
- Streaming: the view passes an async generator of `ServerSentEvent` to `event_stream_response(request, events)` from `app/api/sse.py`, which owns headers, heartbeat, first-event priming (errors before it stay JSON errors), `event: error` frames and shielded close. The generator closes the service stream with `contextlib.aclosing`. A feature owns no SSE engine, heartbeat or deadline code. Frames: each non-empty delta yields `ServerSentEvent({"type": "content.delta", "delta": ..., "metadata": ...})`; the stream ends with `ServerSentEvent({"type": "done", **summary.model_dump(exclude_none=True)})`.
- Timing, deadlines, outcome labels and telemetry live in the business service, behind ports (`CompletionSink`, `StreamPolicy`) that `compose_business_services` in `app/bootstrap/services.py` builds once. The HTTP completions routes use that instance, so every completion is observed.

Reference views (`app/api/v1/completions/router.py`):

```python
@router.post("", response_model=CompletionResponse)
async def complete(payload: CompletionRequest, request: Request) -> CompletionResponse:
    service = get_app_resources(request.app).completion_service
    result = await service.complete(payload)
    return CompletionResponse.model_validate(result, from_attributes=True)


@router.post("/stream")
async def stream(payload: CompletionRequest, request: Request) -> StreamingResponse:
    service = get_app_resources(request.app).completion_service
    return await event_stream_response(request, _events(service.stream(payload)))
```

## Verification

Run the narrowest useful checks, then widen:

- `uv run ruff check app tests`
- `uv run pyright`
- focused pytest for the touched module/API
- full `uv run pytest` after bootstrap, platform, or public API changes

Docker smoke checks: use a minimal local runtime with external AI dependencies disabled unless real provider calls are requested.

AI features: see `docs/agents/ai-engineering.md`.
