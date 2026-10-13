# team-gateway

The single edge of the Agora marketplace. It serves the platform contracts over **Connect**
(gRPC, gRPC-web and JSON over HTTP/1.1, plus HTTP/2 cleartext via h2c) on `:8080`, verifies the
caller's RS256 JWT, stamps the resolved principal as trusted gRPC metadata and forwards each call
to the owning upstream service. It also hosts a few edge-only HTTP endpoints (`/api/track`,
`/api/events/live`, `/api/admin/metrics`, `/healthz`).

**Bounded context:** edge routing and identity resolution. It owns **no database** and **no
domain logic** (AGENTS.md rules 1-3). Status: deployed service, part of the root
`docker-compose.services.yaml`.

## Contract

### Connect services (forwarded, one handler per service)

The gateway serves 22 services; each RPC is a thin forward to the upstream below. Registered in
`internal/edge/server.go`, dialed in `internal/upstream/clients.go`. Server reflection (v1 and
v1alpha) is served only when `EDGE_REFLECTION_ENABLED=true` (default off; refused in staging/production).

| Contract service(s) | Upstream (env var) |
|---|---|
| identity `AuthService`, `AddressService`, `SessionService` | team-identity (`UPSTREAM_IDENTITY_ADDR`) |
| listing `ListingService` | team-domain (`UPSTREAM_LISTING_ADDR`) |
| search `SearchService` | team-search (`UPSTREAM_SEARCH_ADDR`) |
| engagement `EngagementService` | team-engagement (`UPSTREAM_ENGAGEMENT_ADDR`) |
| order `CartService`, `OrderService` | team-order (`UPSTREAM_ORDER_ADDR`) |
| payment `PaymentService` | team-payment (`UPSTREAM_PAYMENT_ADDR`) |
| chat `ChatService` | team-chat (`UPSTREAM_CHAT_ADDR`); `StreamChat` goes to team-ai (`UPSTREAM_AI_ADDR`) |
| ai `AIService` | team-ai (`UPSTREAM_AI_ADDR`) |
| recommendation `RecommendationService` | team-ai (`UPSTREAM_RECOMMENDATION_ADDR`) |
| promotion `VoucherService`, `FlashSaleService`, `SubscriptionService`, `SponsoredService` | team-promotion (`UPSTREAM_PROMOTION_ADDR`) |
| notification `NotificationService` | team-notification (`UPSTREAM_NOTIFICATION_ADDR`) |
| analytics `AnalyticsQueryService` | team-analytics (`UPSTREAM_ANALYTICS_ADDR`) |
| referral / verification / sharing / audit | team-referral / team-verification / team-sharing / team-audit (`UPSTREAM_*_ADDR`) |

Upstream calls get a deadline (`CALL_TIMEOUT_SECONDS`). Idempotent reads (`callRead`) retry up to
`RETRY_MAX` times on `Unavailable`; writes (`callWrite`) never retry. Upstream gRPC statuses map to
the matching Connect codes.

### Authorization at the edge

For every unary RPC the interceptor chain runs, outermost first: request id, auth, logging,
rate limit (`internal/edge/interceptors.go`).

| Case | Result |
|---|---|
| No `Authorization: Bearer` header | Anonymous principal with `PUBLIC_SCOPES` |
| Bearer verifies (RS256 signature, `exp`, `kid` in JWKS) and its `sid` is not revoked | Principal from the token (`sub`, `typ`, `scopes`) |
| Bearer present but malformed, bad signature, unknown `kid`, expired or session revoked | `401 Unauthenticated`, never downgraded to anonymous |

The gateway checks only the token, not per-RPC scopes. **Scope gates live in the services**; the
gateway forwards `x-principal-id`, `x-principal-type`, `x-principal-scopes` and `x-request-id`,
built fresh from the verified token so a client cannot spoof them. It also forwards audit-only
`x-client-ip` / `x-client-user-agent` (see `TRUSTED_PROXIES`). Inbound `x-client-*` headers are
never copied through. The one edge-level RPC gate is `VerificationService.ReviewKyc`, which needs
an authenticated principal with the `admin` scope (`requireAdmin`, `verification.go`);
team-verification enforces the full rule again.

Rate limiting is a per-instance token bucket (`RATE_LIMIT_RPS` / `RATE_LIMIT_BURST`), keyed by
principal id, or by client IP for anonymous callers. Idle buckets are evicted after 10 minutes.
With N replicas the effective limit is N times higher (TODO ADR-0010).

### Edge HTTP endpoints

| Endpoint | Authorization | Behaviour |
|---|---|---|
| `GET /healthz` | none | `200 ok` |
| `POST /api/track` | none; principal resolved from the `Authorization` bearer or the `session` cookie, anonymous if absent or invalid | Accepts one beacon object or an array of up to 100 (body capped at 64 KiB). An unknown `type`, a malformed body or an empty batch gets `400` and nothing is produced (the batch is validated first). Otherwise each beacon becomes a `TrackingEvent` and the response is `204`, even if the Kafka produce fails (logged). Beacon types and GA4 aliases: `beaconEventTypes` in `collector.go`. |
| `GET /api/events/live?room=<room>` (SSE) | Per room, see below. A browser `EventSource` cannot set headers, so the `session` cookie is honored when `Authorization` is absent. | Sends a `connected` event, then a heartbeat every 15 s, then any message broadcast to the room (nothing broadcasts yet, see Known gaps). The handler sets no CORS headers (no wildcard); CORS comes from the gateway-wide `CORS_ORIGINS` middleware. |
| `GET /api/admin/metrics` (cockpit) | Bearer only. No token or anonymous: `401`. Invalid token: `401`. No `admin` scope: `403`. All checked before any upstream call. | Shapes a fixed payload from three sources, fetched concurrently: Prometheus (fixed PromQL set, `PROMETHEUS_URL`), team-analytics admin RPCs (24 h order total, revenue, recent paid orders) and Jaeger (recent traces, fixed query: service `team-gateway`, last 1 h, limit 5, 2 s timeout). The browser supplies no query. A missing or unreachable source yields `null` figures, `prometheus_available=false` or empty `recent_orders` / `recent_traces`, never fabricated values. |

SSE room rules (`SSEHandler.authorizeRoom`, `realtime.go`):

| Room | Who may subscribe |
|---|---|
| `global`, `listing:<id>` | anyone |
| `user:<id>` | the verified user whose id equals `<id>` |
| `chat:<id>` | any authenticated (non-anonymous) principal |
| `ops:<id>` | principal with the `admin` scope |
| anything else, or an empty id | `403` |

An invalid or expired token gets `401`; an anonymous caller on a protected room gets `401`.

### Consumes

The upstream gRPC services above, team-identity's JWKS over HTTP (`JWKS_URL`), Prometheus and
Jaeger for the cockpit, and `identity.events` (below).

## Events

| Direction | Topic | Type | Key | Notes |
|---|---|---|---|---|
| Produces | `KAFKA_ANALYTICS_TOPIC` (`analytics.events`) | `platform.analytics.v1.TrackingEvent` in an `EventEnvelope` (principal stamped from the edge-resolved identity) | session id, falling back to anonymous id | Best-effort: a failed produce is logged and the beacon still gets `204`. Records carry `traceparent` and `x-request-id` headers (go-sdk `kafkax`). |
| Consumes | `IDENTITY_EVENTS_TOPIC` (`identity.events`) | `platform.identity.v1.SessionRevoked` in an `EventEnvelope`; other types ignored | n/a | Feeds the session denylist. |

Envelope and topics follow ADR-0002.

### Session revocation

`internal/revocation` consumes `SessionRevoked` into an in-memory denylist of session ids. After
local JWT verification the edge rejects (as `401`, same as any invalid token) a token whose `sid`
claim is denylisted. There is no call to identity.

- No consumer group: every replica reads every partition from the earliest offset, so a restart
  rebuilds the list. Topic retention must be at least the token TTL.
- An entry lives until the event's `expires_at` (24 h if absent), then is pruned.
- **Fail open.** The consumer always runs; there is no enable flag. With Kafka down the gateway
  still starts, revoked tokens stay valid until they expire and revocations are not enforced
  (logged, and `gateway_revocation_consumer_up` is `0`).
- OTel gauges (exported only when `OTEL_ENABLED=true`): `gateway_revocation_consumer_up`
  (alert on `0`), `gateway_revocation_consumer_lag`, `gateway_revocation_denylist_size`.
- Tokens without a `sid` (service tokens) are never checked against the denylist.

## Layout and go-sdk (ADR-0016)

```
cmd/gateway/main.go   signal context, config.Load, app.Run, exit code
internal/app/         composition root (const service = "team-gateway"); tests build Settings by hand
internal/config/      Settings (SDK groups + gateway groups), Validate, Load; its test checks .env.example
internal/edge/        Connect handlers, policy, auth resolution, rate limits, collector, cockpit, SSE
internal/events/      analytics producer (kafkax producer behind AnalyticsPublisher)
internal/revocation/  identity.events consumer and session denylist
internal/token/       RS256 JWT verification (jwtx wire types), JWKS cache
internal/upstream/    gRPC clients to the upstream services
third_party/go-sdk    vendored go-sdk/v1.0.0
```

`app.Run` builds, in order: logger (`logx`), telemetry (`otelx`: tracer, meter, W3C propagator),
upstream clients, JWKS verifier, edge, analytics producer (`kafkax`), revocation consumer, HTTP
server, then `lifecycle.Run`. The HTTP server is the primary component of `lifecycle.StopAfter`;
the revocation consumer and the trusted-proxy refresher are its dependents. On SIGINT/SIGTERM the
server stops accepting and drains in-flight requests for up to `SHUTDOWN_GRACE_SECONDS`, then the
dependents stop, then the closers run in reverse order (analytics producer, upstream connections,
telemetry flush).

Not adopted, and why:

- `grpcx` and `auth` interceptors: the edge serves Connect over HTTP, and verifies the JWT itself
  to forward `x-principal-*` (AGENTS.md rule 5).
- `healthx`: it flips a gRPC health status on dependency checks. `/healthz` is a plain HTTP `200`
  that deliberately ignores dependencies (the edge fails open without Kafka or JWKS).
- `upstream.NewClient`: its interceptors rewrite the outgoing principal metadata from the gRPC
  server context (and drop it when there is none), bound every unary call at 2 s and balance with
  round_robin. The edge builds `x-principal-*` itself (anonymous callers included), sets per-RPC
  deadlines (`CALL_TIMEOUT_SECONDS`, `AI_CALL_TIMEOUT_SECONDS`) and caps the reconnect backoff
  (`DIAL_TIMEOUT_SECONDS`), so `internal/upstream` stays.
- `lifecycle.HTTP`: when the grace period runs out it returns the shutdown deadline error, which
  exits the process non-zero; the edge's own `serve` closes the leftover connections and exits 0,
  and treats a zero grace as "close at once" (the SDK reads it as 10 s).
- The JWKS cache, the rate limiter and the revocation denylist are edge-only and stay local.

Adopted from the SDK: `auth` (principal metadata keys, `TypeAnonymous`), `idempotency`
(`MDKey`, `Validate`), `reqctx` (request-id key), `jwtx` (claims and JWKS wire types, shared with
team-identity), `resilience` (`callRead`: retry only on `Unavailable`, 50 ms doubling backoff with
jitter), `kafkax` (producer and the group-less `NewConsumerClient("")` of the revocation
consumer), `config` (`Runtime.IsStrict`).

## Data

None. The gateway has no database, no migrations and no local state beyond in-memory structures
(JWKS cache, rate-limit buckets, session denylist, SSE room registry).

## Configuration

Loaded by `config.Load` (`internal/config/config.go`, go-sdk `config.Load[Settings]`) with the
listed defaults: the SDK groups `Runtime` (`ENV`, `LOG_*`), `Kafka` (`KAFKA_BROKERS`) and `OTel`
(`OTEL_*`) next to the gateway's own groups. `KAFKA_BROKERS` defaults to `localhost:9092` only
when `ENV` is `local` or `test`; in a strict `ENV` it is required. `.env.example` mirrors them; `make check-env`
(`TestEnvExampleInSync`) fails if the two drift in either direction. This table is not covered by
that gate.

| Variable | Default | Meaning |
|---|---|---|
| `ENV` | none, **required** | `local` or `test` for development; `staging`, `stage`, `prod`, `production` are strict (reflection refused, `KAFKA_BROKERS` required) |
| `LOG_LEVEL` | `info` | Log level |
| `LOG_JSON` | `true` | JSON logs |
| `HTTP_HOST` | `0.0.0.0` | Listen host |
| `HTTP_PORT` | `8080` | Listen port, must be 1-65535 |
| `SHUTDOWN_GRACE_SECONDS` | `10` | Drain time on SIGINT/SIGTERM |
| `UPSTREAM_SEARCH_ADDR` | `localhost:50052` | Required non-empty |
| `UPSTREAM_LISTING_ADDR` | `localhost:50051` | Required non-empty |
| `UPSTREAM_IDENTITY_ADDR` | `localhost:50053` | Required non-empty |
| `UPSTREAM_ENGAGEMENT_ADDR` | `localhost:50054` | |
| `UPSTREAM_ORDER_ADDR` | `localhost:50055` | |
| `UPSTREAM_PAYMENT_ADDR` | `localhost:50056` | |
| `UPSTREAM_CHAT_ADDR` | `localhost:50057` | |
| `UPSTREAM_AI_ADDR` | `localhost:50060` | team-ai; also used for `StreamChat` |
| `UPSTREAM_RECOMMENDATION_ADDR` | `localhost:50060` | team-ai |
| `UPSTREAM_PROMOTION_ADDR` | `localhost:50061` | |
| `UPSTREAM_NOTIFICATION_ADDR` | `localhost:50058` | |
| `UPSTREAM_ANALYTICS_ADDR` | `team-analytics-svc:50059` | |
| `UPSTREAM_REFERRAL_ADDR` | `team-referral-svc:50062` | |
| `UPSTREAM_VERIFICATION_ADDR` | `team-verification-svc:50064` | |
| `UPSTREAM_SHARING_ADDR` | `team-sharing-svc:50065` | |
| `UPSTREAM_AUDIT_ADDR` | `team-audit-svc:50066` | |
| `DIAL_TIMEOUT_SECONDS` | `2` | Cap of the upstream reconnect backoff and min connect timeout |
| `TRACK_RATE_LIMIT_RPS` / `TRACK_RATE_LIMIT_BURST` | `5` / `20` | `/api/track` bucket per user or IP |
| `AI_CALL_TIMEOUT_SECONDS` | `30` | Single-attempt deadline of the four AI generation RPCs |
| `STREAM_MAX_REQUEST_BYTES` | `16384` | Max `StreamChat` request message |
| `STREAM_REVOCATION_CHECK_SECONDS` | `5` | How often an open stream re-checks its session against the revocation denylist; a revoked session or expired token ends it with `unauthenticated` (also an authenticated `/api/events/live` SSE room: final `event: unauthenticated`, then close) |
| `EDGE_REFLECTION_ENABLED` | `false` | gRPC reflection; refused when `ENV` is staging/production |
| `JWKS_URL` | none, **required** | team-identity `/.well-known/jwks.json` (`.env.example` uses `http://localhost:50063/...`) |
| `JWKS_CACHE_TTL` | `300` | Seconds, must be > 0. Also refreshed on an unknown `kid` (at most once per 10 s). |
| `PUBLIC_SCOPES` | `listing.read,search:read` | Scopes of anonymous callers (comma-separated) |
| `RATE_LIMIT_RPS` | `20` | Per principal or per IP |
| `RATE_LIMIT_BURST` | `40` | |
| `CALL_TIMEOUT_SECONDS` | `5` | Deadline per unary upstream call (not applied to `StreamChat`) |
| `RETRY_MAX` | `2` | Retries for idempotent reads on `Unavailable` (50 ms backoff, doubling, with jitter) |
| `CORS_ORIGINS` | `http://localhost:3000` | Comma-separated allowed origins; credentials allowed; methods GET, POST, OPTIONS |
| `PROMETHEUS_URL` | `http://prometheus:9090` | Cockpit. Empty disables the source. |
| `JAEGER_QUERY_URL` | `http://jaeger:16686` | Cockpit traces. Empty disables the source. |
| `JAEGER_UI_URL` | `http://localhost:16686` | Browser-facing base for trace links |
| `TRUSTED_PROXIES` | empty | CIDRs, bare IPs or hostnames whose `X-Forwarded-For` is believed. Hostnames are re-resolved every 30 s; an unresolvable name trusts nobody. Empty trusts nobody: `x-client-ip` is the socket peer. The client is the rightmost XFF entry that is not itself trusted. Audit data only, never authorization. A malformed entry fails startup. |
| `KAFKA_BROKERS` | `localhost:9092` for a local or test `ENV`, **required** in a strict one | Comma-separated; used by the analytics producer and the revocation consumer. Kafka has no enable flag: every deploy runs it. |
| `KAFKA_ANALYTICS_TOPIC` | `analytics.events` | |
| `IDENTITY_EVENTS_TOPIC` | `identity.events` | Revocation source |
| `OTEL_ENABLED` | `false` | Tracer and meter providers; also needed for the gRPC RED metrics and revocation gauges |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty (`.env.example` sets `http://localhost:4317`) | |
| `OTEL_SERVICE_NAME` | empty, falls back to `team-gateway` | |

The root compose file sets `TRUSTED_PROXIES=team-frontend-svc`, `KAFKA_BROKERS=redpanda:9092` and
`RATE_LIMIT_RPS=1000` (burst 2000). A leftover `KAFKA_ENABLED` in an old environment is ignored.

## Run locally

Root compose (from the repo root; the gateway publishes `8080:8080` and fetches JWKS from
`team-identity-svc:50063`):

```bash
docker compose -f docker-compose.services.yaml up -d --build team-gateway
```

The gateway is not in the `jobs` profile. Generate code before building the image (see Gotchas):
the Dockerfile `COPY`s the working tree and there is no `.dockerignore`.

Standalone (needs reachable upstreams and a JWKS endpoint):

```bash
cp .env.example .env     # .env is gitignored; the Makefile loads it
make proto               # once after clone / proto change
make run                 # go run ./cmd/gateway
```

## Build, test and lint

| Command | What it does |
|---|---|
| `make proto` | `buf generate` into `generated/` (needs the `buf` binary and network access to buf.build for the remote plugins in `buf.gen.yaml`, which include the Connect plugin; no Docker) |
| `make proto-local` | The same with the locally installed plugins of `buf.gen.local.yaml` (absolute paths under `~/go/bin`), for when buf.build rate-limits |
| `make test` | `go test -race ./...` |
| `make lint` | `go tool golangci-lint run ./...` with the synced `.golangci.yml` |
| `make check` | **Merge gate**: `go vet`, `lint`, race tests with `GO_SDK_REQUIRE_DOCKER=1`, `check-env` |
| `make check-env` | `.env.example` vs `Settings` drift test |
| `make docker` | `make proto`, then `docker build -t team-gateway:dev --build-arg BIN=gateway .` |

Go 1.26 (`go.mod`, Dockerfile builder `golang:1.26`); `go.mod` is the source for dependency
versions. This repo has no CI workflow of its own; run `make check` before opening a PR.

## Spec and verification

- Feature manifest: `FEATURES.yaml` (4 features, all `automated`: three `tracking.*` for
  `/api/track` and `gateway.client-ip-not-spoofable`). Session revocation and client-IP behaviour
  are covered by `platform-e2e` (`auth/session_revocation.feature`).
- The runtime features `gateway.reflection-rejected-in-prod`, `gateway.shutdown-drains-in-flight`
  and `gateway.existing-suites-stay-green` are `not-testable` (no UI) and unit-verified in
  `internal/config`, `internal/app` and by `make check`.
- E2E coverage is verified in `platform-e2e`: `make -C platform-e2e features-check` and
  `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC
  (propose, apply with the `spec-dispatch` / `spec-to-e2e` skills, e2e, archive).

## Gotchas

- `generated/` is gitignored. Regenerate with `make proto` after a clone or a proto change, and
  never hand-edit it (AGENTS.md rule 4). `make proto-local` uses `buf.gen.local.yaml`, whose
  absolute local plugin paths are machine-specific.
- `third_party/go-sdk` is the vendored go-sdk (`VERSION` names the tag, now `go-sdk/v0.2.1`), with
  `make/go-service.mk` and `.golangci.yml` copied from it. Never edit them here; upgrade with
  `platform-core/tools/sync-go-sdk.sh <tag> team-gateway` and review the diff. The script's
  `go mod tidy` drops the SDK `require` while nothing imports it; the gateway imports it, so the
  line `github.com/buidangphuc/platform-core/packages/go-sdk v0.0.0-00010101000000-000000000000`
  stays in `go.mod`.
- `proto/` is vendored from platform-core (ADR-0001). Never edit it here; change the contract in
  platform-core, then re-vendor and regenerate.
- `coverage.out` and `coverage.summary` in the repo root are test artifacts, not source.
- With the broker unreachable, `/api/track` still returns `204` (the failed produce is logged) and
  revoked sessions stay valid until the token expires.
- `JWKS_URL` is required: startup fails without it. An unreachable JWKS at boot is only a
  warning; the cache fills lazily, but requests carrying a bearer token get `401` until it does.
- The gateway only verifies tokens. Never assume it enforces an RPC's scope; check the service.
- State is per replica: rate-limit buckets, the denylist and the SSE registry are in memory.

## Known gaps

- **`StreamChat` bypasses the interceptor chain.** The chain is built from unary interceptors
  (`connect.UnaryInterceptorFunc`), whose streaming wrappers are pass-throughs. The server-streaming
  `ChatService.StreamChat` therefore gets no rate limiting, no request log and no client context
  (`x-client-ip` is not forwarded), and no call timeout. It still resolves the token through the
  `outgoing` fallback: an invalid token forwards no scopes, and a valid one is checked against the
  revocation denylist.
- **Nothing publishes into `GlobalBroker`.** `RealtimeBroker.Broadcast` has no caller, so
  `/api/events/live` currently delivers only the `connected` handshake and heartbeats, for every
  room. The room list in the code comments (flash-sale stock, chat, notifications, `ops:orders`)
  describes intent, not a wired feed.
- The `session` cookie is honored by `/api/track` and `/api/events/live` but not by Connect RPCs
  or `/api/admin/metrics`, which read only the `Authorization` header.
- Revocation is fail-open and per replica. Edge-side admin gating is the `adminProcedures` map
  in `policy.go` (`ReviewKyc`, `ResolveDispute`, `QueryAuditLog`, `ForceFailSaga`).
- The cockpit roster (`cockpitRoster` in `cockpit.go`) hardcodes ten services and their ports; the
  newer services (promotion, analytics, referral, verification, sharing, audit) have no row.
- Upstream connections use insecure transport credentials (ADR-0010 zero-trust is not applied at
  this hop).

## Links

- [`AGENTS.md`](../AGENTS.md) for the rules (esp. 1, 2, 4 and 5) and the add-a-feature recipe.
- ADRs in `platform-core/docs/ADR/`: 0001 proto distribution, 0002 async broker, 0003 auth model
  (and its addendum on revocation and client context), 0004 observability, 0006 RS256/JWKS auth,
  0010 service zero-trust.
