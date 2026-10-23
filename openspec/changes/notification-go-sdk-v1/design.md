## D1 Target layout and package moves

| Today | After |
|---|---|
| `cmd/server/main.go` (wiring, signals, `pgxpool`) | `cmd/server/main.go` (signal context, `config.Load`, `app.Run`, non-zero exit) |
| `internal/config`, `internal/bootstrap/kafka.go` | `internal/config` (`Settings` over SDK groups + `Notification` fields) |
| `internal/grpcserver`, `internal/interceptor` | `grpcx.NewServer` + `auth` (deleted) |
| `internal/handler` (+ `errors.go`) | `internal/rpc` (handlers, `ErrorTable`, no business logic) |
| `internal/service` (alerts, prefs) | `internal/alerts`, `internal/prefs` (domain, no grpc import) |
| `internal/repository` | `internal/store` (pgx queries, `migrations/` embedded); in-memory repos become `_test.go` fakes in the packages that use them |
| `internal/consumer` | `internal/events` (`listing.go`, `chat.go`, `order.go`, shared `handler.go`) |
| `internal/upstream` | `internal/names` (resolver; client from `upstream.NewClient`) |
| `migrations/` | `internal/store/migrations/` (files unchanged) |

`cmd/server` keeps its name: `go-service.mk` defaults `BINARIES` to `server`, the Dockerfile builds `./cmd/server` and `ENTRYPOINT ["/server"]`. ADR-0016 says `cmd/<bin>`; `server` is a valid `<bin>`. Rejected: renaming to `cmd/notification` (churns Dockerfile and compose for no gain).

Domain packages never import `google.golang.org/grpc` or `codes`: they return sentinel errors (`alerts.ErrEmptyListing`, `alerts.ErrInvalidType`, `alerts.ErrEmptySubID`, `alerts.ErrEmptyUser`, `inbox.ErrNotFound`, `prefs.ErrNilPrefs`) that `internal/rpc` maps through the table. Domain package names drop the `service`/`repository` split because the SDK layout has one package per concept.

## D2 Settings

`config.Settings` embeds `config.Runtime` and `config.GRPC`, and names `Postgres`, `OTel`, `Kafka`, `LogKeyFP`, plus a `Notification` group:

```
LOW_STOCK_THRESHOLD (default 5, int32, >= 0)
LISTING_EVENTS_TOPIC / LISTING_EVENTS_DLQ_TOPIC / NOTIFICATION_LISTING_CONSUMER_GROUP
CHAT_EVENTS_TOPIC / CHAT_EVENTS_DLQ_TOPIC / NOTIFICATION_CHAT_CONSUMER_GROUP
ORDER_EVENTS_TOPIC / ORDER_EVENTS_DLQ_TOPIC / NOTIFICATION_ORDER_CONSUMER_GROUP
UPSTREAM_DOMAIN_ADDR / UPSTREAM_IDENTITY_ADDR
```

All keep today's names and defaults (except the two `UPSTREAM_*_ADDR`, now without default), so compose and gitops keep working apart from the deleted keys. `KAFKA_BROKERS` loses the `localhost:9092` code default (the SDK fills a local default for a local ENV only; staging and prod require it). `DATABASE_URL` is required with no localhost default: the old default silently pointed at `localhost:5440`. `GRPC_PORT` has no default (SDK `required`); the Dockerfile `ENV GRPC_PORT=50058` stays and compose and gitops already set it. `ENV` becomes required; compose sets `ENV=local` already, gitops baseline gets `ENV` (Deploy track). `SHUTDOWN_GRACE` and `SHUTDOWN_DRAIN_DELAY` replace the `SHUTDOWN_GRACE_SECONDS` line in the old `.env.example`, which no code read. `DB_MAX_CONNS` is now honoured. `.env.example` lists every key once; `TestEnvExampleInSync` (the target `make check-env` runs) reads `../../.env.example` with `config.CheckEnvExample`.

`LowStockThreshold` validation stays in `Validate` with the same message shape (`invalid LOW_STOCK_THRESHOLD`).

`UPSTREAM_*_ADDR` have no default and are required in every ENV (compose and gitops already set both). Removed: field `KafkaBroker`/`KAFKA_BROKER`, the `localhost` defaults of `DATABASE_URL` and `KAFKA_BROKERS`. Removed keys, absent from `.env.example` and README (compose and gitops via the Deploy track): `DATABASE_ENABLED`, `KAFKA_ENABLED`, `SHUTDOWN_GRACE_SECONDS`. Kept: `LOG_JSON` (SDK `Runtime`) and `GRPC_REFLECTION_ENABLED` (`false` in `.env.example`).

## D3 Composition root (`internal/app`)

Order, as in ADR-0016: logger (`logx.New`, `slog.SetDefault`) -> `otelx.Init` -> `healthx.New` -> `postgres.Open` (failure returns, no local exception) -> `health.Register("postgres", postgres.Checker(pool))` -> `kafkax.NewConsumerClient` x3 and one `kafkax.NewProducer` for the DLQ (no kafka checker) -> `names` resolver from `upstream.NewClient` x2 -> `grpcx.NewServer` + `rpc.Register` -> `lifecycle.Run` with components `grpcx.Serve`, `health.Run` and one `consume.Consumer.Run` per topic. Each closer is pushed on a `lifecycle.Stack` right after opening. `OnShutdown: health.Drain`. A consumer returning an error fails the whole `lifecycle.Run` (the old "consumer stopped while the service is running" log, with a healthy pod, is gone).

The three consumers share one DLQ producer; each has its own consumer client and group, as today (`team-notification.listing|chat|order`).

## D4 Handlers and errors

`rpc.Handler` holds the domain services and an `ErrorTable`:

| Error | Code |
|---|---|
| `inbox.ErrNotFound` | NotFound ("notification not found") |
| `alerts.ErrEmptyListing`, `ErrInvalidType`, `ErrEmptySubID`, `ErrEmptyUser`, `prefs.ErrNilPrefs` | InvalidArgument |

Unknown errors go through `grpcx.Status` and become `Internal` / `internal error`; the SDK table adds 23505 -> `AlreadyExists` and `Canceled` / `DeadlineExceeded`. `callerUserID` becomes `auth.RequireUser(ctx)`: no principal or anonymous -> `Unauthenticated`, service principal -> `PermissionDenied`, user -> id. The old message strings of `callerUserID` ("authentication required", "per-user notification RPCs require a user principal") are replaced by the SDK ones; codes are unchanged and no spec names the text. `RequireScopes` is still not used: no RPC checks a scope today and none is added.

`ListNotifications` stays offset-paged by `page_number` and `page_size`. `pagination.Clamp(size, 20, 100)` replaces the open-coded default. The proto has no token field, so the SDK offset token is not used and the wire format is unchanged.

## D5 Consumers

Each topic gets a `consume.Consumer` whose `consume.Handler` unwraps the `EventEnvelope`, then runs the domain logic inside one `pgx.Tx`:

1. `tx.Begin`; `ledger.Seen(ctx, tx, "<consumer>", eventID)`; if seen, commit and return nil (no second notification).
2. Preference check (chat and order), diff against `listing_last_seen` (listing), notification inserts, state writes, all on `tx`.
3. `tx.Commit`. Any error rolls back, including the ledger row, so the retry notifies (this is what `TestChat_CreateFailureIsRetriedNotDeduped` asserts today with two calls; now it is one transaction).

Rejected: keeping `IsProcessed` then `MarkProcessed` after the writes. It has a crash window between notification insert and ledger insert, and the SDK ledger is designed for the transaction form. The `processed_events (consumer, event_id)` table already matches `consume.NewDedupe("")`, so no migration.

Permanent errors (malformed envelope, empty `event_id`, order shipped without buyer) are wrapped in `consume.Permanent` and land in the DLQ topic with the SDK's dead-letter headers; the offset advances. Events with a type the consumer does not handle, an empty `seller_id` on `OrderPaidEvent`, or a missing chat recipient are skipped with nil (offset advances, no DLQ), as today. Retries are the SDK default (5 attempts, doubling backoff). The old `runLoop`, `RecordReader`, `DeadLetterProducer`, `RunConfig` and `bootstrap/kafka.go` are deleted. Rebalance timing: handler time is a few SQL statements; the SDK default `MaxPollRecords` 500 x worst case stays inside the default rebalance timeout, and `Options.MaxPollRecords` is left default.

Logging: the consumers build `consume.Options.KeyFP` from `kafkax.NewFingerprinter(settings.LogKeyFP.Secret)`; the loop logs `key_fp`, never the key or payload. Handler errors that wrap user ids are not logged verbatim: the handler logs `event_id` and error class only.

The listing consumer keeps its state tables (`listing_last_seen` price, stock, and the low-stock state of migration 005) and its rules (first sight never fires, deleted listing drops state, low-stock re-arm above threshold). The `InMemory*` stores and `InMemoryDeduper` move to `_test.go` fakes so unit tests keep their speed; each rule also keeps a Postgres-backed test on `testkit.Postgres`.

## D6 Sender names

`internal/names` keeps `Resolver.ResolveSenderName(ctx, senderID, sellerID) string` (never errors, `""` on any failure, caller falls back to `Người dùng`). The two gRPC connections come from `upstream.NewClient(addr, upstream.AsService("service-team-notification", "listing.read", "identity.read"))`; the hand-written `withServicePrincipal` goes. Dialing is lazy, so there is no "lookup disabled" branch: the nil-client skips remain only for tests that pass one fake. The 2 s bound is `upstream.Bounded(ctx, 2*time.Second)` around the storefront-then-profile pair.

## D7 Health and probes

`healthx` readiness (`""`) is the `postgres` checker only; `liveness` is independent. Kafka is not a readiness check (human, Round 2): a broker outage must not remove the pod from traffic, because the gRPC RPCs do not need Kafka; consumers retry on their own. `probes.livenessService: liveness` is still set by the Deploy track; the deploy change must go with this one.

## D8 Mock data

The only existing "mock" is the nil-pool mode. It is not mock data (the real handler code did not run), so it is deleted, not ported. No `MOCK_DATA` group is added: with zero adapters the group would be dead config. If a later change adds a fixture adapter (for example sender names), it adds `config.MockData` and `mockdata.New(cfg, "<adapter>")` then.

## D9 Behaviour preserved (test inventory)

Existing tests map one to one and keep their names where the code moves: handler tests (`TestUsersDoNotSeeEachOthersData`, `TestMarkAsReadOtherUsersNotificationIsNotFound`, `TestPerUserRPCsRequireAUserPrincipal`, `TestMapErrsHideInternalCause`) to `internal/rpc`; consumer tests (`TestChat_*`, `TestOrderShipped_*`, `TestOrder*`, `TestPayment*`, `TestCancel*`, `TestReturn*`, `TestLowStock*`, `TestPriceDrop*`, `TestBackInStock*`, `TestRestock*`, `TestRedelivered*`, `TestStateReadFailureIsTransient`) to `internal/events`; `TestResolve_*` to `internal/names`; `*_pg_test.go` and `TestPostgres*` to `internal/store` / `internal/events` on `testkit.Postgres`. `TestRun_MalformedRecord_RoutedToDLQAndCommitted` and `TestRun_TransientFetchErrorsDoNotStopTheLoop` test the deleted loop; the SDK `consume` suite covers the loop, and `TestMalformedRecordGoesToDLQAndAdvances` here covers the wiring. `TestTask5ReadmeDocumentsOrderWorkEventsAndLowStockThreshold` keeps asserting the README and moves to `internal/config`.

## D9a Test infrastructure for Kafka

`testkit.Redpanda(t)` (SDK `pkg/testkit`) starts a real Redpanda container and returns the brokers, like `testkit.Postgres`; both need Docker (`GO_SDK_REQUIRE_DOCKER=1`; a skipped Docker-backed test is a gate failure). `TestMigrateThenRunServesAllRPCs`, `TestChatEventBecomesNotificationWithin30s` and `TestReadinessIgnoresKafka` use it; consumer unit tests use `testkit.Recorder` for the DLQ producer and `testkit.Produce` / `ReadAll` where a real topic is needed. Listing alerts do not consult preferences (README: "Listing alerts do not check prefs"); this change keeps that and pins it with `TestListingAlertsIgnorePrefs`.

## D10 Deploy interplay

The service never migrates at boot. The binary has a `migrate up` subcommand (`cmd/server/main.go` dispatches on `os.Args[1]`, then `internal/app.Migrate` runs `migrate.Up(ctx, url, fs.Sub(store.Migrations, "migrations"))` over the embedded files and exits non-zero on failure). Compose keeps the `team-notification-migrate` service, now running the service image with `migrate up`; the service `depends_on` it with `service_completed_successfully`. Gitops gets an Argo PreSync migrate Job per service, in `deploy-go-sdk-v1`. Order: this change and `deploy-go-sdk-v1` merge together; `deploy-go-sdk-v1` is the umbrella change for the deploy edits of all services and owns every shared deploy-runtime requirement (including "Services refuse unsafe configuration in staging and production"). The Dockerfile stays unchanged; the agora repository is public, so `go mod download` fetches `go-sdk@v1.0.2` with no `GOPRIVATE` or credentials. The exact lines are in `tasks.md` "## Deploy track".

## Clarifications / ### Round 1 — 2026-10-23

- Target `github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2` (tag exists): require the tag, no `replace`, no `third_party`; reference `../platform-core/make/go-service.mk` and the SDK `.golangci.yml` (no copies; revive doc-comment rules are off, so add no doc comments; pin `golangci-lint` as a go tool like the SDK `go.mod`). ADR-0016 layout. (human, recipe)
- Delete every dependency enable flag (`DATABASE_ENABLED`, `KAFKA_ENABLED`, ...) and every runtime in-memory or noop fallback (`NewInMemory*` in main/bootstrap, noop publishers, `RequireDurableStorage`); importing a dependency means using it. In-memory implementations stay only as test fakes. Kept flags: `OTEL_ENABLED`, `GRPC_REFLECTION_ENABLED` (off by default, refused in strict ENV), `SEED_ADMIN_ENABLED` (identity only). (human, recipe)
- Mock = mock data served by the real service, never a mock service; existing mock switches move onto `pkg/mockdata` (`MOCK_DATA=<adapters>`, refused in strict ENV before validation); the real code path always runs. (human, recipe)
- Use the SDK for everything it covers: config, logx, otelx, healthx (readiness = overall, liveness service independent of deps, `probes.livenessService: liveness`), lifecycle, grpcx (error table incl. 23505 -> AlreadyExists, Canceled/DeadlineExceeded codes), auth, postgres.Open (no OpenOrLocal), migrate, kafkax/consume (key_fp HMAC logging with `LOG_KEY_FP_SECRET`, DLQ), upstream (service principal), pagination (keep existing cursor formats), resilience and jwtx where relevant. These behaviour changes are accepted. (human, recipe)
- Outbox, idempotency (HTTP key rule), auditx, flags, resilience and jwtx do not apply: this service publishes no events, has no write RPC that needs an `Idempotency-Key`, no privileged action to audit, no feature flag and verifies no JWT. (human, recipe)
- Keep every current requirement and every Q1-Q11/C4/privacy-hardening behaviour green; main's behaviour wins over the old migration (reference only: `git show origin/feat/go-sdk-v1-e2e:<path>`). MODIFY only requirements that describe removed flags, fallbacks or env vars (none exist for this service); ADD capability requirements where no spec describes the service. (human, recipe)
- No DPO sign-off needed: basis legitimate interest, platform log retention. (human, recipe)
- e2e is not run per service now; it runs once at the end. Scenarios may still name e2e features in VERIFIED BY. `docker-compose.services.yaml` and `platform-gitops` edits are not code tasks here; they are listed in `tasks.md` "## Deploy track (done by change deploy-go-sdk-v1)". (human, recipe)

### Round 2 — 2026-10-23

- Migrations: every service has a `migrate up` subcommand in its binary (SDK `pkg/migrate`, embedded migrations); the service never migrates at boot; compose keeps a `<svc>-migrate` service running the service image with `migrate up`; gitops gets an Argo PreSync migrate Job per service (in `deploy-go-sdk-v1`). (human)
- Kafka is not a readiness check in any service: readiness is Postgres (and required upstream checks, none here). (human)
- The shared deploy-runtime requirement "Services refuse unsafe configuration in staging and production" is MODIFIED only by `deploy-go-sdk-v1`; this change carries no MODIFIED copy, and lists the needed bullet under the Deploy track. (human)
- Upstream addresses the service uses (`UPSTREAM_DOMAIN_ADDR`, `UPSTREAM_IDENTITY_ADDR`) are required in every ENV, no fallback mode. No scenario of an existing spec or unarchived change for this service describes a fallback. (human)
- Local compose sets `GRPC_REFLECTION_ENABLED=true` for every service (Deploy track line). (human)
- Docker-backed tests must run: a skip is a gate failure. (human)
- The agora repository is public: no `GOPRIVATE` or credentials. (human)
- Outbox, promotion, order, payment, gateway, engagement, search and analytics decisions of the same round do not apply to this service (it publishes no events). (human)
