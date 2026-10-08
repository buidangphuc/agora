## Context

See `proposal.md` — *Why*. State verified in the code (new work stacks on `team-ai` branch
`feat/llm-path-resilience` and `team-gateway` branch `feat/forward-idempotency-key`):

- **Gateway chain.** `Edge.Interceptors` returns four `connect.UnaryInterceptorFunc`s (request-id, auth, logging,
  rate limit). A `UnaryInterceptorFunc` does nothing for streaming handlers, so `ChatForwarder.StreamChat`
  resolves the principal itself in `Edge.outgoing`, mints its own request id and never hits the limiter. The
  limiter (`rateLimiter`) already keys by `user:<id>` or `ip:<peer>` and is per instance. `/api/track`,
  `/api/events/live` and `/api/admin/metrics` are bare `mux.HandleFunc`s, outside every interceptor.
  `CockpitHandler.ServeHTTP` sets `Access-Control-Allow-Origin: *` itself, which also overrides the `rs/cors`
  wrapper for that route. `team-frontend` calls the gateway server-side with the session token, so the gateway
  peer address of every anonymous SSR call is the frontend container.
- **Upstream dial.** `upstream.Dial` uses `grpc.NewClient` (grpc-go v1.66.0) with default connect parameters and
  fail-fast calls. `callRead` retries `Unavailable` after 50 ms and 100 ms, far shorter than the default
  reconnect backoff (gRPC connection-backoff defaults: base 1 s, factor 1.6, jitter 0.2, max 120 s). That explains the
  measured 503 window (team-domain listening at 15:53:25.72Z, gateway 503 until about 15:53:27; 2 to 4 s from
  recreate): the next dial attempt simply had not happened yet.
- **team-ai servicers.** `RecommendationServicer` and `SearchServicer` already gate with `ensure_scopes`;
  `AIServicer` and `ChatServicer` do not (the chat servicer comment records that `chat:read` is granted to no role).
  `grpc.aio` raises `AbortError` (an `Exception`) from `context.abort`, so the current
  `try/except Exception -> abort(INTERNAL, str(exc))` would also catch a scope abort if one were placed inside it.
  The rate-limit interceptor keys `f"{principal.type}:{principal.id}"`, so anonymous is `anonymous:anonymous`.
  The streamer skips quota when `principal_id` is empty (anonymous) and passes `idempotency_key=request_id`;
  `MemoryQuotaStore.reserve` (and the Postgres/Mongo adapters) return the existing reservation for the same
  `(subject, resource, window, idempotency_key)` without charging again. The request id is the client's
  `X-Request-Id` forwarded verbatim by the gateway.
- **Scopes.** `team-identity/internal/authz/scopes.go` grants `listing.read|write`, `search:read|write`,
  `engagement:read|write` and `admin`. Seller and admin hold `listing.write`, buyer does not; anonymous gets
  `PUBLIC_SCOPES=listing.read,search:read`. JWT TTL is `JWT_TTL_SECONDS=3600`. `order-domain-correctness` adds
  `recommendations:read` to the same table.
- **Recommendation data.** Producer (`platform-recsys/recsys/load/qdrant.py`): point id `uuid5(NS, listing_id)`,
  `NS=6f7a1e2c-9b3d-4c5a-8e21-0d9f4a2b1c00`, payload `{listing_id, model_version, updated_at}`, Cosine, stale
  generations pruned by `model_version`. Redis (`redis_cache.py`): `recs:v1:user:{key}`, `recs:v1:item:{id}`,
  `recs:v1:popular`, `recs:v1:model_version` (flipped last). Consumer reads `user` and `popular` only.
  `team-frontend` already hydrates every returned id through `getListing` and drops missing or `stock <= 0`
  listings, so stock is correctly a hydration concern. `qdrant-client` is 1.19.0 in `team-ai/uv.lock`.
- **Tests that exist.** `platform-e2e/src/api/services/ai_service.py` returns hard-coded `chat_copilot` replies
  and calls `/platform.ai.v1.AiService/GetShoppingAdvice` (wrong service and method names).
  `docker-compose.services.yaml` runs the gateway at `RATE_LIMIT_RPS=1000`, team-ai with `CHAT_BACKEND=mock` and
  `RECS_ENABLED=false`.

## Goals / Non-Goals

**Goals:**
- Every gateway route either authenticates the caller or is deliberately public, and every call shape is logged,
  identified and throttled.
- An AI RPC is reachable only by a principal entitled to it; nothing a caller receives reveals internals.
- A recommendation request against data written by the real producer returns real listing ids.
- Each behaviour change has a test that fails on the old code.

**Non-Goals (design-level):** no shared gateway limiter store; no per-IP bucket inside team-ai (see D7); no
replacement of the trust model (ADR-0010); no re-ranking or stock-aware serving; the gateway does not become a
scope authority for RPCs it only forwards.

## Decisions

### D1 — Cockpit: gate in the gateway with the existing `admin` scope, strip the lies

The gateway owns this HTTP endpoint (it is not a forwarded RPC), so it is the right place to enforce auth: a thin
`requireScope("admin")` wrapper around `CockpitHandler` calls the same `Edge.resolve(header)` the interceptors use
(single verifier, ADR-0006) and answers 401 when the principal is `anonymous`, 403 when `admin` is missing. The
handler's own `Access-Control-Allow-Origin` line is deleted so only the `rs/cors` policy decides (specific origins
plus `AllowCredentials`, which is incompatible with `*` anyway). The response struct makes `TotalOrders24h` and
`TotalRevenue24h` pointer types (`null` in JSON) and `recent_traces` becomes `[]`; `derivedOrders24h`,
`derivedRevenue24h` and `recentTraces()` are deleted. No new scope.
Because the session token lives in an httpOnly cookie, the browser can never attach it: `CockpitView` must stop
fetching `http://localhost:8080/api/admin/metrics` directly and use a same-origin Next route handler
(`/api/admin/cockpit`) that calls the gateway with `getToken()`, exactly like `/api/assistant/stream`. That keeps
Rule 1 (browser to Next to gateway) and is the minimum frontend change the gateway change forces.
*Alternatives:* keep it open but rename to `/healthz`-style (still leaks topology); put a scope check in the
frontend only (bypassable by anyone who can reach :8080); source real order numbers from `team-analytics` now
(a product decision, left as a follow-up).

### D2 — Remove the SSE endpoint instead of protecting it

`HandleSSE`, `RealtimeBroker` and `GlobalBroker` are deleted and the route is dropped from `NewMux`, so the path
falls through to the mux's 404. There is no publisher, so "protect and document" would add an auth/room-ACL
surface (room names like `user:<id>` need per-room authorisation) that protects nothing. `CockpitView` stops
opening `EventSource`; its live-order panel renders an empty state. A real feed later comes as its own change
(Kafka `order.events` consumer feeding an authenticated, room-authorised SSE route) and brings a spec with it.
*Alternative:* keep the route behind `admin` and room allow-lists for the future feed. Rejected: speculative,
untested code on the edge; it is easy to restore from git.

### D2b — Stock RPCs are removed from the edge (defense in depth)

`ListingForwarder.ReserveStock` and `ReleaseStock` are deleted; the embedded `UnimplementedListingServiceHandler`
then answers `unimplemented` (HTTP 501 on Connect) for both paths. They are called only by `team-order` over gRPC,
so removing them changes no legitimate flow. This is the outer layer: `order-domain-correctness` adds the
`inventory.write` gate inside `team-domain` (and makes `team-order` send a service principal), which protects against
any other in-network caller; the route removal protects the public edge even if that gate regresses (ADR-0010: the
gateway must not forward what no browser should call). Callers checked: `team-frontend/src` (none; only a UI string),
`platform-e2e` (only the unused constants `LISTING_RESERVE_STOCK`/`LISTING_RELEASE_STOCK`), `platform-core/tools/*.sh`
(comments only). The listing `CommitReservation` is not routed today. The voucher `CommitReservation` in
`promotion.go` is a different (promotion) RPC and is left alone here (Open Questions). Consequence for
`order-domain-correctness` task 9.3: its "forged `ReserveStock` rejected" e2e must assert the gateway path is
501/`unimplemented` (and keep the in-network `inventory.write` rejection as a `team-domain` test), and any e2e control
that needs a held reservation must create it through checkout (an order left unpaid) or a direct in-network test in
`team-domain`/`team-order`, never through the gateway.

### D3 — One `connect.Interceptor` implementation for unary and streaming

`Edge.Interceptors` returns interceptors that implement the full `connect.Interceptor` interface
(`WrapUnary`, `WrapStreamingHandler`; `WrapStreamingClient` is a pass-through). Shared helpers do the work once:
request-id resolution, principal resolution, key derivation, log emission; only the wrapper shape differs. For
streaming, the request id is written with `conn.ResponseHeader().Set` before the first `Send`, the principal and id
go into the context passed to the handler, and the log line is written in a `defer` with the final Connect code.
`ChatForwarder.StreamChat` keeps calling `Edge.outgoing`, which now finds the principal and request id in the
context, so its duplicate resolution path disappears. The ordering stays request-id, auth, logging, rate limit, then
the stream guards (D4).
Plain HTTP routes get `Edge.HTTPMiddleware(route, opts)` applied per route (`/api/track`, cockpit) rather than around
the whole mux, to avoid double counting Connect paths (they already pass through the interceptors) and to keep
`/healthz` and gRPC reflection exempt. It reuses the same request-id rule, the same `rateLimiter` (key
`user:<id>` when a valid bearer is present, else `ip:<host>`) and emits the same `edge.request` log line.
Request-id validation: accept `^[A-Za-z0-9._-]{1,64}$`, else mint a new id. It matters beyond logs because the id is
forwarded as `x-request-id` and used as a key downstream.
Per-instance buckets and the anonymous-by-IP key are unchanged and documented in the README and the code comment;
note for operators that behind the Next.js server every anonymous SSR call shares the frontend's address (see Open
Questions).
*Alternatives:* a `net/http` middleware for everything (loses Connect procedure names and codes in the log);
per-stream token accounting by message count (overkill; a stream start costs one token).

### D4 — Stream guards: concurrency cap and maximum duration

After the rate-limit token is taken, the streaming wrapper takes a slot from a `map[key]int` guarded by a mutex
(key = principal id, or IP for anonymous; cap `STREAM_MAX_CONCURRENT_PER_PRINCIPAL`, default 4; slot released in
`defer`, including on panic and client disconnect) and derives `ctx` with `STREAM_MAX_SECONDS` (default 120). The
forwarder's upstream `Recv` loop already stops on ctx cancellation; the deadline turns into
`connect.CodeDeadlineExceeded`. The caller's own `Connect-Timeout-Ms` still applies when shorter. The upstream
(team-ai) is additionally capped by `context.time_remaining()` (see `llm-path-resilience`), so both sides agree on
the deadline.
*Alternative:* rate limit only. Rejected: a token bucket bounds starts, not how many hour-long streams one account
holds.

### D5 — AI call policy: `callGenerate` for three RPCs, read policy for the summary

A new `Edge.callGenerate` is `callOnce` with `AI_CALL_TIMEOUT_SECONDS` (default 30) and no retry. `ShoppingAssistant`,
`MagicListing` and `ChatCopilot` move to it: they are user-triggered generation calls, rule-based today and
LLM-backed on the roadmap, so an upstream `Unavailable` must not multiply cost or latency. `SummarizeReviews` stays
on `callRead` because it renders on the public product page and the frontend wrapper has no timeout of its own: a
30 s budget would let a slow team-ai stall every product page view. The "includes LLM-backed calls" comment in
`ai.go` is rewritten to say what is true (today none of the four calls an LLM).
*Alternative:* one policy for all four. Rejected for the page-render reason above.

### D6 — CORS, env gate, dial timeout

`newCORS(origins)` moves out of `main.go` into `internal/edge` so a test can drive a real preflight
(`httptest` `OPTIONS` with `Access-Control-Request-Headers: idempotency-key`) without starting the process;
`Idempotency-Key` is added to `AllowedHeaders`. `TestEnvExampleInSync` is added to `internal/config/config_test.go`
exactly like `team-domain`'s (path `../../.env.example`), and `Makefile`'s `check-env` gains `-count=1` and a
"no tests to run" failure (grep the `-v` output) so the gate cannot silently regress to vacuous again. The
`.env.example` header comment that admits the gap is removed. `DIAL_TIMEOUT_SECONDS` is wired rather than deleted
because it has a natural meaning now: it becomes `MinConnectTimeout` of the connect parameters (D8).

### D7 — team-ai access control: scope table, `ai:use`, deny anonymous

| RPC | Required scope | Why |
|---|---|---|
| `AIService/MagicListing` | `listing.write` | seller feature; seller and admin already hold it, buyer and anonymous do not |
| `AIService/ChatCopilot` | `listing.write` (+ `seller_id` must equal the caller unless `admin`) | seller feature; `seller_id` is client-supplied |
| `AIService/ShoppingAssistant` | `ai:use` | authenticated shopper feature |
| `ChatService/StreamChat` | `ai:use` | the only real-LLM path (cost) |
| `AIService/SummarizeReviews` | `listing.read` | stateless, rendered on the public product page; anonymous keeps it |

`ai:use` is a new scope owned by `team-identity`: added to buyer, seller and admin in `scopes.go`, never to
`PUBLIC_SCOPES`. Reusing `engagement:read` or `search:write` (granted to all signed-in roles today) would avoid an
identity edit but overloads their meaning and ties revoking AI access to favourites or saved searches. The identity
edit is one line per role and is sequenced after `order-domain-correctness` (same file).
The table lives in one module (`app/transport/grpc/scopes.py`), each servicer method calls `ensure_scopes` as its
first statement **outside** the error-mapping wrapper (an abort inside it would be re-mapped), and a unit test
iterates the `AIService` descriptor and fails if any method is missing from the table, so a future RPC cannot ship
ungated.
**Anonymous policy: deny on every LLM-capable RPC** rather than a per-IP bucket. team-ai's peer is always the
gateway and no client address is forwarded, so a per-IP bucket inside team-ai would be one bucket again; forwarding
a client IP needs a new trusted metadata key and a gateway trust-proxy setting. Denial also closes the quota bypass
(anonymous skipped quota by design) without any new code path. The edge limiter still throttles anonymous floods
before they reach team-ai. `RateLimitInterceptor` is left as is; with anonymous denied, the shared
`anonymous:anonymous` bucket only ever throttles callers who would be denied anyway.
*Alternatives:* trust `principal.type == "user"` instead of a scope (ADR-0003 says gate by scope); per-IP
anonymous quota (above).

### D8 — Error mapping: field names out, fixed messages

One `map_servicer_error(exc)` and a decorator used by the four `AIService` methods: `pydantic.ValidationError` to
`INVALID_ARGUMENT` with a message built from `err["loc"]` only (str of a pydantic error includes `input_value`, so
`str(exc)` must never be forwarded); `ServiceUnavailableError` to `UNAVAILABLE`; `grpc.aio.AbortError` re-raised;
everything else to `INTERNAL` with the constant `internal error`. The original is logged with `logger.exception`
and the request id. `ChatServicer.map_stream_error` already uses fixed strings and is left as is.
Request-shape errors that today fail inside the assistant (for example `title_hint` shorter than 2) are covered by
the same mapping because the schemas are constructed inside the wrapped body.

### D9 — Quota idempotency key is server-minted

The streamer passes `uuid4().hex` (per call) as the quota `idempotency_key`; `request_id` stays for tracing and
logs only. Retries within one call (attempt loop, fallback) all share the same call key, so a retry inside the
call does not double charge, which is the only legitimate idempotency need on this path (the gateway does not
retry `StreamChat`). The gateway's request-id validation (D3) is defence in depth, not the fix.
*Alternative:* bind the key to `(principal, client Idempotency-Key)`. Rejected: no client key exists for chat, and
it would reintroduce a client-controlled value.

### D10 — `LLM_TRACE_CONTENT` becomes a content policy for Langfuse too

Keep the name and values; change the meaning from "log text" to "text in any sink". `build_langfuse_tracker`
builds the Langfuse client with the SDK's `mask` hook (supported in the `Langfuse(...)` constructor; the docs also
describe `mask_otel_spans` as an export-stage hook) driven by the same `RedactionPolicy`: `off` replaces every
string in input and output with `[redacted]` (structure, model, usage, latency and status stay), `redacted` applies
`redact_text` recursively, `full` installs no mask. The model input stays masked in `off` (existing
`for_llm_input`). Verification uses an in-memory span exporter and asserts no exported attribute contains the text;
if the LangChain callback path bypasses `mask` in 4.6.1, the fallback is `mask_otel_spans`, decided in task 7.2
by that test (this is the one item in this change that depends on third-party behaviour not yet run).
*Alternatives:* rename to `LLM_LOG_CONTENT` and add `LANGFUSE_CAPTURE_CONTENT` (two knobs to keep in sync, and the
existing name would silently keep its misleading meaning); disable Langfuse callbacks when `off` (loses latency,
usage and error visibility, which are the reasons to run it).

### D11 — Recommendation contract: derive the id, read the payload, read the version, drop fake popularity

- **Id mapping.** The consumer computes `uuid5(NS, seed_listing_id)` with the same constant namespace and queries by
  that point id (recommend/nearest query by id; the exact client call is chosen in the task because the legacy
  `client.recommend` is deprecated in qdrant-client in favour of `query_points`). The namespace is a code constant
  on both sides, not a setting (a setting invites drift), and a golden test pins
  `point_id("listing-1") == 25a4b2d5-6531-5357-90f2-06e92d1e1191` in **both** repos. Payload indexes on
  `listing_id` and `model_version` are created by the producer (idempotent) so the prune filter and any payload
  lookup stay cheap.
- **Result id.** `_hit_to_candidate` reads `payload["listing_id"]`; a hit without it is skipped and counted.
- **Model version.** A small `ModelVersionReader` reads `{prefix}:{schema}:model_version` with a 30 s in-process TTL
  (`RECS_MODEL_VERSION_CACHE_SECONDS`), so the hot path still makes at most one datastore round trip. Preference:
  Redis key, then the contributing ANN hit's payload `model_version`, then `RECS_MODEL_VERSION` (now documented as
  the fallback label).
- **Popularity.** `popular()` on the Qdrant backend is removed from the serving path; the stage reads
  `recs:v1:popular` only and returns `[]` when absent. An arbitrary `scroll` labelled popular is worse than an
  honest empty row (the frontend hides an empty row).
- **Availability.** `in_stock` is honoured when a cache entry or payload carries `false`; the default is no longer
  described as a filter. The producer does not and should not learn stock (the warehouse has no stock signal);
  the frontend already filters on hydration.
- **Not done:** reading `recs:v1:item:{listing_id}` (precomputed similar items) for seed requests. It would be the
  cheaper path and avoids the id mapping, but it changes the retrieval order that `recommendations` specifies; left
  as a follow-up (Open Questions).
- **Entrypoints.** `scripts/run_grpc.py` passes `recommendation_provider=lambda: app.state.resources.recommendation_service`
  exactly as `application.py` does; a test asserts both entrypoints register the service.
- **Tests.** `team-ai` tests build producer-format points (using the same `uuid5` call and payload keys) in
  `QdrantClient(":memory:")` and run the real `QdrantRetrievalBackend` against it, plus `fakeredis` with the
  producer's key names; a fixture file mirrors the producer's JSON shape. `platform-recsys` adds a test that pins
  point id, payload keys and Redis key names.

### D12 — Upstream recovery: tuned backoff plus bounded wait-for-ready on read retries

`Dial` takes options (base, max backoff, dial timeout) and sets
`grpc.WithConnectParams({Backoff{BaseDelay: 100ms, Multiplier: 1.6, Jitter: 0.2, MaxDelay: 1s}, MinConnectTimeout:
DIAL_TIMEOUT_SECONDS})`. The next reconnect attempt after the upstream starts listening is then at most
`MaxDelay x 1.2 = 1.2 s` away plus connect time (local RTT is well under 100 ms), about 0.5 s on average; the
default backoff needs up to its 1 s base growing by 1.6 per failure. Reads: the first attempt stays fail-fast;
the first retry runs with WaitForReady for at most `RECONNECT_WAIT_SECONDS` (default 2), then later retries are
fail-fast as today. WaitForReady is a call option, so a client unary interceptor on each `ClientConn` adds it when
the context carries a marker set by `callRead` (writes never set it). Expected result: a read that arrives during a
restart returns successfully about 0.5 s (worst about 1.3 s) after the upstream listens instead of a 503; writes
still fail fast and succeed on the next attempt once the connection is back (same bound). An outage longer than
`RECONNECT_WAIT_SECONDS` fails a read in about `RECONNECT_WAIT_SECONDS` plus the 0.15 s retry sleeps, not at the full
5 s call deadline.
Keepalive was evaluated and **not** added: a recreated container closes its sockets, so the client learns
immediately and keepalive would not shorten this window; its real use (silent half-open connections) needs pings
more frequent than the servers' enforcement minimum (grpc-go servers reject pings under 5 min by default and answer
`too_many_pings`), so enabling it needs coordinated server settings in 15 services. Recorded as a follow-up.
*Alternatives:* WaitForReady on every call (writes would hang for the deadline during an outage, and Go servers that
are down would accumulate goroutines); raise `RETRY_MAX` (retrying every 50 to 100 ms just burns attempts inside one
backoff window); `grpc.WithBlock` dial (defeats lazy boot, the gateway must start while upstreams are down).

## Risks / Trade-offs

- [Buyers with a token issued before the identity deploy lack `ai:use` and get `PERMISSION_DENIED` on the
  assistant for up to `JWT_TTL_SECONDS`] → deploy identity first, deploy team-ai at least one TTL later (Migration
  Plan) (the frontend is not touched for this; showing a login prompt is a later UI change).
- [Anonymous visitors lose the `/assistant` page's stream] → intended (cost and abuse); the stream route then yields an empty reply for anonymous users; UI handling is out of scope.
- [Removing SSE breaks any external consumer of `/api/events/live`] → repo search found only `CockpitView`, which never
  received an event; route returns 404, restore from git if needed.
- [Null order/revenue tiles change the cockpit's look] → accepted; the numbers were fabricated; the HUD shows "not
  available" until a real source lands.
- [WaitForReady on a read retry adds up to 2 s latency during an outage] → bounded, only after a first failure, only
  for idempotent reads; fails at `RECONNECT_WAIT_SECONDS`, not at the call deadline.
- [Stream cap/duration defaults too tight for real LLM replies] → both configurable; 120 s covers the
  `CHAT_FIRST_TOKEN_TIMEOUT_SECONDS` chain plus a long reply; revisit with production data.
- [Langfuse `mask` may not cover the LangChain callback path] → the span-export test decides between `mask` and
  `mask_otel_spans` before anything else in group 7 is built on it.
- [Namespace constant duplicated across two repos] → the golden-id test in both repos; any change breaks both
  loudly. The alternative (a shared package) would couple two independently deployed Python repos.
- [Dropping the Qdrant popularity scroll can turn a "never empty" row into an empty one when the producer has not
  run] → honest failure; the frontend hides empty rows; the e2e seeds Redis `popular`.
- [Two gateway interceptors logging the same Connect call twice] → the HTTP middleware is applied only to the plain
  routes, asserted by a test that a Connect call produces exactly one `edge.request` line.

## Migration Plan

1. `team-gateway` (independent, deploy first): CORS header, env gate, interceptors for streams/HTTP, cockpit auth,
   SSE removal, AI call policy, tuned dial. Deploy it **with** the frontend cockpit proxy change (step 4) in the same
   release window, otherwise the old cockpit page gets 401. Rollback: revert the image; no data.
2. `team-identity` (after `order-domain-correctness` lands the `scopes.go` edit): add `ai:use` for the three roles.
   Deploy. New tokens carry it immediately.
3. Wait at least `JWT_TTL_SECONDS` (1 h), then deploy `team-ai` with the scope gate, error mapping, quota key,
   trace policy and recommendation contract code. Until then team-ai is unchanged, so nothing breaks in the gap.
4. `team-frontend`: cockpit route handler and `CockpitView`, stream route status passthrough.
5. `platform-recsys`: ship the payload-index change and contract test with the next nightly image; the consumer works
   without the index (slower filters only).
6. Turn on `RECS_ENABLED=true` with `RECS_BACKEND=qdrant` per environment only after a producer run exists.
7. Rollback of team-ai is an image revert (no migrations); rollback of identity leaves `ai:use` granted and unused.

## Decided by the user

- Anonymous callers are denied on LLM-capable RPCs; `ai:use` is not in `PUBLIC_SCOPES` (D7 stands).
- Frontend edits are limited to the cockpit same-origin metrics route and removing `EventSource` from `CockpitView`, with unit tests and no visual change.
- Defaults accepted: SSE removed; cockpit order/revenue tiles "not available"; `SummarizeReviews` stays on the read policy and `listing.read`; an empty popularity row is acceptable; the strict-rate e2e gateway only if CI supports it (else the scenario stays `planned`).

## Open Questions

- Should the voucher `VoucherService/CommitReservation` (and its reserve/release siblings) also leave the edge? It is promotion's internal checkout call, not stock; not changed here.

- Should anonymous visitors keep any AI access (for example a rate-limited `ShoppingAssistant`)? Current decision
  denies all LLM-capable RPCs to anonymous; it is reversible by adding `ai:use` to `PUBLIC_SCOPES`, but then the
  anonymous-abuse and quota-bypass questions return.
- Behind the Next.js server every anonymous SSR call reaches the gateway from the frontend's address, so the edge's
  anonymous bucket is effectively one shared bucket. Fix needs a trusted-proxy/`X-Forwarded-For` policy; out of scope
  here, worth its own change.
- Should `Recommend` also honour `recs:v1:item:{listing_id}` (precomputed similar items) before Qdrant for seed
  requests, and should `user_id` in the request be checked against the caller's principal id (today any caller can
  request another user's precomputed list)? Both are follow-ups, neither changes the tasks below.
