# flag-trim Specification

## Purpose
Defines team-ai's settings surface: the four switches that remain, the limits and behaviours that are always on, what the RAG switch covers, and how observability turns on. Ported from the team-ai rewrite change `flag-trim`.

## Requirements

### Requirement: Only four switches remain
Settings SHALL keep exactly these on/off and selector settings:
- `CHAT_BACKEND: Literal["mock","llm_router"]`
- `RAG_ENABLED: bool`
- `AI_USE_SCOPE_REQUIRED: bool`
- `DOCS_ENABLED: bool`

`DOCS_ENABLED` defaults to true only when `ENVIRONMENT` is local, dev or test.

Settings SHALL NOT have these fields:
- the removed flags: `GRPC_REFLECTION_ENABLED`, `GRPC_BEARER_FALLBACK_ENABLED`, `GRPC_RATE_LIMIT_ENABLED`, `GRACEFUL_SHUTDOWN_ENABLED`, `REQUEST_TIMEOUT_ENABLED`, `SECURITY_HEADERS_ENABLED`, `SECURITY_HSTS_ENABLED`, `GZIP_ENABLED`, `RATE_LIMIT_ENABLED`, `RATE_LIMIT_IP_ENABLED`, `RATE_LIMIT_PRINCIPAL_ENABLED`, `QUOTA_ENABLED`, `TAXONOMY_PERSISTENCE_ENABLED`, `LISTING_INDEXER_ENABLED`, `OTEL_ENABLED`, `LANGFUSE_ENABLED`;
- the removed backend selectors: `RATE_LIMIT_BACKEND`, `QUOTA_BACKEND`, `RAG_BACKEND`, `RAG_EMBED_BACKEND`, `RECS_BACKEND`.

#### Scenario: removed keys in an env file
- **WHEN** a `.env` file passed to Settings contains any removed key
- **THEN** Settings validation fails

#### Scenario: gitops environment still validates
- **WHEN** Settings are built from the production values in `platform-gitops/argocd/apps/team-ai.yaml` as OS environment variables, plus `AUTH_API_KEYS`
- **THEN** validation passes

#### Scenario: docs default
- **WHEN** `ENVIRONMENT=production` and `DOCS_ENABLED` is unset
- **THEN** the OpenAPI docs routes are not served
- **AND** with `ENVIRONMENT=local` they are served

### Requirement: Limits are always on
The app SHALL always enforce:
- the per-principal HTTP rate limit on authenticated routes;
- the gRPC rate limit on `/platform.ai.v1.AIService/ShoppingAssistant` and `/platform.chat.v1.ChatService/StreamChat`.

Both limits SHALL use the Redis rate limiter, with the prefix `RATE_LIMIT_REDIS_PREFIX` and the limit and window from `RATE_LIMIT_PRINCIPAL_PER_MINUTE` and `RATE_LIMIT_WINDOW_SECONDS`.

The app SHALL always meter the chat reply quota, counted in Redis (atomic reservations, so concurrent pods cannot exceed the limit) under the prefix `QUOTA_REDIS_PREFIX` (default `quota`). Quota keys SHALL expire after their window plus a margin. The app SHALL NOT apply a per-IP HTTP rate limit.

#### Scenario: gRPC burst
- **WHEN** one principal calls ShoppingAssistant more times than the limit within one window
- **THEN** the call over the limit fails RESOURCE_EXHAUSTED with a `retry-after` trailer

#### Scenario: quota shared across pods
- **WHEN** two app instances on one Redis reserve chat replies concurrently against a limit of L
- **THEN** exactly L reservations succeed

#### Scenario: no llm_router boot guard
- **WHEN** `CHAT_BACKEND=llm_router` and `ENVIRONMENT=production`
- **THEN** Settings validation does not fail on rate-limit or quota grounds

### Requirement: Always-on runtime behaviours
The app SHALL always:
- serve gRPC reflection;
- drain gracefully on shutdown;
- apply the HTTP request timeout `REQUEST_TIMEOUT_SECONDS`;
- persist the tag taxonomy in Redis on `TAXONOMY_REDIS_DATABASE`, keeping the same key format.

It SHALL NOT have security-header, HSTS or gzip middleware. It SHALL NOT have a gRPC bearer fallback.

#### Scenario: taxonomy persisted by default
- **WHEN** a candidate is promoted, and a new app instance starts on the same Redis
- **THEN** the promoted tag is still canonical

### Requirement: RAG switch covers indexing and backends
With `RAG_ENABLED=true`, the app SHALL use the Qdrant vector store, the model_server embeddings and the Kafka listing indexer. Their settings are required: `RAG_QDRANT_URL`, `RAG_EMBED_SERVER_URL` and `KAFKA_BROKERS`. With `RAG_ENABLED=false`, none of them is built.

#### Scenario: RAG on without embed server
- **WHEN** `RAG_ENABLED=true` and `RAG_EMBED_SERVER_URL` is empty
- **THEN** Settings validation fails

### Requirement: Observability turns on by configuration
OpenTelemetry export SHALL start exactly when its OTLP endpoint setting is non-empty. Langfuse tracing SHALL start exactly when both Langfuse keys are non-empty.

#### Scenario: no endpoint
- **WHEN** the OTLP endpoint setting is empty
- **THEN** no exporter is created and no OTel dependency is imported
