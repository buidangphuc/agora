## D1 Layout and what each SDK package replaces

| Today | After | SDK package |
|---|---|---|
| `internal/config` hand-parsed env | `Settings{sdkconfig.Runtime; sdkconfig.GRPC; Postgres; OTel; Kafka; Outbox; Events}` loaded with `sdkconfig.Load` | `config` |
| `internal/bootstrap`, pgxpool setup | `postgres.Open` + `postgres.Checker` | `postgres`, `healthx` |
| `internal/observability`, `interceptor/tracing.go` | `otelx.Init`, `grpcx` stats handler and request-id | `otelx`, `grpcx`, `reqctx` |
| `interceptor/auth.go` | `auth.RequirePrincipal`; gateway metadata read by the `grpcx` chain | `auth` |
| `internal/grpcserver`, `cmd/server/main.go` signal handling | `grpcx.NewServer`, `grpcx.Serve`, `lifecycle.Run` | `grpcx`, `lifecycle` |
| `internal/events` KafkaProducer + Relayer, `repository/outbox*.go` | `kafkax.NewProducer`, `outbox.NewStore`, `outbox.NewRelayer`; `events` keeps only `BuildMessageOutboxRow` | `kafkax`, `outbox` |
| `handler/errors.go` | `grpcx.Status` with an `ErrorTable` | `grpcx` |
| slog setup in main | `logx.New` | `logx` |

Not used, on purpose: `pagination` (the cursor is the decimal page number and a page size below 1 or above 100 falls back to the default; `pagination.EncodeOffset` and `Clamp` would change both, and the user's rule keeps existing cursor formats), `consume` and `kafkax.Fingerprinter` (chat consumes no topic, so `LOG_KEY_FP_SECRET` is not in `Settings`), `idempotency` (no write is replayed by a client key; `GetOrCreateThread` is already idempotent in SQL), `upstream` and `UPSTREAM_LISTING_ADDR` (no code calls team-domain), `auditx` (chat has no privileged or money action), `flags`, `jwtx`, `resilience`, `mockdata` (chat has no mock switch; there is no adapter to swap). `migrate.Up` is used by the `migrate up` subcommand (D10) and by tests; the service never migrates at boot.

The binary stays `server` under `cmd/server` so the Dockerfile target, compose and the gitops image name do not change.

## D2 Always-on dependencies, no fallbacks

`Database.Enabled`, `Events.KafkaEnabled`, `Outbox.Enabled`, the `res.Pool != nil && res.Producer != nil` guard, the nil-pool path, `NewInMemoryChatRepository`/`NewInMemoryOutboxRepository` in non-test files and the `WithInMemoryOutbox` option are deleted. The in-memory chat repository and in-memory outbox move to test-only files: `internal/chat/fake_test.go` and `internal/rpc/fake_test.go` each hold their own copy of the in-memory chat repository and outbox (a `_test.go` file of one package cannot be imported by another, and an exported testkit package would be a non-test file that defines an in-memory repository, which the no-fallback scan forbids). The duplication is accepted; `store` is tested against Postgres only. `app.Run` builds in this order: logger, telemetry, health, Postgres, Kafka producer, repository, server, relayer. An error at any step closes what is already open (`lifecycle.Stack`) and returns it; boot never continues without a dependency.

## D3 Readiness is Postgres only

`healthx` readiness is the overall status. Registered checks: `postgres` only. Registering Kafka would flip readiness to NOT_SERVING on a broker outage, take chat out of the gateway's endpoints, and break the existing guarantee that a message sent while Kafka is down is stored and delivered later. The producer is still constructed at boot (an invalid config fails boot) and dials lazily, so a broker that is down at boot does not block start. With `ENV=local` and `KAFKA_BROKERS` unset, `Load` resolves the SDK default `localhost:9092` and boot builds the producer anyway; the old `KAFKA_ENABLED=false` path that booted with no Kafka at all is gone, so a developer without a broker still boots (dial is lazy) and sends are stored and relayed once a broker appears. Rejected: a Kafka readiness check; a Kafka liveness check (restarts do not help a broker).

## D4 Outbox table moves to the SDK v1 schema in the existing migration chain

The table `chat_outbox_events` has the v0 shape (migration `0003`). `outbox.NewStore` needs `topic`, `traceparent` and `seq`. Migrations `0004`..`0011` are the statements of `outbox.UpgradeSQL("chat_outbox_events")` split one transaction-safe unit per file: `0004` Prepare (columns and sequence), `0005` Backfill (batch loop is not expressible in one file; the file numbers rows in a `DO` block loop until none is left), `0006` Switch (one transaction), `0007` seq NOT NULL (constraint, validate, set), `0008..0010` the three `CREATE INDEX CONCURRENTLY`, `0011` drop `chat_outbox_events_claim_idx` (named `idx_chat_outbox_claim` in 0003; the migration drops both names if present), `0012` requeue rows parked as `failed` (`UPDATE ... SET status='pending'` with attempts and last error cleared; down file is a no-op). `CONCURRENTLY` cannot run inside a transaction block, so each of those is its own file, and the `migrate up` must run `0004`..`0011` without wrapping them in one transaction (SDK `migrate.Up` runs each file separately; a wrapped `CREATE INDEX CONCURRENTLY` fails). A Postgres test applies `0001..0012` to a database holding v0 rows and asserts the final schema equals `outbox.SchemaSQL` (columns, NOT NULL, indexes) and that existing rows keep creation order in `seq`.

Deploy order (README "Deploying the go-sdk v1 outbox"): the first rollout scales the old pods to 0 before any new pod starts (Recreate strategy once, then back to rolling; never two relayers), the migrate job (`migrate up`) runs, then v1 starts. No manual requeue or purge step remains: `0012` requeues rows parked as `failed` (parked v0 rows would block their thread under v1), and published rows are purged after 7 days by the service itself (D11). A v0 pod running during the switch would write rows without `seq`; scaling to 0 first removes that case. Size bound: `0005` runs in one transaction and takes a lock on `chat_outbox_events`, which nobody writes during the migration; it must finish in under 5 s on 10,000 rows (`TestOutboxBackfillIsBounded`). The table may be larger at first deploy because published rows are no longer purged by hand; the run time grows roughly linearly and the lock affects no live writer, so this is accepted.

## D5 Config mapping and defaults

| Old | New | Default change |
|---|---|---|
| `SERVER_HOST/PORT` | `GRPC_HOST` (0.0.0.0) / `GRPC_PORT` (required, no default) | port no longer defaulted; compose and gitops set 50057 |
| `SERVER_SHUTDOWN_GRACE_SECONDS=5.0` | `SHUTDOWN_GRACE` (10s) | 5s to 10s |
| `SERVER_REFLECTION_ENABLED=true` | `GRPC_REFLECTION_ENABLED=false` | on to off; strict ENV refuses true |
| `RUNTIME_LOG_LEVEL/JSON` | `LOG_LEVEL`, `LOG_JSON` (default true) | JSON by default |
| `POSTGRES_HOST/PORT/DB/USER/PASSWORD/SSLMODE` | `DATABASE_URL` (required), `DB_MAX_CONNS` (10, was 20) | pool 20 to 10 |
| OTel on when endpoint set | `OTEL_ENABLED` (false) | explicit |
| `KAFKA_ENABLED`, `OUTBOX_ENABLED` | deleted | always on |
| `OUTBOX_CLAIM_LOCK_SECONDS=60` | `OUTBOX_CLAIM_LOCK=60s` | same value |
| `OTEL_SERVICE_NAME` (default `team-chat`), `OTEL_EXPORTER_OTLP_ENDPOINT` | same names, SDK `OTel` group | the SDK group has no service-name default; `Load` fills `team-chat` when empty |
| `KAFKA_BROKERS` (default `localhost:9092`) | same name, SDK `Kafka` group | local ENV: SDK default `localhost:9092`; strict ENV: required |
| `OUTBOX_POLL_INTERVAL=1s`, `OUTBOX_BATCH_SIZE=100` | same names | none (SDK defaults equal today) |
| `OUTBOX_MAX_ATTEMPTS=10` | deleted | no attempt limit (D4 of the SDK outbox) |
| `UPSTREAM_LISTING_ADDR` | deleted | unused |
| none | `ENV` required | new; staging and prod overlays must set it |

`.env.example` and `TestEnvExampleInSync` stay: the test lists every `env` tag of `Settings` and compares with `.env.example`.

## D6 Caller-visible differences (accepted)

1. A duplicate-key SQLSTATE 23505 maps to `AlreadyExists` instead of `Internal` (grpcx table). Chat has no known path that returns it today (`GetOrCreateThread` upserts), so the effect is a different code on a race that is now correct.
2. A cancelled or timed-out request maps to `Canceled` or `DeadlineExceeded` instead of `Internal`.
3. gRPC reflection is off by default.
4. Operator-visible config defaults change (accepted, pinned by `TestDefaults`): pool 20 to 10 (`DB_MAX_CONNS`), shutdown grace 5s to 10s, JSON logs by default, `GRPC_PORT` not defaulted, OTel needs `OTEL_ENABLED=true` (an endpoint alone no longer enables it).
5. Pagination moves into one rpc helper that resolves page and size once (size below 1 or above 100 becomes the default, 20 or 50) and passes the effective size to `chat`; `next_cursor` is computed with that effective size. Today the handler used the requested size for `next_cursor` (size 101 on page 2 over 120 rows gave an empty cursor although rows remained); this is fixed, an accepted behaviour change. The cursor format is unchanged. `sender_name` derivation (`Người dùng` for ids of 6 characters or fewer) lives in the handler today and moves to `rpc`.
Everything else (codes, messages for validation, participant checks, anonymous rejection, `internal error` text) is unchanged and pinned by tests that exist today and are ported.

## D7 Logs

`logx` attaches request id, principal id and trace id from the context. The chat code logs only `op`, ids it already logs and the error cause, never `content`, `payload` or `sender_name`. A test captures the log output of a `SendMessage` whose content is `s3cret-needle` and asserts the needle is absent.

## D8 Test strategy

Unit tests use the in-memory fakes. Postgres tests use `testkit` (throwaway database); Docker-backed tests must run and a skip is a gate failure (human, Round 2). The Kafka relay test uses `testkit.Recorder` as the `outbox.Publisher`. A real-broker scenario is covered by the existing e2e feature `chat/chat_thread.feature` and `notification` features once at the end (user decision: e2e is not run per service).

## D10 migrate up subcommand

`cmd/server/main.go` dispatches on its arguments: `migrate up` calls `app.Migrate(ctx)`, anything else loads `config.Load()` and calls `app.Run`. `app.Migrate` loads `config.LoadMigrate()` (`ENV`, `DATABASE_URL` only), opens no Kafka client and no gRPC port, and calls `migrate.Up(ctx, databaseURL, migrations.Files)` where `Files` is the `embed.FS` rooted at the migrations. The Dockerfile copies nothing extra: migrations are embedded in the binary, so the compose `team-chat-migrate` service and the gitops PreSync Job use the service image with the command `server migrate up`. `app.Run` never calls it.

## D11 Outbox purge

`app.Run` starts one goroutine in the lifecycle stack that calls `outbox.Store.Purge(ctx, 7*24*time.Hour)` every hour (unexported constants `outboxRetention` and `purgeInterval`; the test overrides the interval through `Options`). Only published rows are deleted (SDK behaviour). A failure is logged with the cause and retried at the next tick. It stops with the relayer at shutdown. Not configurable by env on purpose: retention is a fixed product decision.

## Clarifications

### Round 1 — 2026-10-23
- Target `github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2`: require the tag, no replace, no third_party; reference `../platform-core/make/go-service.mk` and the SDK `.golangci.yml` (no copies; revive doc-comment rules are off, so add no doc comments); pin golangci-lint as a go tool like the SDK `go.mod`. ADR-0016 layout. (human, recipe)
- Delete every dependency enable flag (`DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, and any consumer, sweep or flag-server enable flag) and every runtime in-memory or noop fallback; importing a dependency means using it; in-memory implementations stay only as test fakes. Kept flags: `OTEL_ENABLED`, `GRPC_REFLECTION_ENABLED` (off by default, refused in strict ENV), `SEED_ADMIN_ENABLED` (identity only). (human, recipe)
- Mock is mock data served by the real service, never a mock service; switches move onto `pkg/mockdata` (`MOCK_DATA=<adapters>`, refused in strict ENV before validation). team-chat has no mock switch, so nothing moves. (human, recipe)
- Use the SDK for everything it covers (config, logx, otelx, healthx with readiness = overall and a service-independent `liveness`, lifecycle, grpcx error table including 23505 to AlreadyExists and Canceled/DeadlineExceeded, auth, postgres.Open, migrate, outbox, pagination); these behaviour changes are accepted. (human, recipe)
- Keep every current requirement and Q1-Q11/C4/privacy-hardening behaviour green; main's behaviour wins over the old migration; MODIFY only requirements that describe removed flags, fallbacks or env vars; ADD capability requirements where no spec describes the service. (human, recipe)
- No DPO sign-off is needed: basis legitimate interest, platform log retention. (human, recipe)
- e2e is not run per service now (once at the end); scenarios may still name e2e features in VERIFIED BY. Compose and gitops edits are not in this change's code tasks; they are listed under "Deploy track" in tasks.md. (human, recipe)

## D9 Test inventory

Filled while implementing task 1.7.2: each `Test*` of team-chat on main either appears in a scenario VERIFIED BY or is listed here as deleted with a reason. Replaced on purpose: the grpcserver reflection-flag cases (flag default flips; `TestReflectionOffByDefault`), the relayer attempt-limit cases (no attempt limit in the SDK outbox; `TestRelayerHasNoAttemptLimit`), tests of `KAFKA_ENABLED`, `OUTBOX_ENABLED` and the in-memory outbox (flags and fallbacks deleted).

Kept, moved to `internal/chat` (same name): `TestChatService_GetOrCreateThread`, `TestChatService_GetThreadAndMarkThreadRead`, `TestChatService_GetThreadMessages`, `TestChatService_ListThreads`, `TestChatService_SearchMessages`, `TestChatService_SendMessage`, `TestSendMessageOutboxFailureStoresNothing`.

Deleted or replaced, with reason:
- `TestBuildMessageOutboxRow`: row building moved to the SDK outbox envelope; replaced by `TestEnvelopeCarriesPrincipalAndRequestID`.
- `TestChatHandler_SearchMessages`: handler moved to `internal/rpc`; replaced by `TestSearchMessagesPagination`.
- `TestChatHandler_SendMessageAndEventEmission`: event emission now goes through `Store.EnqueueTx`; replaced by `TestSaveMessageIsAtomicWithOutbox`.
- `TestChatHandler_SendMessageEventCarriesRecipient`: replaced by `TestSendMessagePassesRecipientAndSellerToOutbox`.
- `TestSendMessagePassesRecipientToOutbox`: replaced by `TestSendMessagePassesRecipientAndSellerToOutbox`.
- `TestChatThreadAndMessageLifecycle`: grpcserver replaced by `internal/rpc`; replaced by `TestChatLifecycleOverGRPC`.
- `TestClaimPendingKeepsCreationOrder_Postgres`: custom claim deleted with the in-house relayer; replaced by `TestOutboxUpgradeKeepsRowOrder`.
- `TestInMemoryChatRepository_GetOrCreateThread`, `TestInMemoryChatRepository_ListThreadsForUser`, `TestInMemoryChatRepository_MessagesAndUnreadCounts`, `TestInMemoryChatRepository_SearchMessages`: in-memory repository is a deleted fallback; the Postgres store is covered by `TestUnreadCounts` and `TestSearchMessagesScopedToParticipant`.
- `TestInternalErrIsGenericAndLogsCause`: replaced by `TestInternalErrorIsMasked`.
- `TestOutboxDefaultsAndOverrides`: replaced by `TestOutboxOverrides` and `TestDefaults`.
- `TestRelayerDeliversAfterKafkaRecovers`: replaced by `TestMessageSentWhileKafkaDownIsDeliveredAfterRecovery`.
- `TestRelayerParksAfterMaxAttempts`: no attempt limit in the SDK outbox; replaced by `TestRelayerHasNoAttemptLimit`.
- `TestRelayerPublishesKeyedByThreadAndOnlyOnce`: replaced by `TestThreadEventsPublishInSendOrder`.
- `TestSaveMessageIsAtomicWithOutbox_Postgres`: replaced by `TestSaveMessageIsAtomicWithOutbox`.

### Round 2 — 2026-10-23
- Migrations: every service gets a `migrate up` subcommand in its binary (SDK `pkg/migrate`, embedded migrations); the service never migrates at boot; compose keeps a `team-chat-migrate` service running the service image with `migrate up`; gitops gets an Argo PreSync migrate Job per service (in deploy-go-sdk-v1). (human)
- Kafka is not a readiness check in any service (readiness = Postgres and required upstream checks only). team-chat has no upstream. (human)
- The shared deploy-runtime requirements (such as "Services refuse unsafe configuration in staging and production") are MODIFIED only by change deploy-go-sdk-v1; this change carries no MODIFIED copy and lists the team-chat bullet under the Deploy track. (human)
- Outbox upgrade: migration `0012` requeues rows parked as `failed`; the first rollout scales old pods to 0 before new pods start (Recreate once, never two relayers); published rows are purged after 7 days by `Store.Purge` on a schedule in the service (D11). (human)
- Upstream addresses and clients the service uses are required in every ENV with no fallback mode; team-chat uses none, so nothing changes here. (human)
- Local compose sets `GRPC_REFLECTION_ENABLED=true` for every service (Deploy track line); strict ENV still refuses it. (human)
- Accepted: the SDK 2 s upstream bound replaces any `UPSTREAM_CALL_TIMEOUT*`; team-chat has none. (human)
- Docker-backed tests must run: a skip is a gate failure. (human)
- The agora repo is public: no `GOPRIVATE` or credentials are needed to fetch the SDK. (human)
