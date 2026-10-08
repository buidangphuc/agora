## Why

Verified by reading `team-gateway`, `team-ai`, `team-identity` and `platform-recsys`, the edge and the AI service
have gaps that let the wrong caller in, leak internals, or quietly serve wrong data:

- **Gateway.** `GET /api/admin/metrics` needs no auth and sets `Access-Control-Allow-Origin: *`; its
  `total_orders_24h`, `total_revenue_24h` and `recent_traces` are constants in `internal/edge/cockpit.go`.
  `GET /api/events/live` (SSE) has no auth and no room authorisation, and nothing in the repo ever calls
  `Broadcast`, so it can only emit the handshake and heartbeats. The interceptor chain is unary-only, so
  `StreamChat` (the one LLM path) and the plain HTTP routes skip request-id, logging and rate limit, and
  `StreamChat` has no deadline. All four `AIService` RPCs go through `callRead` (5 s per attempt, up to
  `RETRY_MAX` retries on `Unavailable`), a policy that is wrong for generation calls. CORS `AllowedHeaders`
  omits `Idempotency-Key`. `make check-env` runs `-run TestEnvExampleInSync`, a test that does not exist in
  `team-gateway`, so it passes vacuously; `DIAL_TIMEOUT_SECONDS` is never read. After an upstream is recreated
  the gateway keeps answering 503 for roughly 2 to 4 s (gRPC default reconnect backoff) even once the upstream
  listens.
- **team-ai.** `AIServicer` has no scope gate, so `MagicListing` and `ChatCopilot` (seller features) and the
  shopper RPCs are open to any forwarded principal including anonymous; every unexpected error aborts with
  `INTERNAL` and the raw `str(exc)` (for a pydantic error that text includes the offending input). Anonymous
  callers skip quota and all share the `anonymous:anonymous` rate-limit bucket. Quota reservation uses
  `idempotency_key=request_id`, and the request id is whatever `X-Request-Id` the client sent, so a client that
  reuses one id gets the same reservation back and is never charged again. `LLM_TRACE_CONTENT=off` only blanks
  the assistant's own log lines; Langfuse still receives the masked model input.
- **Recommendations.** `platform-recsys` stores Qdrant points under `uuid5(NS, listing_id)` with
  `{listing_id, model_version, updated_at}` as payload. `QdrantRetrievalBackend` passes the raw listing id to
  `recommend(positive=...)` and returns `hit.id` (the UUID) as `listing_id`, so the ANN stage cannot work against
  real data; `popular()` is an arbitrary `scroll`, not popularity; `model_version` is a static label although
  `recs:v1:model_version` is written and never read; `in_stock` is read from a payload field the producer never
  writes, so it is always `true`. `make grpc` builds the server without a `recommendation_provider`, so
  `Recommend` is always `UNAVAILABLE` there.

None of this is covered by a gate: the e2e `AiService` helper does not even call the real RPCs (it returns
hard-coded replies and a wrong service name).

## What Changes

Repos: `team-gateway` and `team-ai` (main), `team-identity` (one scope grant), `platform-recsys` (contract
hardening and a golden fixture), `team-frontend` (minimal, forced by the gateway changes), `platform-e2e`.
No proto change.

**Gateway (`team-gateway`)**
- `GET /api/admin/metrics` requires a verified principal holding the existing `admin` scope (401 without a valid
  token, 403 without the scope); the handler no longer sets `Access-Control-Allow-Origin` (the shared CORS
  middleware owns it). Fabricated figures are removed: `total_orders_24h` and `total_revenue_24h` become
  `null` until a real source exists, `recent_traces` is `[]`.
- `GET /api/events/live` and `RealtimeBroker` are removed (dead code, no publisher, no auth). A future
  Kafka-to-SSE bridge must bring its own authorisation requirement.
- One interceptor implementation covers unary **and** streaming handlers (request-id, auth resolution, logging,
  rate limit); `StreamChat` gets a per-principal token bucket, a per-principal concurrent-stream cap and a
  maximum stream duration; plain HTTP routes (`/api/track`, cockpit) get the same request-id, logging and rate
  limit through an HTTP middleware; `X-Request-Id` from the client is validated, not trusted blindly.
- AI generation RPCs (`ShoppingAssistant`, `MagicListing`, `ChatCopilot`) use a dedicated no-retry policy with
  `AI_CALL_TIMEOUT_SECONDS`; `SummarizeReviews` (public, on the product page render path) stays on the read
  policy.
- CORS allows the `Idempotency-Key` request header, with a preflight test.
- The gateway no longer routes `ListingService/ReserveStock` and `ListingService/ReleaseStock` (internal
  service-to-service RPCs called by `team-order`); a request to those paths returns HTTP 501 `unimplemented` for anonymous
  and authenticated callers alike. The listing `CommitReservation` is not routed today and stays unrouted.
- `TestEnvExampleInSync` is added; `DIAL_TIMEOUT_SECONDS` is wired to the gRPC connect timeout; upstream
  connections use a tuned reconnect backoff and reads wait for reconnect (bounded), so recovery after an upstream
  restart is at most about 1.5 s after it listens.

**team-ai**
- Scope gate on `AIService` and `StreamChat`: `MagicListing`, `ChatCopilot` need `listing.write`
  (seller/admin); `ShoppingAssistant` and `StreamChat` need the **new scope `ai:use`** (granted by
  `team-identity` to buyer, seller, admin); `SummarizeReviews` needs `listing.read` (kept public).
  Anonymous callers are denied on every LLM-capable RPC, which also removes the shared anonymous bucket and the
  quota bypass.
- Error mapping without leaks: validation failures become `INVALID_ARGUMENT` with field names only, known
  unavailability becomes `UNAVAILABLE`, everything else becomes `INTERNAL` with a fixed message; details stay in
  logs keyed by request id.
- Quota reservation keys come from a server-minted per-call id, never from the client's request id.
- `LLM_TRACE_CONTENT` becomes one content policy for every sink: `off` sends no prompt or completion text to
  Langfuse (metadata, usage and latency still flow), `redacted` masks, `full` passes through.
- Recommendation consumer matches the producer: point id derived with the producer's UUID5 namespace, listing
  id read from the payload, `model_version` read from `recs:v1:model_version` (cached in process), popularity
  read only from `recs:v1:popular`, no fake stock filter. `make grpc` wires `recommendation_provider`.

**team-identity**: add `ai:use` to the buyer, seller and admin grants in `internal/authz/scopes.go`
(sequenced after `order-domain-correctness`, which also edits this file and adds `recommendations:read`).

**platform-recsys**: document the contract in its README, add a golden-fixture test that pins
`point_id("listing-1")` and the payload keys, and create payload indexes on `listing_id`/`model_version`.

**team-frontend** (scope-limited, decided): exactly two edits, each with unit tests and no visual change: (a) the
cockpit reads admin metrics through a same-origin route handler that attaches the session token, (b) `CockpitView`
stops opening `EventSource`. Nothing else in the frontend changes in this change.

**BREAKING (behavioural)**: anonymous and buyer calls to `MagicListing`/`ChatCopilot`, and anonymous calls to
`ShoppingAssistant`/`StreamChat`, now fail (`PERMISSION_DENIED`; decided: `ai:use` is not in `PUBLIC_SCOPES`); gateway paths for `ReserveStock`/`ReleaseStock` return 501 `unimplemented`; unauthenticated `/api/admin/metrics` returns
401; `/api/events/live` returns 404; buyers holding a token issued before the identity change lack `ai:use` until
it expires (default 1 h, `JWT_TTL_SECONDS=3600`).

## Capabilities

### New Capabilities
- `edge-hardening`: how `team-gateway` authenticates its own HTTP endpoints, covers every RPC shape and HTTP
  route with request-id/logging/rate limit, bounds streams, applies the right call policy to AI RPCs, serves
  CORS for `Idempotency-Key`, keeps its env documentation honest and recovers quickly from an upstream restart.
- `ai-service-hardening`: who may call each `AIService`/`StreamChat` RPC (scopes, anonymous policy), what errors
  callers see, how quota keys are minted, and what `LLM_TRACE_CONTENT` guarantees for Langfuse.
- `recsys-serving-contract`: the producer/consumer contract between `platform-recsys` and `team-ai` (point id
  mapping, payload fields, model version source, popularity source) and its tests against the producer's real
  point format.

### Modified Capabilities
- `ops-cockpit`: the metrics endpoint now requires the `admin` scope and the "frontend is unchanged" clause is
  relaxed; the "orders and revenue are labelled derived" requirement is replaced by "never fabricated".
- `recommendations`: the stock-filter and popularity wording in the retrieval, cache and cold-start requirements
  is corrected to what the data contract can deliver.

## Impact

- `team-gateway`: `cmd/gateway/main.go`, `internal/edge/{interceptors,server,forward,ai,chat,cockpit,realtime}.go`
  (`realtime.go` deleted), `internal/upstream/clients.go`, `internal/config/{config,config_test}.go`,
  `.env.example`, `README.md`, `FEATURES.yaml`.
- `team-ai`: `app/transport/grpc/servicers/{ai,chat,recommend}.py`, `app/transport/grpc/chat_stream.py` (quota
  key), `app/modules/ai/llm/langfuse.py`, `app/core/redaction.py` (mask hook reuse),
  `app/modules/business/recommend/{backends,service,cache,factory}.py`, `scripts/run_grpc.py`, config and
  `.env.example`, `README.md`, `FEATURES.yaml`.
- `team-identity`: `internal/authz/scopes.go` and its test. `platform-recsys`: `README.md`, tests,
  `recsys/load/qdrant.py` (payload indexes). `team-frontend`: cockpit route handler and `CockpitView.tsx` only (scope limit above); the listing forwarder loses two methods, and no frontend, e2e or tooling caller uses them through the gateway (only unused constants in `platform-e2e/src/constants/gateway_endpoints.py`).
  `platform-e2e`: real AI helpers, new scenarios, Qdrant/Redis seeding helper, a strict-rate gateway profile.
- Contract: no proto change (Rule 4). The new scope is an identity-owned string, not a contract message.
- Data/infra: Qdrant payload indexes (idempotent), Redis keys already produced; no new infra.
- Architecture rules: Rule 1 (frontend still only calls the gateway), Rule 2 (the gateway gates only its own
  endpoints and policy, no business logic; services gate their RPCs), Rule 3 (stock stays a hydration concern),
  ADR-0003/0006 (single verifier, scope checks in services), ADR-0010 (no change to the trust model; this
  reduces blast radius of the open RPCs).

## Non-goals

- No LLM response cache, no guardrails or moderation beyond removing error leakage.
- No AI gateway or vLLM/model-serving work (ADR-0011), no capacity-driven routing.
- No UI redesign and no visual change: the frontend scope is limited to the cockpit same-origin metrics route and
  dropping `EventSource` in `CockpitView` (each unit-tested); no other frontend file is edited by this change.
- No shared/coordinated rate-limit store for the gateway (per-instance buckets stay, documented), no
  service-to-service mTLS (ADR-0010 follow-up), no real order/revenue metric source.
- No new `recs` features (no re-ranking, no stock-aware serving), no change to the ALS training itself.
