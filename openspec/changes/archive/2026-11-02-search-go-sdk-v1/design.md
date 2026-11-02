## Context

team-search has two binaries from one module: `cmd/server` (gRPC `SearchService`: `SearchListings`, `Suggest`, and four saved-search RPCs; reads OpenSearch, stores saved searches in Postgres) and `cmd/indexer` (consumes `listing.events`, writes OpenSearch, embeds through the model server, classifies tags through team-ai, purges tombstones). The image ships both (`/server`, `/indexer`); gitops runs the indexer as a separate Deployment with no Service and no probes.

## Clarifications

### Round 1 — 2026-10-23

- Target `github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2`: require the tag, no `replace`, no `third_party`; reference `../platform-core/make/go-service.mk` and the SDK `.golangci.yml` without copies; add no doc comments (revive doc-comment rules are off); pin golangci-lint as a `tool` like the SDK `go.mod`; ADR-0016 layout. (human, recipe)
- Delete every dependency enable flag (`DATABASE_ENABLED`, `KAFKA_ENABLED`, ...) and every runtime in-memory or noop fallback (`NewInMemory*` outside tests, `RequireDurableStorage`): importing a dependency means using it. In-memory implementations stay only as test fakes. Kept flags: `OTEL_ENABLED`, `GRPC_REFLECTION_ENABLED` (off by default, refused in a strict ENV). (human, recipe)
- Mock is mock data served by the real service, never a mock service; existing mock switches move onto `pkg/mockdata` (`MOCK_DATA=<adapters>`, refused in a strict ENV before validation). team-search has no mock switch today, so it declares no `MOCK_DATA` group (D8). (human, recipe)
- Use the SDK for everything it covers: config, logx, otelx, healthx (readiness overall, `liveness` independent of dependencies, `probes.livenessService: liveness`), lifecycle, grpcx (error table including 23505 to AlreadyExists, Canceled and DeadlineExceeded codes), auth, postgres.Open (no OpenOrLocal), migrate, kafkax and consume (`key_fp` HMAC with `LOG_KEY_FP_SECRET`, DLQ), outbox, idempotency, upstream (service principal), auditx, flags, pagination (keep existing cursor formats), resilience, jwtx where relevant. These behaviour changes are accepted. (human, recipe)
- Keep every current behavioural requirement and every Q1-Q11, C4 and privacy-hardening behaviour green; main's behaviour wins over the old migration on `origin/feat/go-sdk-v1-e2e` (reference only). MODIFY only requirements that describe removed flags, fallbacks or env vars; ADD capability requirements where no spec describes the service. (human, recipe)
- No DPO sign-off needed: basis legitimate interest, platform log retention. (human, recipe)
- e2e is not run per service now; it runs once at the end. Scenarios may still name e2e features in VERIFIED BY. docker-compose and platform-gitops edits are not code tasks of this change; they are listed in `tasks.md` "Deploy track". (human, recipe)

### Round 2 — 2026-10-23

- Migrations: the binary has a `migrate up` subcommand (SDK `pkg/migrate`, embedded migrations); the service never migrates at boot; compose keeps a `team-search-migrate` service that runs the service image with `migrate up`; gitops gets an Argo PreSync migrate Job (deploy-go-sdk-v1). (human)
- Kafka is not a readiness check in any service: readiness is Postgres and the required upstream checks only. team-search's query server has no Kafka; the indexer has no health listener. (human)
- The shared `deploy-runtime` requirement "Services refuse unsafe configuration in staging and production" is MODIFIED only by deploy-go-sdk-v1; this change carries no `deploy-runtime` delta and lists the needed team-search bullet under the Deploy track. (human)
- Outbox upgrade rules (requeue of rows parked as `failed`, scale old pods to 0 once, 7-day `Store.Purge`) do not apply: team-search publishes no events and has no outbox table. (human)
- Raw gitops manifests (`platform/search-indexer/indexer.yaml`) get strict ENV overlays in deploy-go-sdk-v1; `LOG_KEY_FP_SECRET` is provisioned in Vault by the user (human task). (human)
- Upstream addresses and clients the service uses are required in every ENV, with no fallback mode: `UPSTREAM_AI_ADDR` for the indexer, and OpenSearch and Postgres for the server. This change deletes no archived scenario: the nil-classifier path never had a scenario in `openspec/specs`. (human)
- Local compose sets `GRPC_REFLECTION_ENABLED=true` for the query server (Deploy track). (human)
- Keep `ENABLE_HYBRID_SEARCH` and `ENABLE_RERANKER` as retrieval-strategy toggles; an invalid `TOMBSTONE_TTL` or `TOMBSTONE_PURGE_INTERVAL` refuses boot. (human)
- Docker-backed tests must run: a skipped Docker test is a gate failure, including the OpenSearch integration tests. (human)
- Accepted: the SDK 2 s upstream timeout bound (the classifier keeps `WithTimeout(2*time.Second)`). (human)

## Decisions

### D1 Layout

```
cmd/server/main.go     signals, config.Load, app.Run(ctx, settings), exit code
cmd/indexer/main.go    signals, config.LoadIndexer, app.RunIndexer(ctx, settings), exit code
internal/app/          server.go (Run), indexer.go (RunIndexer); consts service = "team-search" and "team-search-indexer"
internal/config/       Settings, IndexerSettings, Load, LoadIndexer
internal/rpc/          SearchService handlers (from internal/handler), error table, visibility rules
internal/store/        saved-search store on pgxpool (from internal/repository)
internal/events/       listing-event handler (from internal/consumer/listing.go)
internal/index, internal/retrieval, internal/taxonomy, internal/tombstone   domain packages, kept
migrations/embed.go    //go:embed *.sql, used by store tests through migrate.Up
```

Deleted: `internal/bootstrap`, `internal/observability`, `internal/interceptor`, `internal/grpcserver`, `internal/repository`, `internal/consumer/consumer.go`, `internal/config/envcheck.go`. The OpenSearch adapter in `internal/index` stays the hand-written client over `opensearch-go`; the SDK has no OpenSearch package.

### D2 Two settings types, one per binary

The server needs `GRPC`, `Postgres`, `OTel`, OpenSearch and retrieval. The indexer needs `Kafka`, `LogKeyFP`, `OTel`, OpenSearch, retrieval, taxonomy, tombstone, consumer group and topic, and must not require `DATABASE_URL` or `GRPC_PORT` (it has no database and no listener). A single struct would make one binary refuse boot for the other's keys. So `Settings` (server) and `IndexerSettings` both embed `sdkconfig.Runtime` and are bound by `sdkconfig.Load[T]`; `.env.example` lists the union, and `TestEnvExampleInSync` checks that every key of each struct is documented and that no documented key belongs to neither struct. `UPSTREAM_AI_ADDR` is required in `IndexerSettings` only (D6). The indexer has no `GRPC` group (so no `GRPC_PORT`), and `ShutdownGrace` lives in that group in the SDK, so `IndexerSettings` declares its own `ShutdownGrace time.Duration` with `env:"SHUTDOWN_GRACE" default:"10s"`, checked with `sdkconfig.PositiveDuration`. `OPENSEARCH_URL` keeps its default `http://localhost:9200` (local development) and an empty value still refuses load (`TestValidateRequiresOpenSearchURL` is kept); deployments set it explicitly. Other keys: `GRPC_HOST` (`GRPC` group, kept), `OPENSEARCH_INDEX`, `EMBEDDING_DIM`, `LOG_LEVEL`, `LOG_JSON` (kept, `LOG_*` mapped to the SDK `Runtime`/log group, `OPENSEARCH_INDEX` and `EMBEDDING_DIM` stay service fields with their defaults), `OTEL_ENABLED` and `OTEL_EXPORTER_OTLP_ENDPOINT` (kept, mapped to the SDK `OTel` group). All of them are listed in `.env.example` and covered by `TestEnvExampleInSync`. The SDK OTel group has no service-name default, so `Load` sets `OTel.ServiceName` to `team-search` and `LoadIndexer` to `team-search-indexer` in code after binding when `OTEL_SERVICE_NAME` is unset; `OTEL_SERVICE_NAME` stays an overridable key in `.env.example`. `DB_MAX_CONNS` (server) is new, from `sdkconfig.Postgres`.

`ENV` has no default (SDK `Runtime`). `SHUTDOWN_GRACE_SECONDS` becomes `SHUTDOWN_GRACE` (`10s`). `KAFKA_BROKERS` comes from `sdkconfig.Kafka` (required in a strict ENV, `localhost:9092` locally). `KAFKA_CONSUMER_GROUP` (default `team-search-indexer`) and `KAFKA_LISTING_TOPIC` (default `listing.events`) stay service fields. The DLQ topic is `<topic>.dlq`, as today.

### D3 Postgres on pgxpool, no database/sql

`postgres.Open(ctx, s.Postgres)` replaces `bootstrap.openPostgres`; `postgres.Checker(pool)` is registered with health. The saved-search store moves from `database/sql` to `pgxpool.Pool` queries with the same SQL, the same ordering (`created_at DESC, id DESC`) and the same scoping by `user_id`. `pgx.ErrNoRows` maps to `store.ErrSavedSearchNotFound`. The in-memory repository moves to a `_test.go` fake in `internal/rpc` behind a small `SavedSearches` interface; the handler has no `saved == nil` branch. Migrations are applied by `/server migrate up` (D12), never at boot; tests apply `migrations.Files` through `migrate.Up` on `testkit.SharedPostgres`.

### D4 Auth through `pkg/auth`

`auth.UnaryServerInterceptor` (inside `grpcx.NewServer`) replaces the hand-written principal interceptor. `visibility.go` moves from `*commonv1.Principal` to `auth.Principal`. The saved-search gate becomes `auth.RequireUser` (anonymous, empty id or id `anonymous` gives `Unauthenticated`; a service gives `PermissionDenied`), then `auth.RequireScopes(ctx, scope)` (exact match). That keeps the current order (identity before scopes) and codes. Rejected: keeping a local `callerID`, because the SDK gate is the same rule and a local copy is the drift the migration removes.

### D4a Anonymous search stays public

`SearchListings` and `Suggest` use `auth.RequireScopesAllowAnonymous(ctx, "search:read")`, not `auth.RequireScopes` (which rejects anonymous principals with `Unauthenticated`). The gateway grants anonymous callers `listing.read` and `search:read`, so public browsing depends on this choice (existing tests `in_stock_test.go`, `rating_test.go`, `visibility_handler_test.go`; FEATURES `search.visibility-hides-drafts`). Only the four saved-search RPCs use `RequireUser` then `RequireScopes`. A call with no principal metadata still returns `Unauthenticated` (the SDK interceptor rejects it).

### D5 Errors through `grpcx.Status`

An `ErrorTable` maps `store.ErrSavedSearchNotFound` to `NotFound` ("saved search not found"). Everything else a handler returns as a plain error goes to `grpcx.Status`: statuses that already carry a code (visibility `InvalidArgument`, `Unauthenticated`, `PermissionDenied`) pass unchanged, `context.Canceled` and `DeadlineExceeded` map to their codes, a Postgres 23505 maps to `AlreadyExists` (no unique constraint besides the primary key today, so it is unreachable but free), and the rest becomes `Internal` "internal error". Accepted change: the per-RPC `Internal` texts (`search failed`, `suggest failed`, `save search failed`, ...) become `internal error`; the gateway forwards the code and no client branches on the text.

### D6 Indexer on `consume` and `kafkax`

`kafkax.NewConsumerClient(s.Kafka, group, []string{topic})` builds the group client; `kafkax.NewProducer(s.Kafka)` is the DLQ producer; `consume.NewConsumer(client, handler, producer, consume.Options{DLQTopic: topic + ".dlq", KeyFP: fp})` runs it, with `fp` from `kafkax.NewFingerprinter(s.LogKeyFP.Secret)`. Retry count 5 and backoff 100 ms to 30 s are the SDK defaults and equal today's `RetryConfig`. The handler signature becomes `func(ctx, consume.Record) error`; the listing handler reads `r.Value`. Offsets commit once per fetch; a DLQ publish failure stops the loop without committing, as today. DLQ records now also carry source partition, offset and group headers (accepted). The hand-written `Consumer`, `kgoDLQ` and `RetryConfig` and their tests are deleted; the behaviours they tested are re-asserted through the service handler and `consume`.

Request id and trace propagation (`x-request-id`, `traceparent`) and the per-call `grpc.request` log come from the `grpcx` interceptors that replace `internal/interceptor`; `TestRequestIDAndTraceparentPropagate` covers them.

The tag classifier is built with `upstream.NewClient(s.Taxonomy.AIAddr, upstream.AsService("service-team-search", "ai.classify"), upstream.WithTimeout(2*time.Second))`, replacing the hand-set `x-principal-*` metadata and `grpc.NewClient`. The SDK client dials lazily and its TLS and credentials posture is the SDK default; on the cluster network this is the same plaintext-in-mesh posture as the old `insecure` credentials, so it is unchanged in practice. `UPSTREAM_AI_ADDR` is required in `IndexerSettings`: importing a dependency means using it, so the nil-classifier path is removed. A classifier call that fails still sets `tags_pending` and never fails ingestion (spec `tag-taxonomy-enrichment`, unchanged). The `log.Printf` calls in `internal/events` become `slog` with context.

The tombstone loop becomes `lifecycle.Every(interval, purge)` (no `RunAtStart`: today the first purge is one interval after boot). `TOMBSTONE_TTL` and `TOMBSTONE_PURGE_INTERVAL` are `time.Duration` fields checked with `sdkconfig.PositiveDuration`, so an unparsable or non-positive value refuses boot (D9).

The indexer has no gRPC listener today and gets none: a crashed consumer exits non-zero and Kubernetes restarts it, which is the liveness it needs. A health port would add a surface and a Service nobody asked for.

### D7 Health and shutdown

Server: `healthx.New(healthx.DefaultInterval, logger)`; checkers `postgres` (`postgres.Checker(pool)`) and `opensearch` (a new `Ping(ctx)` on the `index.Index` adapter, bounded by the checker context). `healthx` serves `liveness` itself. `lifecycle.Run` with `OnShutdown: health.Drain`, components `grpcx.Serve(srv, s.GRPC)` and `lifecycle.Func(health.Run)`, closers on a `lifecycle.Stack` (otel shutdown, pool close). Gitops sets `probes.livenessService: liveness` for team-search (deploy track). Reflection registers only when `GRPC_REFLECTION_ENABLED=true`.

Kafka is not a readiness check: the query server has no Kafka dependency and the indexer serves no health.

Indexer: `lifecycle.Run` with the consumer and the tombstone ticker as components; the consumer's `Run` returns nil on context cancel.

### D8 SDK packages not adopted, and why

- `mockdata`: team-search has no mock switch; the model server and team-ai are real services. No `MOCK_DATA` group is declared (a group with no adapter is an unused knob). A mock adapter, if wanted later, is a new change.
- `outbox` (including the upgrade rules for parked rows and the purge), `idempotency`, `auditx`, `flags`, `jwtx`, `resilience`: the service publishes no events, performs no privileged action, evaluates no Flipt flag and verifies no JWT (the gateway does). Embed and rerank HTTP calls already fail open with a 2 s timeout; wrapping them in a retry policy would change the fail-open latency contract in `search-retrieval`, so it is not done.
- `migrate` at boot: never; migrations run only through the `migrate up` subcommand (D12).
- Dedupe: the read-model version guard (`occurred_at` in nanoseconds) already makes redelivery idempotent (specs `search-read-model-deletes`, `search-stock-read-model`); a `processed_events` table would add a write per event for no new guarantee.

### D9 Behaviour kept and behaviour changed on purpose

Kept: visibility rules and draft ownership, the `in_stock` filter, hybrid retrieval, the version guard, tombstones, `tags_pending` and `vector_pending`, the plain-integer page cursor (`strconv.Itoa(offset)`; `pagination.Clamp` bounds the size; `pagination.EncodeOffset` is not used because it changes the cursor format, and the gateway and frontend already hold integer cursors), defaults 10 and 50.

Changed (accepted, listed in the proposal): required `ENV` and `GRPC_PORT`, `SHUTDOWN_GRACE`, reflection default off, `internal error` messages, `key_fp` logging, DLQ headers, boot refusal for invalid tombstone durations. The last one replaces "warn and use the default": a silently ignored operator setting is a runtime fallback of the kind the user ruled out, and the fix is a one-line deploy change.

### D10 Retained retrieval toggles

`ENABLE_HYBRID_SEARCH` and `ENABLE_RERANKER` select a retrieval strategy (spec `search-retrieval`: the reranker is an optional stage that fails open to RRF order); they do not decide whether an imported dependency is used. They are kept with their current defaults. The embed client and the model-server URL are always built. Recorded as a question under `## Open issues

- QUESTION: the repository states no platform log retention figure. The proposal records it as `TBD, platform default`; give the real number.
