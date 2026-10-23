# Design

## Decisions

### D1 Composition root and layout

`cmd/gateway/main.go` is `signal.NotifyContext` + `config.Load` + `app.Run(ctx, settings)` only (binary name `gateway` stays, so the Dockerfile entrypoint and the compose build keep their names; `BINARIES := gateway`). `internal/app.Run` builds in the order of `examples/hello`: `logx.New`, `otelx.Init`, `mockdata.New` + `LogActive`, `healthx.New`, token verifier, Kafka producer, upstream connections, edge, revocation consumer, mux (with the health handler), `lifecycle.Run`. Everything opened goes on a `lifecycle.Stack`; closers run in reverse on shutdown and on an early failure.

The Connect handler layer stays in `internal/edge`. The ADR layout names `internal/rpc` for handlers, but the gateway has no gRPC server: its handlers are Connect forwarders, policy tables and interceptors (about 13 000 lines and 150 tests that name `internal/edge`). Renaming the package would change nothing observable and would break every existing VERIFIED BY citation. Rejected: `git mv internal/edge internal/rpc`.

### D2 Settings: SDK groups for the listener, edge keys unchanged

`Settings` = `Runtime`, `GRPC`, `Kafka`, `OTel`, `MockData` (SDK groups) plus the service groups `Upstream`, `Auth`, `Edge`, `Events`. The listener uses `config.GRPC` (`GRPC_HOST`, `GRPC_PORT` required, `SHUTDOWN_GRACE`, `SHUTDOWN_DRAIN_DELAY`, `GRPC_REFLECTION_ENABLED`) because the edge serves the gRPC protocol (Connect speaks gRPC, gRPC-web and JSON on one port) and because that group carries the strict-ENV reflection refusal that runs before validation. Cost: `HTTP_HOST`, `HTTP_PORT`, `SHUTDOWN_GRACE_SECONDS`, `EDGE_REFLECTION_ENABLED` are renamed (deploy track and the e2e private-gateway helper follow). A retired key that is still set is ignored, never mapped; in particular a leftover `EDGE_REFLECTION_ENABLED=true` cannot turn reflection on.

The other edge keys keep their name and unit (`CALL_TIMEOUT_SECONDS`, `AI_CALL_TIMEOUT_SECONDS`, `STREAM_REVOCATION_CHECK_SECONDS`, `DIAL_TIMEOUT_SECONDS`, `JWKS_CACHE_TTL` as integer seconds). The SDK rule "durations are Go durations" governs the SDK groups; converting these would touch specs that name them (`edge-stream-and-http-policy`, `edge-route-policy`), the e2e steps that set `STREAM_REVOCATION_CHECK_SECONDS=1`, and the compose value `JWKS_CACHE_TTL=300`, which would stop parsing. The one new key introduced here is a Go duration (`TRACK_PUBLISH_TIMEOUT`). Rejected: renaming every key for uniformity.

`keepempty:"true"` marks `PROMETHEUS_URL`, `JAEGER_QUERY_URL`, `JAEGER_UI_URL` (an explicit empty value disables the call, as today) and every `UPSTREAM_*_ADDR` (an explicit empty value is reported by `Validate` instead of silently taking the default). `MockData` is declared with zero adapters: the gateway has no data source to swap, but the declaration keeps the strict-ENV refusal uniform and `mockdata.New(cfg)` with no known name rejects any non-empty value at boot.

`LogKeyFP` and `Audit` are not declared: the gateway logs no record key and writes no audit event.

### D3 Kafka producer and revocation consumer

No `KAFKA_ENABLED`. `app.Run` builds `kafkax.NewProducer(s.Kafka)` and the revocation reader `kafkax.NewConsumerClient(s.Kafka, "", []string{s.Identity.Topic})` always. An empty group is the SDK's group-less reader (all partitions, from the start, nothing committed), which is exactly the revocation contract (ADR-0003 addendum: every replica rebuilds the full denylist). `consume.Consumer` is not used: it needs a group, commits and a DLQ, none of which fit.

`events.KafkaPublisher` keeps `PublishTrackingEvent` and the `EventEnvelope` wrapping; it calls `kafkax.Producer.PublishRecord` (the record gains `traceparent` and `x-request-id` headers) instead of `kgo.ProduceSync`. `NoopPublisher` is deleted. A produce failure is still logged and swallowed by `HandleTrack` (tracking is best-effort by requirement), but the call is bounded by `TRACK_PUBLISH_TIMEOUT` (default `2s`, `context.WithTimeout` around the publish) because the SDK delivery timeout is 30 s, far too long for a browser request.

Catch-up decision (human, round 2): readiness waits until the revocation consumer has read up to the end offset of every partition, captured once when the consumer starts (`kadm.NewClient(client).ListEndOffsets` over the client from `kafkax.NewConsumerClient`; kadm ships in the franz-go module already in the graph, no new module). An empty topic has end offset 0 and is caught up as soon as the offsets are known. The `revocation` check is a latch: an error until caught up, nil afterwards even when the broker later fails, so a broker outage never takes a ready pod out of the Service. Until the offsets can be listed (broker down, topic missing, ACL denied) the check keeps failing and the pod stays NOT_SERVING; that is intended, because serving without the denylist is a security gap. Records produced after the captured offset are applied as they arrive and do not delay readiness.

The consumer loop keeps its structure (`Run` retries every 5 s, `probeLoop` pings every 10 s, `pruneLoop` every minute, gauges `gateway_revocation_consumer_up`, `_lag`, `_denylist_size`). It is now a `lifecycle.Component`, so shutdown cancels it and its client is closed. A decode failure logs partition and offset only.

### D4 Health over Connect

The SDK health server is a `grpc.health.v1` server. The gateway has no `grpc.Server`, so `internal/app/health.go` mounts a Connect unary handler for `/grpc.health.v1.Health/Check` built with `connect.NewUnaryHandler` over the `grpc_health_v1` messages already in the module graph (no new module), delegating to `health.Server().Check`. An unknown service returns `NotFound` (what the SDK server returns). `Watch` is not served: Kubernetes gRPC probes use `Check`. A Connect handler under `h2c` speaks the gRPC protocol over cleartext HTTP/2, so a Kubernetes `grpc:` probe and `grpc.NewClient` work against port 8080 (tested).

Checks registered: `revocation` (the catch-up latch of D3; the consumer state is also reported through the `gateway_revocation_consumer_up` gauge) and `jwks` (`verifier.Check`: an error until the first successful fetch, then nil even if later fetches fail, because cached keys keep verifying). There is no `kafka` check (`kafkax.Checker` is not used) and no Postgres check. Readiness is the overall status, `liveness` is the SDK name and is independent. `lifecycle.Options.OnShutdown = health.Drain`.

`GET /healthz` stays a process-only 200. It is what compose and the existing e2e wait on, and it keeps the deploy-runtime scenario that names it valid. The deploy track moves the Kubernetes probes to gRPC on the container port with `probes.livenessService: liveness` (the chart makes `grpcPort` and `healthPath` mutually exclusive).

The health handler is mounted on the mux by path before the edge chain: it does not pass the bearer resolution, the per-IP rate limit (probes from one node IP would otherwise count against `RATE_LIMIT_RPS`), the procedure policy tables (an unregistered procedure is denied there) or the request-id interceptor. It returns only the SERVING or NOT_SERVING status.

Kafka is deliberately not a readiness check (human, round 2): once the denylist has caught up, a broker outage leaves every replica in the Service, and only telemetry, new revocations and beacons degrade. Before catch-up, readiness needs the broker. The gateway has no database, so there is no `migrate up` subcommand and no migrate service to add.

### D5 JWT verification

The SDK has wire types, not a verifier (`jwtx` holds `Claims`, `JWK`, `JWKS`). `internal/token` keeps `Verifier`, the kid-keyed TTL cache, the one forced refresh per 10 s on an unknown kid, RS256-only, `exp` and `sub` required. It adopts `jwtx.JWKS`/`jwtx.JWK` for decoding (the local `jwk` and `jwksDoc` types go) and `jwtx.Claims` for the claim fields behind a small `jwt.Claims` adapter (`golang-jwt` needs the getter methods). `Verifier.Check` feeds readiness (D4) and never fetches. Boot does not fail when identity is unreachable: `app.Run` calls `Verifier.Prime` at boot (5 s bound, warn on failure, as `main.go` does today) and, until the first successful fetch, a background `lifecycle.Func` component (`primeJWKS`) retries `Prime` every 5 s, then returns. Without that driver readiness would deadlock: a pod that starts before identity is NOT_SERVING, receives no traffic, so no bearer request ever triggers the lazy fetch. The lazy fetch on an unknown `kid` stays for rotation. Rejected: failing boot on an unreachable JWKS (it would restart-loop pods that start before identity); relying on the lazy fetch alone (deadlock above).

### D6 Upstream calls: SDK retry yes, breaker no, SDK client no

`callRead` becomes `resilience.Do(ctx, fn, retry)` with one policy: `retry` is `resilience.Retry(resilience.RetryOptions{MaxAttempts: RETRY_MAX+1, BaseDelay: 50ms, MaxDelay: 200ms, RetryIf: code == Unavailable})`. The SDK default `Retryable` also retries `ResourceExhausted`, `Aborted` and `DeadlineExceeded`, which would turn one slow read into three slow reads and retry upstream rate limiting, so the existing Unavailable-only rule is passed explicitly. There is NO circuit breaker (human, round 2): no `resilience.Breaker`, no `UPSTREAM_BREAKER_*` keys, no `resilience.ErrOpen` mapping. Whose deadline counts: the retry predicate returns false when the parent context is done (cancelled, or a caller deadline such as `Connect-Timeout-Ms` shorter than `CALL_TIMEOUT_SECONDS` expired), and `resilience.Do` stops as soon as the caller's context is done, including between attempts; the attempt's own `CALL_TIMEOUT_SECONDS` expiry is `DeadlineExceeded` and is not retried. Every attempt still calls `noteAttempt`, so the `attempts` log field stays true. `callWrite` and `callAI` are unchanged (one attempt).

Without a breaker a recreated upstream is reachable on the next read, which satisfies the requirement "a recreated upstream is reachable again within 5 seconds" through the existing bounded reconnect backoff.

`upstream.NewClient` is not used. Its interceptor deletes `x-principal-*` for an anonymous caller (the gateway must forward the anonymous principal with its public scopes), deletes metadata it did not set (the gateway sets principal and client context itself), applies a 2 s default bound with a 500 ms margin under the parent deadline (a `CALL_TIMEOUT_SECONDS=0.1` test would expire at once) and replaces the principal with a service principal. The gateway is the issuer of the principal, not a service caller. `internal/upstream.Dial` keeps `grpc.NewClient` with `otelgrpc` and the bounded reconnect backoff; it takes the metadata names from the SDK (`auth.MDPrincipalID`, `MDPrincipalType`, `MDPrincipalScopes`, `reqctx.MetadataRequestID`, `idempotency.MDKey`) and `idempotency.Validate` replaces the local key check (same 1 to 255 printable ASCII rule, same client message). The local request-id rule `^[A-Za-z0-9._-]{1,64}$` stays: `edge-route-policy` fixes 64 characters while `reqctx.ValidRequestID` allows 128.

### D6a Idempotency-Key

`idempotency.Validate` replaces `validIdempotencyKey`; `outgoingWithIdempotencyKey` keeps its wrapper: no header adds nothing, a repeated header or an invalid trimmed value is `invalid_argument` with the fixed message, the trimmed value is forwarded. It applies to `CreateOrder` only, as today.

### D7 Logging and telemetry

`logx.New(s.Runtime, "team-gateway")` replaces `observability.NewLogger`; the edge interceptors put the request id and the principal id on the context with `reqctx.WithRequestID` and `auth.WithPrincipal` (in addition to the existing private context keys) so records written with a context carry `request_id`, `principal_id` and `trace_id`. The `edge.request` line keeps `method`, `principal`, `code`, `latency_ms`, `request_id`, `attempts`; `logx` does not duplicate a key a record already names. `otelx.Init(ctx, s.OTel, "team-gateway")` replaces both `InitTracer` and `InitMeter`; the otelgrpc client handler on the upstream dials is kept, and the revocation gauges are registered on the same global meter.

### D8 Lifecycle

Components: the HTTP server (`lifecycle.HTTP(srv, grace)` with `grace = ShutdownGrace - CloseGrace`, the SDK's guidance for a component with its own timer), the trusted-proxy refresher (`lifecycle.Func` around `TrustedProxies.Run`), the revocation consumer, `health.Run`. Closers: producer, upstream connections, telemetry. A listener error ends `Run` with that error; long-lived Connect streams and SSE connections that outlive the grace are closed by `lifecycle.HTTP` (same as the former `srv.Close()` fallback).

### D9 Tests and fakes

Fakes live in `_test.go` files or `internal/events/eventstest` (a recording publisher); a test asserts the dependency graph of `./cmd/gateway` contains no package whose path ends in `test` or `fake`. Real-broker tests use the Redpanda container of `testkit` (`Produce`, `ReadAll`, `Eventually`); JWKS tests keep their `httptest` server (the real HTTP fetch path). Moved or kept tests keep their names so scenarios in `openspec/specs/*` that cite them still resolve.

### D10 Lint

The SDK `.golangci.yml` disables the revive doc-comment rules, so new code carries no doc comments and existing package comments are left alone. `golangci-lint` is pinned with a `tool` directive like the SDK `go.mod`. goimports `local-prefixes` stay `github.com/buidangphuc`.

### D11 Dockerfile

The template `platform-core/templates/go-service/Dockerfile` still copies `third_party/`, which no longer exists, so the gateway keeps its own Dockerfile and changes only `ARG BIN=gateway`, `GRPC_PORT` (default 8080, replacing `HTTP_PORT`) and the build line. `generated/` is gitignored; a worktree build needs it copied from the main checkout first.

## Clarifications

### Round 1 — 2026-10-23

- G-sdk [dependency] Target version: Which go-sdk version and how is it consumed? → **`go-sdk@v1.0.2` required by tag, no `replace`, no `third_party`; Makefile includes `../platform-core/make/go-service.mk` and the SDK `.golangci.yml` (no copies); no doc comments; `golangci-lint` pinned as a go tool like the SDK `go.mod`** (human, recipe)
- G-layout [structure] Layout: Which structure? → **ADR-0016: `cmd/<bin>/main.go` calls `internal/app.Run(ctx, settings)`; `internal/config`, `internal/rpc`, domain packages, `internal/store`, `internal/events` if Kafka** (human, recipe)
- G-flags [config] Enable flags: Which stay? → **Delete every dependency enable flag (`KAFKA_ENABLED` here) and every runtime in-memory or noop fallback (`NoopPublisher`); in-memory implementations stay only as test fakes; kept flags are `OTEL_ENABLED`, `GRPC_REFLECTION_ENABLED` (off by default, refused in strict ENV) and, for identity only, `SEED_ADMIN_ENABLED`** (human, recipe)
- G-mock [mock] Mock mode: What does mock mean? → **Mock data served by the real service through `pkg/mockdata` (`MOCK_DATA=<adapters>`, refused in strict ENV before validation); never a mock service; the real code path always runs** (human, recipe)
- G-sdk-use [behaviour] SDK coverage: What does the service use from the SDK? → **config, logx, otelx, healthx (readiness overall, liveness independent, `probes.livenessService: liveness`), lifecycle, grpcx (error table, codes), auth, kafkax/consume (key_fp HMAC logging with `LOG_KEY_FP_SECRET`, DLQ), upstream, resilience, idempotency, jwtx where relevant; these behaviour changes are accepted** (human, recipe)
- G-keep [behaviour] Existing behaviour: What wins on conflict? → **Main's behaviour and every Q1-Q11, C4 and privacy-hardening behaviour stay green; the old migration on `origin/feat/go-sdk-v1-e2e` is reference only; only requirements about removed flags, fallbacks and env vars are MODIFIED, new capability requirements are ADDED where no spec describes the service** (human, recipe)
- G-dpo [privacy] DPO sign-off → **Not needed: basis legitimate interest, platform log retention** (human, recipe)
- G-e2e [process] e2e timing → **Not run per service; once at the end. Compose and gitops edits are listed in `tasks.md` under the deploy track, not done here** (human, recipe)

### Round 2 — 2026-10-23

- R2-1 [migrations] Every service gets a `migrate up` subcommand (SDK `pkg/migrate`, embedded migrations), never migrates at boot, compose keeps a `<svc>-migrate` service, gitops gets an Argo PreSync migrate Job per service (deploy-go-sdk-v1). → **Not applicable to the gateway: it holds no database, so no `migrate up` subcommand, no migrate service and no PreSync Job are added for it** (human, accepted)
- R2-2 [health] Kafka as a readiness check? → **Kafka is NOT a readiness check in any service (readiness = Postgres and required upstream checks only); the gateway has no `kafka` check and reports consumer state through `gateway_revocation_consumer_up`; its readiness is the JWKS fetch plus the revocation catch-up latch of R2-10** (human)
- R2-3 [specs] Shared deploy-runtime requirements → **MODIFIED only by change deploy-go-sdk-v1; the MODIFIED copy of "Services refuse unsafe configuration in staging and production" is deleted from this change and the team-gateway bullet is listed under the Deploy track in `tasks.md`** (human)
- R2-4 [outbox] Outbox upgrade → **Not applicable to the gateway (no outbox, no database)** (human, accepted)
- R2-5 [deploy] Raw gitops manifests and `LOG_KEY_FP_SECRET` → **Not applicable to the gateway (Helm values, no record-key logging, no `LOG_KEY_FP_SECRET`)** (human, accepted)
- R2-6 [upstreams] Upstream addresses and clients are required in every ENV, no fallback modes → **All sixteen `UPSTREAM_*_ADDR` are required in every ENV (an empty value is refused by `config.Load`); the gateway has no fallback scenario in `openspec/specs` to remove** (human)
- R2-7 [analytics] ga_client_id redaction before DLQ → **Not applicable to the gateway** (human, accepted)
- R2-8 [reflection] Local compose → **Local compose sets `GRPC_REFLECTION_ENABLED=true` for every service, the gateway included (Deploy track line); the "not served by default" scenario therefore runs against a gateway with the key unset, not against the compose gateway** (human)
- R2-9 [search] Toggles and tombstone TTL → **Not applicable to the gateway** (human, accepted)
- R2-10 [gateway] Rename and resilience → **`HTTP_PORT` becomes `GRPC_PORT` and `EDGE_REFLECTION_ENABLED` becomes `GRPC_REFLECTION_ENABLED` (already in this change); NO new read circuit breaker (SDK retry only); readiness waits until the revocation consumer has read up to the end offset captured at start (D3)** (human)
- R2-11 [analytics] GA4 keys, relay probe, `OTEL_SERVICE_NAME` → **Not applicable to the gateway** (human, accepted)
- R2-12 [accepted changes] Order dispute consumer stall, promotion kill-switch, payment DLQ shape, removal of `UPSTREAM_CALL_TIMEOUT*` in favour of the SDK 2 s bound → **Accepted; none touches the gateway, which keeps `CALL_TIMEOUT_SECONDS` and `AI_CALL_TIMEOUT_SECONDS` (it does not use `upstream.NewClient`, see D6)** (human)
- R2-13 [tests] Docker-backed tests → **Must run: a skip is a gate failure (`GO_SDK_REQUIRE_DOCKER=1` and a no-SKIP check in task 1.9.1). The engagement page-size cap does not touch the gateway** (human)
- R2-14 [repo] The agora repo is public → **No `GOPRIVATE` and no credentials are needed to fetch go-sdk `v1.0.2`** (human)

### Author scope notes (not human decisions)

- The "use the SDK for everything it covers" rule is applied where the SDK covers the gateway's job; packages it does not fit are listed with a reason in `proposal.md` ("Not adopted") and D6.
