## Context

team-analytics ships three binaries from one image (`/consumer`, `/ga4-relay`, `/ga4import`). `/consumer` is a Kafka worker that also serves gRPC health and, on the DuckDB adapter, the read-only `AnalyticsQueryService`, an internal HTTP listener on `METRICS_ADDR` (`/` Prometheus text, `/internal/ga4import` trigger), a daily GA4 import scheduler and a Parquet export loop. `/ga4-relay` is a second Kafka worker with its own DuckDB file. `/ga4import` is a thin HTTP client of the trigger. There is no Postgres: SDK packages `postgres`, `migrate`, `outbox`, `idempotency`, `consume` dedupe, `upstream`, `auditx`, `flags`, `pagination` and `jwtx` do not apply. Warehouse idempotency stays in the DuckDB writers (event id keys). There is no privileged mutation and no outbound RPC, so no audit write is added.

SDK used: `config`, `logx`, `otelx`, `healthx`, `lifecycle`, `grpcx`, `auth`, `kafkax`, `consume`, `mockdata`, `reqctx` (through grpcx and consume). Not used, with the reason: `resilience` (D7), the rest above (no Postgres, no upstream, no audit).

## D1 Layout and package moves

```
cmd/consumer/main.go      -> app.RunConsumer(ctx, settings)
cmd/ga4-relay/main.go     -> app.RunGA4Relay(ctx, settings)
cmd/ga4import/main.go     -> ga4import.RunCLI(ctx, args, stdout, stderr)
internal/app/             composition roots, openWarehouse (WAREHOUSE_DRIVER switch), const service
internal/config/          Settings + groups + DeclaredEnvKeys test hook; test reads ../../.env.example
internal/rpc/             AnalyticsQueryService handlers, error table, Register(srv, handler)
internal/query/           Repository, DuckDB repository, aggregation helpers (no grpc, no codes)
internal/events/          (was internal/consumer) envelope -> record mapping, batch handlers
internal/warehouse/       WarehouseWriter seam, duckdb and bigquery adapters, fake (tests only)
internal/ga4import/       Importer, Source (bigquery, fixture), scheduler, trigger handler, CLI
internal/ga4relay/        relay, client, mapping and ledger (SQL), purge
internal/export/          Parquet export loop (unchanged)
internal/observability/   reconcile gauge and metrics handler (logging.go deleted)
```

`internal/store` is not created: ADR-0016 reserves it for Postgres queries and migrations, and the warehouse adapters already live in `internal/warehouse`. Renaming them would only move files. `internal/consumer` becomes `internal/events` as the ADR names it. `query.Service` mixes proto assembly, authorization and aggregation; it moves to `internal/rpc` (it must call `codes` and `auth`), while `internal/query` keeps what has no transport dependency. Rejected: keeping `query.Service` in place (a domain package importing grpc breaks the ADR rule) and a full rewrite of the aggregations (out of scope, behaviour must not change).

## D2 Settings

```go
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	OTel      sdkconfig.OTel
	Kafka     Kafka            // holds sdkconfig.Kafka (named, not embedded) + topics and groups
	LogKeyFP  sdkconfig.LogKeyFP
	MockData  sdkconfig.MockData
	Metrics   Metrics          // METRICS_ADDR keepempty
	Warehouse Warehouse
	Batch     Batch
	Tracking  Tracking
	Recs      Recs
	Engagement Engagement
	GA4       GA4
}
```

`Settings.Validate` keeps every existing cross-field rule (ports, batch size, tracking thresholds, driver fields, GA4 ratio, relay fields when the relay has credentials) and loses the `Enabled` and `IsProd` helpers. String settings that were `default:"..."` and could be set empty on purpose keep `keepempty:"true"` only where an empty value is meaningful (`METRICS_ADDR`, which disables the listener). `GRPC_PORT` has no default in the SDK; the Dockerfile keeps `ENV GRPC_PORT=50059`, so the three binaries get it from the image and manifests need not repeat it; `docker-compose.local.yaml` sets `GRPC_PORT: "50059"` explicitly, because the SDK requires the key and `TestStandaloneComposeLoadsUnderLocalEnv` loads that file, which never sees the Dockerfile `ENV`. `OTEL_SERVICE_NAME` stays an accepted key: `sdkconfig.OTel` declares it, `otelx.Init(ctx, s.OTel, service)` uses it when set and the binary name otherwise, and it is listed in `.env.example`.

Strict ENV adds required keys beyond the SDK ones, checked in `Settings.Validate` (after the SDK `Load` rules): `GA4_API_SECRET` and `GA4_MEASUREMENT_ID` (the relay must be able to send; a strict ENV never boots a disabled relay) and `GA4_EXPORT_DATASET` (the import must read BigQuery; `MOCK_DATA` is refused there). In `local` and `test` the existing optional behaviour stays (relay disabled without credentials, import no-op without dataset).

`GA4_IMPORT_TRIGGER_URL` moves from `os.Getenv` in `cmd/ga4import` into `GA4` (default `http://localhost:9109/internal/ga4import`) so every key is declared and in `.env.example`.

## D3 No flags, no fallbacks

Deleted: `Kafka.Enabled` and the `errors.New("KAFKA_ENABLED must be true ...")` guard, and `Runtime.IsProd`. The listing and engagement consumers, the GA4 import and the query service need DuckDB tables; today they are skipped by a type assertion on the writer. That assertion stays as a capability check of the BigQuery driver (D8); it is not a flag, has no environment switch and its boot log line is not asserted by a test. In-memory implementations reachable from a binary (`query.MemoryRepository`, `ga4relay.NewMemoryLedger`, `ga4relay.NewMemoryMappingStore`) move to `_test.go` files of their packages; a test lists the dependencies of the three binaries with `go list -deps` and fails when a non-test file in them declares `NewMemory*`, `NewInMemory*` or `Noop*`. `warehouse/fake` is a separate package imported only from tests.

Kept flags: `OTEL_ENABLED`, `GRPC_REFLECTION_ENABLED` (off by default, refused when `ENV` is strict; local compose sets it true, deploy track). `SEED_ADMIN_ENABLED` belongs to identity. Credential gating of the GA4 relay (no `GA4_API_SECRET` or `GA4_MEASUREMENT_ID` means disabled) and the empty `GA4_EXPORT_DATASET` no-op are existing spec'd behaviour of optional external integrations in `local` and `test` only; in a strict ENV those keys are required (D2), so a deployed relay or import can never run as a silent no-op. Other values that disable a loop are kept for the same reason and are not dependency enable flags: `PARQUET_EXPORT_INTERVAL_SECONDS=0` disables the Parquet export loop (`export.Enabled`; the export feeds the recsys training files from the warehouse, nothing is faked and no dependency is bypassed), an empty `METRICS_ADDR` disables the metrics listener (`keepempty`), and an empty `GA4_EXPORT_DATASET` makes the import a no-op. None selects an in-memory or noop substitute for Postgres, Kafka or the warehouse; each only turns off an optional job.

## D4 Mock data

`mockdata.New(s.MockData, "ga4export")` is built in `app`. When `Active("ga4export")` the importer's `Source` is the Parquet fixture source reading `GA4_EXPORT_FIXTURE_DIR`; otherwise it is the BigQuery source on `GA4_EXPORT_DATASET`. Only the outer `Source` changes: `Importer.ImportDay`, the atomic `ga4_sessions` replace, the import-run record and the reconcile gauge are the real code. `GA4_EXPORT_SOURCE` is deleted. `LogActive` logs a warning once. Strict ENV with `MOCK_DATA` set fails in `config.Load` before validation. No other analytics adapter has a mock (BigQuery warehouse and Kafka are always real); the GA4 relay's `GA4_ENDPOINT` stays a plain URL setting and the e2e GA4 stub it points at is outside this change.

## D5 Consumers

| Stream | Today | After |
|---|---|---|
| `analytics.events` + `order.events`, group `KAFKA_CONSUMER_GROUP` | `consumer.Consumer.Run`, Batcher across polls, ticker flush, `CommitUncommittedOffsets`, poison logged with raw key | `consume.BatchConsumer`; `MaxPollRecords = BATCH_MAX_SIZE`; client option `kgo.FetchMaxWait(BATCH_FLUSH_INTERVAL_SECONDS)`; DLQ `KAFKA_DLQ_TOPIC` |
| `listing.events`, group `KAFKA_LISTING_CONSUMER_GROUP`, earliest offset | own loop, retry forever | `consume.BatchConsumer`, `kgo.ConsumeResetOffset(AtStart)`, DLQ `KAFKA_LISTING_DLQ_TOPIC` |
| `engagement.events`, group `ENGAGEMENT_CONSUMER_GROUP` | own loop, `DeadLetter` with header `error` | `consume.Consumer` (per record); undecodable or unknown type returns `consume.Permanent`, SDK writes the dead-letter record to `ENGAGEMENT_DLQ_TOPIC` and commits |

Handler rules:

- Tracking and order: split a fetch into tracking rows and order-fact rows, write both, return nil. A write error is returned as is, so the SDK retries the same batch up to 5 times (writes are idempotent on event id), then dead-letters the whole batch and commits. The batch is therefore parked, not durably written: a warehouse outage longer than the 4 backoff waits (100, 200, 400, 800 ms, about 1.5 s) moves whole batches to `KAFKA_DLQ_TOPIC`, which holds them for 7 days. Recovery is a replay: the dead-letter records keep the original key and value, so a runbook step re-produces them to `analytics.events` (writes are idempotent on event id, so a replay of a batch that was partly written is safe). The `tracking` requirement "Events are written in batches for throughput" is MODIFIED to say so (delta `specs/tracking`). A batch at `BATCH_MAX_SIZE` is bounded by the group rebalance timeout (franz-go default 60 s): one DuckDB write of 500 rows must finish in under 5 s, so 5 attempts plus about 1.5 s of waits stay under 27 s (scenario with `TestMaxSizeBatchWriteFitsRebalanceBudget`). A non-tracking, non-order envelope is skipped. An undecodable record is counted with `RecordDecodeFailures`, logged with `key_fp`, skipped and never returned as an error (a returned error would dead-letter the good records of the batch), exactly as today.
- Listing: undecodable record logged with `key_fp` and skipped (as today); rows upserted in one call.
- Engagement: rebalance budget `MaxPollRecords x (4 backoff waits of about 1.5 s + 5 x handler time)` must stay under 30 s, half of the 60 s rebalance timeout; with a one-row DuckDB write (about 50 ms) `events.EngagementOptions(s)` sets `MaxPollRecords = 10`, giving `10 x (1.5 s + 0.25 s) = 17.5 s` (scenario "Engagement polls fit the rebalance timeout", `TestEngagementOptionsFitRebalanceBudget`). A good record is written with `WriteEngagementFacts` (one row). The scenario "Analytics review hidden: a corrupt payload is dead-lettered" in `analytics-review-hidden` asserted an `error` header and `ProcessEngagementRecords`; it is MODIFIED by this change (delta `specs/analytics-review-hidden`): the SDK dead-letters to `ENGAGEMENT_DLQ_TOPIC` with `dlq-*` headers and the other records are still written.
- Dead-letter copies of a tracking record carry no `ga_client_id` without analytics consent. The SDK writes the dead-letter record with the original value, so the `consume.DLQProducer` handed to the tracking consumer (`KAFKA_DLQ_TOPIC`) and to the relay consumer (`GA4_RELAY_DLQ_TOPIC`, which also receives `analytics.events` records) is `events.NewRedactingDLQ(producer, fp, logger)`. Its `PublishRecord` decodes the value as an `EventEnvelope`; when the payload type is `platform.analytics.v1.TrackingEvent` and `consent.analytics` is false (or `consent` is absent, or `consent.version` is 0, negative or unknown, which the proto defines as all denied) it clears `ga_client_id`, re-marshals and publishes with the same key, headers and topic. A record that is not a tracking envelope, or whose value cannot be decoded, is published unchanged (an undecodable value has no parseable field to blank; assumption, see Clarifications). The listing, engagement and order dead-letter topics carry no `ga_client_id` and use the plain producer. The warehouse already blanks the id on write (`openspec/specs/consent`); this closes the same gap on the parked copy.
- Group names, topics and start offsets are unchanged, so no group replays on deploy.
- `kafkax.NewFingerprinter(s.LogKeyFP.Secret)` is built once; the same instance goes into every `Options.KeyFP` and into handler logs.

Why a `BatchConsumer` for the main stream. The earlier attempt on `feat/go-sdk-v1-e2e` kept a custom loop because the old Batcher buffers across polls. A custom loop keeps raw-key logging, no DLQ and a second commit protocol to maintain. The requirement is "flush on size or max interval, commit only after a durable write" (`openspec/specs/tracking`), which a fetch capped at `BATCH_MAX_SIZE` and waiting at most `BATCH_FLUSH_INTERVAL_SECONDS` meets. The cost is smaller writes at low traffic (a fetch returns as soon as data exists); DuckDB appends are cheap and the volume is about 100 events/s. Rejected: keeping the custom loop; lowering `FetchMinBytes` to force bigger fetches (adds latency for no requirement).

`kgo` is imported by `internal/app` only for `FetchMaxWait` and `ConsumeResetOffset` options passed to `kafkax.NewConsumerClient`. `consume.NewBatchConsumer` requires a group client with `BlockRebalanceOnPoll`, which `NewConsumerClient` sets; `stack.Push` closes each client after `AllowRebalance`.

## D6 gRPC server, auth and errors

`grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})` gives request id, recovery, logging and the principal-from-metadata interceptor (`auth`). `rpc.Register(srv, handler)` registers `AnalyticsQueryService` only when the warehouse is DuckDB (as today; the BigQuery query path is a follow-up). Each handler authorizes first:

- Seller RPCs (`GetSellerFunnel`, `GetRevenueBreakdown`, `GetDemandForecast`): `auth.RequirePrincipal` (anonymous or none: `Unauthenticated`), then allow when `p.HasScope("admin")` or (`p.Type == auth.TypeUser` and `p.ID == seller_id`), else `PermissionDenied`. The check runs before any repository call, as today.
- Admin RPCs (`GetPlatformOrderSummary`, `ListRecentOrders`, `GetChannelFunnel`, `GetTrackingQualityReport`, `GetRecommendationPerformance`): `auth.RequireScopes(ctx, "admin")`.
- Validation failures (`seller_id` or `listing_id` required, `window_hours` out of range, `from` after `to`) stay `status.Error(codes.InvalidArgument, ...)` returned by the handler and pass through `grpcx.Status` unchanged. A repository that lacks an optional capability (tracking quality, recommendation performance, channel funnel on a warehouse without it) returns `query.ErrUnsupported`, mapped to `Unavailable` with the fixed text `not available on this warehouse` by `grpcx.NewErrorTable`. The "repository not configured" branch is deleted: it existed only for the no-database fallback. Repository errors, which today become `Internal` with `fmt.Sprintf("seller funnel: %v", err)` (driver text to the caller), become `internal error`. `PermissionDenied` text changes from `insufficient_scope: missing "admin"` to the SDK's `missing scope`; scenarios assert codes, not text.

`auth.HasScope` does not treat `admin` as a wildcard; `admin` is the exact scope the seller-analytics-access spec already names, so the rule is unchanged.

Reflection: `GRPC_REFLECTION_ENABLED` defaults to false (was true). Nothing in compose or gitops sets it; local grpcurl users set it with `ENV=local`.

## D7 GA4 relay

The relay becomes a gRPC-health-only process: `grpcx.NewServer` with the `healthx` server and no business service, `grpcx.Serve`, `consume.Consumer` for `analytics.events` and `order.events` in group `GA4_RELAY_CONSUMER_GROUP`, DLQ `GA4_RELAY_DLQ_TOPIC` (same topic, SDK headers). `GA4_RELAY_PROBE_PORT` is dropped; probes use `GRPC_PORT` (image default 50059). When the credentials are missing the process logs `ga4 relay disabled`, starts no consumer, opens no DuckDB file and no Kafka client, and its health reports SERVING (existing behaviour, spec'd).

`Relay.Handle(ctx, rec consume.Record) error`: returns nil for processed, ignored and already-sent records; `consume.Permanent(err)` for an undecodable envelope, an `OrderPaidEvent` without order id, or a `SendError` (4xx or exhausted), after incrementing `ga4_relay_failed_total{reason}`; any other error (ledger or mapping store failure, payload build failure) is transient and retried by the SDK with its backoff, and increments `ga4_relay_failed_total{reason="store"}` once per failed attempt. The SDK limit applies here too: after 5 failed attempts (about 1.5 s of waits) the record is dead-lettered to `GA4_RELAY_DLQ_TOPIC` and committed. For an `OrderPaidEvent` that parks a GA4 purchase; for a record on `analytics.events` it parks a buyer-to-client-id mapping, and later purchases of that buyer are skipped under "No consented client id, no send" until the mapping is replayed. The loss is not accepted silently: the relay requirement "Retry and failure handling" is MODIFIED (delta `specs/ga4-purchase-relay`) with a scenario that proves the dead-letter, the counter and that replaying the parked purchase sends exactly one request (the ledger makes replay idempotent). Replaying mapping records after purchases can leave an older client id as the latest; the relay accepts that because the mapping is a best-effort attribution aid. The mapping handler for `analytics.events` follows the same rule. The Kafka offset commit and the dead-letter publish are the SDK's, so `Message`, `DLQProducer`, `OffsetCommitter`, `committer`, `dlqProducer` and `relayConsumer.loop` are deleted. The per-order lock and the ledger stay: they make redelivery and concurrent duplicates send once.

Rebalance budget of the relay. `consume.Consumer` blocks group rebalances while a poll is handled (`Options.MaxPollRecords` doc: `MaxPollRecords x MaxAttempts x handler time` plus backoff must fit the 60 s rebalance timeout; default 500 records). During a GA4 outage `Relay.Handle` blocks inside `Client.Send` for the gaps 1 + 2 + 4 + 8 s = 15 s, and up to 40 s when GA4 hangs (5 attempts of 5 s request timeout plus the gaps); the resulting `SendError` is `Permanent`, so the SDK does not retry on top. The relay therefore sets `MaxPollRecords = 1` (`ga4relay.ConsumerOptions(s)`): `1 x 40 s < 60 s`, and a single record is committed per poll. Store failures are transient and retried by the SDK: handler time is milliseconds, so `1 x (1.5 s + 5 x handler)` is also well inside the budget. The cost is one offset commit per record on a stream that also carries `analytics.events` (mapping upserts); the relay's lag is not an SLO and the mapping handler is a single row write. Rejected: bounding the `Client` retry loop (the spec fixes 5 attempts and the 1, 2, 4, 8 s gaps); a larger poll (a 5-order poll in an outage already passes 60 s, the member is kicked and the poll is redelivered). Scenario "A GA4 outage does not exceed the rebalance timeout" with `TestRelayConsumerOptionsFitRebalanceBudget`.

The GA4 `Client` keeps its own retry loop (5 attempts, gaps 1 s, 2 s, 4 s, 8 s, cap 30 s) rather than `pkg/resilience.Retry`, because the spec requires gaps of at least 1, 2, 4 and 8 s and `resilience.Retry` applies jitter, which can shorten a gap. Rejected: resilience with a zero jitter option (none exists in v1.0.2; changing the SDK is out of scope).

Readiness of the relay: only a `duckdb` ping check, registered only when the relay is enabled (the dependency exists only then). Kafka is not a readiness check (D8).

## D8 Health and lifecycle

`healthx.New(healthx.DefaultInterval, logger)`; checks: `duckdb` (`dw.DB().PingContext`) when the warehouse is DuckDB, and no Kafka check: readiness covers the warehouse and required upstream checks only (human, Round 2); analytics has no upstream RPC client, and a broker outage shows as consumer lag and in the SDK consumer logs, not as NOT_SERVING (a Kafka check would unready the pod without any pod-local remedy). The BigQuery adapter has no cheap ping; the driver is covered by its open-time failure. A BigQuery writer is neither a `*duckdb.Writer` nor a `ListingSellerWriter` or `EngagementFactWriter`, so `RunConsumer` skips by type assertions, as `cmd/consumer/main.go` does today: the `AnalyticsQueryService`, the GA4 import and the Parquet export (DuckDB writer) and the listing and engagement consumers (capability interfaces). The tracking and order consumer and the health service still run. This matches today's behaviour; scenario `TestNonDuckDBWarehouseServesHealthOnly` covers the visible part. The consumer binary then composes:

```
lifecycle.Run(ctx, Options{ShutdownGrace: s.ShutdownGrace, DrainDelay: s.DrainDelay,
    OnShutdown: {health.Drain}, Closers: {stack.Close}, Logger: logger},
  lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), workers...), lifecycle.Func(health.Run))
```

Workers: tracking and order batch consumer, listing, engagement, metrics HTTP (`lifecycle.HTTP`), GA4 scheduler, Parquet export, each wrapped so a job that gives up waits for shutdown instead of ending the process. `StopAfter` stops the workers after the server has drained, so the last fetch is still handled. Stack order (closed in reverse): otel shutdown, warehouse, Kafka clients (`AllowRebalance` then `Close`). A failure to open the warehouse fails boot in every ENV including `local`.

`GRPC` listener grace and `lifecycle` grace use the same `SHUTDOWN_GRACE`. Pod `terminationGracePeriodSeconds` must exceed `SHUTDOWN_DRAIN_DELAY + SHUTDOWN_GRACE`; the deploy track checks it.

## D9 Logging

`logx.New(s.Runtime, service)` then `slog.SetDefault`. Calls use `*Context` variants and key-value pairs (the shared `.golangci.yml` runs `sloglint` with `kv-only` and `context: scope`). The GA4 secret is never passed to a logger or error; the existing test `TestGA4RelayInvalidConfigErrorDoesNotContainSecret` stays.

## D10 Test and fixture strategy

Unit tests use `warehouse/fake`, `consume` handlers called directly, and `grpcx.ServeListener` on `127.0.0.1:0`. DuckDB tests use a temp file. Dead-letter and group-progress scenarios use `testkit.Redpanda(t)` with `testkit.Produce` and `testkit.ReadAll` (Docker; `make check` sets `GO_SDK_REQUIRE_DOCKER=1`, so they fail instead of skipping, and the gate additionally fails when the `go test -v` output contains a `--- SKIP` line: a skipped Docker-backed test is a gate failure). The SDK keeps `consume.Permanent` detection private, so handlers return it wrapped around a package sentinel (`events.ErrDeadLetter`, `ga4relay.ErrDeadLetter`) that unit tests assert with `errors.Is`, and one Redpanda test per consumer proves the SDK then dead-letters and commits. Each `VERIFIED BY` below names a Go test; the cross-service flows stay in `team-analytics/FEATURES.yaml` as `not-testable` backend entries, verified by the platform-e2e integration test run once at the end.

## Risks

- Smaller DuckDB writes at low traffic (D5). Mitigation: the batch cap and the 5 s probe interval are unchanged; the burst check `TestBurstOf1000EventsIsWrittenAtOrAbove100PerSecond` (tracking scenario "A 1000-event burst is written at 100 events/s or faster", task 1.7.2) proves at least 100 events/s on the batch consumer.
- A DLQ copy of a poison batch holds user ids (but not `ga_client_id` without consent, D5). Mitigation: 7-day retention topics, deploy track.
- Deploy ordering: the new binaries refuse a missing `ENV`, so the manifests and compose must ship together with the image (deploy change). The old image ignores the new keys.
- `GRPC_PORT` and `ENV` missing in a stale manifest fail boot loudly (intended).

## Clarifications

### Round 1 — 2026-10-23
- Target `github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2` (tag exists): require the tag, no `replace`, no `third_party`; reference `../platform-core/make/go-service.mk` and the SDK `.golangci.yml` without copies; add no doc comments (revive doc-comment rules are off); pin `golangci-lint` as a `go tool` like the SDK `go.mod`; ADR-0016 layout (human, recipe)
- Delete every dependency enable flag (`DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `FEATURE_FLAGS_ENABLED`, `*_CONSUMER_ENABLED`, `SESSION_SWEEP_ENABLED`, ...) and every runtime in-memory or noop fallback (`NewInMemory*` in main or bootstrap, `NoopPublisher`, `RequireDurableStorage`): importing a dependency means using it; in-memory implementations stay only as test fakes; kept flags are `OTEL_ENABLED`, `GRPC_REFLECTION_ENABLED` (off by default, refused in strict ENV) and `SEED_ADMIN_ENABLED` (identity) (human, recipe)
- Mock is mock data served by the real service, never a mock service: existing mock switches move onto `pkg/mockdata` (`MOCK_DATA=<adapters>`, refused in strict ENV before validation) and the real code path always runs (human, recipe)
- Use the SDK for everything it covers: config, logx, otelx, healthx (readiness is overall, liveness service independent of dependencies, `probes.livenessService: liveness`), lifecycle, grpcx (error table including 23505 to AlreadyExists, Canceled and DeadlineExceeded codes), auth, postgres.Open, migrate, kafkax and consume (`key_fp` HMAC logging with `LOG_KEY_FP_SECRET`, DLQ), outbox, idempotency, upstream, auditx (Writer interface, fail closed "audit trail unavailable"), flags, pagination (keep existing cursor formats), resilience, jwtx where relevant; the resulting behaviour changes are accepted (human, recipe)
- Keep every current behavioural requirement and every Q1 to Q11, C4 and privacy-hardening behaviour on this branch green; main's behaviour wins over the old migration on `origin/feat/go-sdk-v1-e2e` (reference only); MODIFY only requirements that describe removed flags, fallbacks or env vars; ADD new capability requirements where no spec describes the service (human, recipe)
- No DPO sign-off needed: basis legitimate interest, platform log retention (human, recipe)
- e2e is not run per service now (run once at the end); scenarios may still name e2e features in VERIFIED BY; no `docker-compose.services.yaml` or `platform-gitops` edits in this change's code tasks, they are listed in a "Deploy track (done by change deploy-go-sdk-v1)" section of `tasks.md` (human, recipe)

### Round 2 — 2026-10-23
- Migrations: every service gets a `migrate up` subcommand (SDK `pkg/migrate`, embedded migrations), never migrates at boot, compose keeps a `<svc>-migrate` service and gitops an Argo PreSync Job (deploy-go-sdk-v1). team-analytics has no Postgres and no migration files (the DuckDB schema is created by the warehouse adapter at open), so this change adds no `migrate` subcommand, no compose migrate service and no PreSync Job; outbox purge and requeue (decision 4) do not apply either (human)
- Kafka is not a readiness check in any service: readiness is the warehouse ping (the analytics counterpart of Postgres) and required upstream checks only; the consumer and the relay register no Kafka check, so a broker outage does not flip readiness (human)
- The shared deploy-runtime requirement "Services refuse unsafe configuration in staging and production" is MODIFIED only by change deploy-go-sdk-v1; this change holds no MODIFIED copy of it and lists the team-analytics bullet under the Deploy track (human)
- Raw gitops manifests of analytics and ga4-relay get strict ENV overlays in deploy-go-sdk-v1; `LOG_KEY_FP_SECRET` is provisioned in Vault by the user (human task, tasks 3.12) (human)
- Upstream addresses and clients are required in every ENV with no fallback mode; team-analytics has no upstream RPC client, so nothing is removed here (human)
- `ga_client_id` is redacted from a tracking record before it is published to a dead-letter topic when `consent.analytics` is false (D5, scenarios `Dead-letter copies of tracking records carry no ga_client_id without analytics consent`) (human)
- Local compose sets `GRPC_REFLECTION_ENABLED=true` for every service (Deploy track 3.1, 3.2) (human)
- `OTEL_SERVICE_NAME` stays an accepted key (SDK OTel group) and is listed in `.env.example` (human)
- `GA4_API_SECRET`, `GA4_MEASUREMENT_ID` and `GA4_EXPORT_DATASET` are required in a strict ENV (D2); the relay probe is gRPC health (human)
- Accepted: the platform-wide behaviour changes of decisions 10 to 13 that name other services (gateway, order, promotion, payment, engagement) do not touch this change; the SDK 2 s upstream bound does not apply (no upstream calls) (human)
- Docker-backed tests must run: a skip is a gate failure (D10, task 1.9.1) (human)
- Assumption (not a human decision): an undecodable value on a tracking dead-letter record is published unchanged because no field can be located in it
