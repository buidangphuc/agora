## Context

team-domain on main (branch feat/go-sdk-all = main + C4 + q11) has `cmd/server/main.go` (213 lines, hand wiring), `internal/bootstrap` (resources, health poll, relayer and sweeper goroutines, identity addon), `internal/config` (own env binder plus `envcheck.go`), `internal/consumer` (own franz-go loop with DLQ and `key_fp`), `internal/events` (relayer, Kafka and no-op publisher), `internal/upstream` (audit and team-ai clients), `internal/interceptor` (principal and tracing), `internal/handler`, `internal/service`, `internal/repository` (Postgres plus in-memory fakes used at runtime when `DATABASE_ENABLED=false`), `internal/storage`. The reference port on `origin/feat/go-sdk-v1-e2e` (`internal/app`, `rpc`, `listing`, `store`, `objects`) is a layout guide only: main's behaviour wins (seller hold, moderation, screening, audit, stock events, key_fp, privacy hardening are newer than that port).

## D1 Layout (ADR-0016)

`cmd/server/main.go` parses `os.Args`, loads settings and calls `app.Run(ctx, settings)`; `server migrate up` calls `app.Migrate`. Packages (old to new):

| Old | New | Notes |
|---|---|---|
| `handler` | `rpc` | gRPC handlers and `Register`; error table in `rpc/errors.go` |
| `service` | `listing` | `ListingService`, storefront, bundle, moderation, `ReservationSweeper` |
| `repository` | `store` | Postgres repos, `PgTxWriter`, outbox glue, seller hold |
| `consumer` | `events` | identity consumer handler next to the envelope builders |
| `events` (relayer, publisher) | deleted | replaced by `pkg/outbox` and `pkg/kafkax` |
| `upstream` (ai) | `screen` | `screen.AIClient` over `upstream.NewClient` |
| `upstream` (audit) | `audit` | `audit.Writer` implements `auditx.Writer` over the generated `AuditServiceClient` |
| `storage` | `objects` | S3 presigner, unchanged logic |
| `bootstrap`, `grpcserver`, `interceptor`, `observability` | `app` (+ SDK) | deleted; `app.Run` is the only composition root |
| `config` | `config` | embeds SDK groups; `envcheck.go` deleted (SDK `config.CheckEnvExample`, `config.DeclaredEnvKeys`); `TestEnvExampleInSync` keeps the `make check-env` contract |

Domain packages keep their file names where possible so `git log --follow` works; the move is mechanical first (task 1), behaviour edits second.

## D2 Settings on SDK groups

`config.Settings` embeds `sdkconfig.Runtime`, `GRPC`, `Postgres`, `OTel`, `Kafka`, `Outbox`, `Audit`, `LogKeyFP` and adds local groups `Events` (`KAFKA_LISTING_TOPIC`, `KAFKA_IDENTITY_TOPIC`, `KAFKA_IDENTITY_GROUP`), `Storage` (`STORAGE_*`), `Reservation` (`RESERVATION_TTL`, `RESERVATION_SWEEP_INTERVAL`), `Upstream` (`UPSTREAM_AI_ADDR`, `UPSTREAM_AI_TIMEOUT_SECONDS`) and `Screen` (`SCREEN_TERMS_FILE`). A local `Validate()` keeps the domain-specific refusals: default `minioadmin` keys in a strict ENV; an empty `UPSTREAM_AI_ADDR` or `UPSTREAM_AUDIT_ADDR` in every ENV (the SDK `Audit` group requires the audit address only when strict, so the local `Validate()` adds the non-strict check; decision D11); a non-empty `SCREEN_TERMS_FILE` that cannot be stat-ed, refused naming the variable (existing spec `listing-status-ownership` "Missing terms file stops start-up", `TestScreenTermsFileMissingFails` stays green); a negative `SHUTDOWN_GRACE` or `SHUTDOWN_DRAIN_DELAY`. `GRPC_PORT` has no default and is required by the SDK `GRPC` group (today's config defaults it to 50051), so every deployment must set it; the range check is covered by the SDK `GRPC.Validate` (port above 0); the environment normalisation (trim, lower-case) is the SDK `Runtime.IsStrict`, and a table case `" Production "` keeps it from being dropped silently. The SDK refuses only an empty `DATABASE_URL` (no default-URL notion), which matches today's behaviour. `config.Load[Settings]()` calls `ResolveFor`, so the SDK-owned refusals (reflection, `KAFKA_BROKERS`, `LOG_KEY_FP_SECRET`, strict `UPSTREAM_AUDIT_ADDR`) run before group validation and name the variable.

Reservation durations keep today's rule (warn and fall back, never stop the service: spec `inventory-reservations`), so they stay strings parsed by a local helper rather than `config.PositiveDuration`, which returns an error. `UPSTREAM_AI_TIMEOUT_SECONDS` stays a float of seconds (existing spec `listing-status-ownership` names it); only the SDK groups use unit-less duration keys.

`OTEL_SERVICE_NAME` has no default in config; `app.Run` calls `otelx.Init` with the service name `team-domain`, which the SDK uses when the variable is empty, so the exported name is unchanged (scenario "Domain SDK: The telemetry service name is team-domain").

No `MockData` group: team-domain has no mock switch today (the screening backend mock lives in team-ai). The rule "mock = mock data in the real service" is therefore satisfied vacuously; nothing is added.

## D3 No flags, no runtime fallbacks

`main.go` today builds `NewPostgresListingRepository` only `if res.Outbox != nil`, else `NewListingService(repo)` without events; the `Resources` struct may hold a nil pool; `OpenResources` picks `events.NoopPublisher{}` when Kafka is off. All of that is deleted: `app.Run` opens the pool (`postgres.Open`), builds the `kafkax.Producer` and runs the relayer, the sweeper and the identity consumer unconditionally. `NewListingService` (no outbox) and the `if res.Pool != nil` guard disappear; `InMemoryListingRepository`, `InMemoryOutbox` and `listing_fake_hold.go` move to `internal/store/storetest` (non-test file names so other packages' tests can import them) and a source scan test keeps them out of non-test importers.

Upstream addresses are required in every ENV (human, round 2): an empty `UPSTREAM_AI_ADDR` or `UPSTREAM_AUDIT_ADDR` refuses boot, so there is no nil `screener` and no nil audit writer; `app.Run` builds `screen.AIClient` and `audit.Writer` unconditionally (`auditx.Dial` is given a non-empty address). `TestNoRuntimeFallbacksInSource` therefore allows no nil-valued substitute. The existing main scenarios that described the unset-address modes (`Auto-screen: Unconfigured team-ai fails safe`, `Listing audit: No audit upstream configured`, `Moderation: No audit upstream configured`) keep their titles because `openspec` refuses to drop a scenario from a MODIFIED block, and now assert the config refusal; the matching unit tests (`TestNewAIClientEmptyAddrIsNil`, `TestCreateWithEventNilScreenerPendingReview`, `TestAdminUpdateNilAuditWriterUnavailable`, `TestUpdateWithEventAdminNilAuditUnavailable`, `TestModerateNilAuditUnavailable`) are deleted. Tests that need no audit or screen call pass a fake, never a nil.

## D4 Health and lifecycle

`healthx.New(healthx.DefaultInterval, logger)` with the single check `postgres` (`postgres.Checker(pool)`); the old 10 s hand-rolled poll goes away. Kafka is deliberately not a check (human, round 2): a broker outage must not take the pod out of rotation for reads, and the outbox keeps the rows pending. No upstream check is added for team-ai or team-audit: both are called per request with fail-safe (`pending_review`) or fail-closed (`Unavailable`) behaviour. Readiness is the overall `""` status; `liveness` is SERVING from construction. The gitops value `probes.livenessService: liveness` is set in the deploy track so a dependency outage stops traffic without restarting the pod.

`lifecycle.Run` components: `lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), relayer, sweeper, purger, consumer)` then `lifecycle.Func(health.Run)`. The dependents keep running until the gRPC server has stopped so an RPC that enqueues an outbox row during the grace period still has its row relayed; `OnShutdown: health.Drain`, `DrainDelay: s.DrainDelay`, closers (`stack.Close`) release the producer, the pool and telemetry in reverse order. The sweeper is `lifecycle.Every(interval, svc.SweepOnce...)` or a `Component` wrapping the existing `ReservationSweeper.Run`; the decision is local to task 1.6.2 and must keep the start log line with the effective TTL and interval (spec `inventory-reservations`) and the `processed_events` purge after each sweep (no new scenario: it stays covered by the existing `TestSweepOncePurgesProcessedEventsOlderThan14Days` and `TestSweepOnceWithoutPurgeCapabilityStillSweeps`, moved to `internal/listing`, run by task 1.6.3).

## D5 Kafka: producer, outbox, consumer

Producer: `kafkax.NewProducer(s.Kafka)`; it stamps `x-request-id` and `traceparent` and fails fast on bad config.

Outbox: `outbox.NewStore(pool, "outbox_events")`, `EnqueueTx` replaces `OutboxStore.Enqueue*`; `outbox.NewRelayer(store, producer, RelayerOptions{Topic: s.Events.ListingTopic, Config: s.Outbox, Logger})` publishes keyed by `AggregateID` (= listing id, same as today). Behaviour change (accepted): no attempt limit; transient errors retry with capped backoff and log at ERROR after `AlertAfter`; only record-level errors (`MessageTooLarge`, `outbox.Permanent`, ...) park a row, and a parked row holds back only its own aggregate. `OUTBOX_MAX_ATTEMPTS` is deleted. `Row.Payload` is the same marshalled `EventEnvelope`, so the wire bytes are unchanged; `EventID` stays the idempotency key (so a retried business write enqueues once).

Purge: a lifecycle component calls `outbox.Store.Purge` for `published` rows older than 7 days on a schedule (`lifecycle.Every`, interval with a test seam in `export_test.go`; no environment variable). `pending` and `failed` rows are never purged.

Consumer: `kafkax.NewConsumerClient(s.Kafka, s.Events.IdentityGroup, []string{s.Events.IdentityTopic})`, `consume.NewConsumer(client, handler, producer, consume.Options{DLQTopic: topic + ".dlq", KeyFP: fp, Logger})` with `fp` from `kafkax.NewFingerprinter(s.LogKeyFP.Secret)`. The handler keeps its body; `ErrPermanent` becomes `consume.Permanent`. Dedupe stays in the store transaction (`processed_events` plus `seller_status_applied` are written in the same transaction as the listing changes, which a generic dedupe helper cannot do), so `consume`'s own dedupe helper is not used.

`key_fp` is now HMAC-SHA256 keyed by `LOG_KEY_FP_SECRET` (first 12 hex), so it changes value versus today's unkeyed SHA-256 (accepted); the empty-key constant `e3b0c44298fc` is gone.

## D6 Migrations and the outbox v1 shape

The SDK relayer reads `topic`, `traceparent`, `seq` and per-aggregate order from the table. Migrations (next free numbers after 0012): `0013_outbox_v1_columns` (UpgradeSQL Prepare), `0014_outbox_v1_backfill` (Backfill loop in a DO block, numbering by `(created_at, event_id)`), `0015_outbox_v1_switch` (Switch: lock, number leftovers, default from the sequence), `0016_outbox_v1_finish` (Finish with plain `CREATE INDEX IF NOT EXISTS`, since golang-migrate wraps a file in one transaction and `CONCURRENTLY` cannot run there; the index build is brief on a purged table; then `UPDATE outbox_events SET status='pending', attempts=0, last_error=NULL WHERE status='failed'`, so rows parked by the old attempt limit are retried by the v1 relayer, human round 2). Down files restore the old shape (drop added columns and indexes, recreate `outbox_events_claim_idx`); the requeue is not reversed. The `principal` column stays, unwritten.

Migrations run only through the subcommand (human, round 2): every service binary has `migrate up`, the service never migrates at boot, compose keeps a `team-domain-migrate` service on the service image with `migrate up`, and gitops gets an Argo PreSync migrate Job (deploy-go-sdk-v1). Rollout order (documented in README and tasks): the first rollout scales the old pods to 0 before any new pod starts (Recreate once, never two relayers; the 0016 migration already requeues the old `failed` rows, and published rows are purged after 7 days by the service), then PreSync `migrate up`, then the new pods. Later rollouts use the normal strategy. Local compose has no old pods, so it applies directly.

`server migrate up` loads a small settings type (`Runtime`, `Postgres`) with `config.Load`; it needs `ENV` and `DATABASE_URL` only and does not read `GRPC_PORT`, so the migrate job sets only those two (a surplus `GRPC_PORT` is ignored; `.env.example` and the overlays describe the full server settings, not the job), embeds `migrations/*.sql` (`migrations/embed.go`, package `migrations`) and calls `migrate.Up`. The compose service `team-domain-migrate`, the Argo PreSync Job and the shared `go-service.mk` `migrate` target all run `server migrate up`; the stock `migrate/migrate` image block is deleted in the deploy track.

## D7 Errors and auth

`rpc.errorTable` (`grpcx.NewErrorTable`, ordered): `repository.ErrNotFound` (now `store.ErrNotFound`) to NotFound "not_found", `ErrVariantNotFound`, `ErrReservationNotFound`, `ErrCategoryNotFound`, `ErrBundleNotFound`, `ErrStorefrontNotFound` to NotFound with today's messages, `ErrSlugTaken` to AlreadyExists "slug already taken", `ErrStatusLocked` and `ErrStatusChanged` to FailedPrecondition "listing status does not allow this change", `ErrReservationReleased` and `ErrListingNotReservable` to FailedPrecondition, `ErrForbidden` to PermissionDenied "not the listing owner", `ErrInvalidCursor` to InvalidArgument, `ErrTransitionNotAllowed` to FailedPrecondition. The 23505 rule is the SDK default, so a duplicate-key race that reaches the driver becomes AlreadyExists instead of Internal. `grpcx.Status(ctx, logger, err, table)` is the single exit of every handler; `internalErr` is deleted. Messages that today carry domain wording stay in `Message`; handler-local `status.Error(InvalidArgument, ...)` validation messages are untouched.

Auth: `auth.UnaryServerInterceptor()` resolves the same `x-principal-*` keys; the handler helpers call `auth.RequireScopes(ctx, "listing.read")`, `auth.RequireService(ctx, "listing.write")`. Accepted text changes: `Unauthenticated` "authentication required", `PermissionDenied` "missing scope" (was `insufficient_scope: missing "x"`), "principal type not allowed" (was "service principal required"). Codes are unchanged.

## D8 Upstreams

team-ai: `upstream.NewClient(addr, upstream.AsService("service-team-domain", "ai.classify"), upstream.WithTimeout(timeout))`; the caller principal is replaced, the inbound deadline minus 500 ms still caps the call (`upstream.Bounded` is the SDK form of today's `boundedCtx`). `screen.AIClient.ScreenListing` keeps the response mapping.

team-audit: `auditx.Dial(s.Audit, "service-team-domain", "audit.write")`, `audit.Writer` maps `auditx.Event` to `WriteAuditEventRequest{ActorId, Action, TargetType:"listing", TargetId, Metadata}` (actions `listing.admin_update`, `listing.admin_delete`, `listing.moderate` and their metadata keys are unchanged), and `auditx.NewGate(writer, logger).Do` wraps the three privileged paths. The three `WriteListing*` methods and `ErrAuditUnavailable` are replaced by the gate; handlers map the gate's `Unavailable` straight through (`grpcx.Status` returns an existing gRPC status unchanged), so the message is "audit trail unavailable".

## D9 Specs outside this change and the VERIFIED BY path map

Existing specs (`listing-moderation`, `listing-status-ownership`, `inventory-reservations`, `listing-read-visibility`, `shop-display-name`, `item-attributes`, ...) name tests as `team-domain internal/<old pkg> TestName`. Test names do not change except the renames and deletions listed below; the package prefix does. The archive step rewrites the prefixes mechanically: `internal/handler` to `internal/rpc`, `internal/service` to `internal/listing`, `internal/repository` to `internal/store`, `internal/consumer` to `internal/events`, `internal/upstream` to `internal/screen` (AI client tests) or `internal/audit` (audit client tests), `internal/bootstrap` to `internal/app`, `internal/grpcserver` to `internal/rpc` (see the exceptions below). The spec-sync check only requires a VERIFIED BY line to exist.

Renames and deletions of cited tests (apply with the archive prefix rewrite):
- `TestNewAuditClientEmptyAddrIsNil` (cited by "Listing audit: No audit upstream configured") is deleted with `upstream/audit.go`; this change MODIFIES the scenario (same title) to cite `internal/config` `TestUpstreamAddressesRequiredInEveryEnv`; the same applies to "Moderation: No audit upstream configured" and "Auto-screen: Unconfigured team-ai fails safe"; no archive rewrite is needed for them.
- `TestAuditClient*` (in the modified requirement "team-domain presents a service principal to team-audit") become `TestAuditWriter*` and `TestAuditGateTimeoutSingleAttempt`.
- `internal/grpcserver/server_test.go` (in-process gRPC tests, including `TestReserveStock_HeldListingFailedPrecondition` and `TestReserveStock_HeldListingExistingReservationNoop` cited by `seller-enforcement-hold`) moves to `internal/rpc` with the same test names; the archive step rewrites `internal/grpcserver` to `internal/rpc` for those, not to `internal/app` (the earlier mapping applies only to composition-root tests). `TestUpdateWithEventAdminNilAuditUnavailable` is deleted (no nil audit writer exists).
- `internal/grpcserver/processed_events_purge_test.go` does not exist; the purge tests live in `internal/service` and move to `internal/listing`.

## D10 Rejected alternatives

- Keep `internal/config/envcheck.go`: the SDK already ships the drift check, a copy would diverge.
- Keep the stock `migrate/migrate` job and leave `migrate` unused: contradicts "use the SDK for everything it covers" and leaves two migration tools.
- Use `consume`'s dedupe helper: cannot share the transaction with the listing changes.
- Use SDK offset pagination: it would change the opaque cursors that the frontend and search already hold; only `pagination.Clamp` is used.
- Keep a local nil-screener or nil-audit mode: rejected by the human decision (round 2), upstreams are required in every ENV.

## Clarifications

### Round 1 — 2026-10-23
- Target `github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2`: require the tag, no replace, no third_party; reference `../platform-core/make/go-service.mk` and the SDK `.golangci.yml`, no copies; revive doc-comment rules are off so no doc comments are added; golangci-lint pinned as a go tool like the SDK go.mod; ADR-0016 layout. (human, recipe)
- Delete every dependency enable flag (`DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `FEATURE_FLAGS_ENABLED`, `*_CONSUMER_ENABLED`, `SESSION_SWEEP_ENABLED`) and every runtime in-memory or noop fallback (`NewInMemory*` in main/bootstrap, `NoopPublisher`, `RequireDurableStorage`); in-memory implementations stay only as test fakes. Kept flags: `OTEL_ENABLED`, `GRPC_REFLECTION_ENABLED` (off by default, refused in a strict ENV), `SEED_ADMIN_ENABLED` (identity). (human, recipe)
- Mock is mock data served by the real service, never a mock service: existing mock switches move onto `pkg/mockdata` (`MOCK_DATA=<adapters>`, refused in a strict ENV before validation). team-domain has none. (human, recipe)
- Use the SDK for everything it covers: config, logx, otelx, healthx (readiness overall, liveness independent of dependencies, `probes.livenessService: liveness`), lifecycle, grpcx (error table including 23505 to AlreadyExists, Canceled and DeadlineExceeded codes), auth, postgres.Open (no OpenOrLocal), migrate, kafkax and consume (key_fp HMAC with `LOG_KEY_FP_SECRET`, DLQ), outbox, upstream (service principal), auditx (Writer interface, fail closed "audit trail unavailable"), pagination (keep existing cursor formats), resilience and jwtx where relevant. The behaviour changes that follow are accepted. (human, recipe)
- Keep every current behavioural requirement and every Q1-Q11, C4 and privacy-hardening behaviour green; main's behaviour wins over the old migration; MODIFY only requirements that describe removed flags, fallbacks or env vars; ADD capability requirements where no spec describes the service. (human, recipe)
- No DPO sign-off needed: basis legitimate interest, platform log retention. (human, recipe)
- e2e is not run per service now (once at the end); scenarios may still name e2e features in VERIFIED BY. Compose and gitops edits are not code tasks; they are listed in tasks.md "Deploy track (done by change deploy-go-sdk-v1)". (human, recipe)

### Round 2 — 2026-10-23
- Migrations: every service has a `migrate up` subcommand in its binary (SDK `pkg/migrate`, embedded migrations); the service never migrates at boot; compose keeps a `<svc>-migrate` service running the service image with `migrate up`; gitops gets an Argo PreSync migrate Job per service (deploy-go-sdk-v1). (human)
- Kafka is not a readiness check in any service: readiness is Postgres only for team-domain (it has no required upstream health check); the consumer and relayer state is visible through logs and the outbox. (human)
- The shared deploy-runtime requirement "Services refuse unsafe configuration in staging and production" (and any other shared deploy-runtime requirement) is MODIFIED only by change deploy-go-sdk-v1; this change has no such copy, and the team-domain bullet is listed under the Deploy track. (human)
- Outbox upgrade: the migration requeues rows parked as `failed`; the first rollout scales old pods to 0 before new pods start (Recreate once, never two relayers); published rows are purged after 7 days by the SDK `Store.Purge` on a schedule in the service. (human)
- Raw gitops manifests (search-indexer, analytics, ga4-relay) get strict ENV overlays in deploy-go-sdk-v1 (not team-domain); `LOG_KEY_FP_SECRET` is provisioned in Vault by the user (human task). (human)
- Upstream addresses and clients the service uses are required in every ENV with no fallback modes: team-domain `UPSTREAM_AI_ADDR` and `UPSTREAM_AUDIT_ADDR`; the fallback scenarios live in `openspec/specs` (`listing-status-ownership`, `listing-admin-audit`, `listing-moderation`) and are rewritten here to assert the config refusal under their existing titles. (human)
- Not applicable to team-domain: analytics `ga_client_id` DLQ redaction, search toggles and tombstone variables, gateway renames, read circuit breaker and revocation-consumer readiness, analytics GA4 variables, engagement page cap, order and payment accepted changes. Item 12's `UPSTREAM_CALL_TIMEOUT*` removal in favour of the SDK 2 s bound is already part of this change. (human)
- Local compose sets `GRPC_REFLECTION_ENABLED=true` for every service (Deploy track line). (human)
- Docker-backed tests must run: a skip is a gate failure (`GO_SDK_REQUIRE_DOCKER=1`). (human)
- The agora repo is public: the SDK tag needs no `GOPRIVATE` or credentials. (human)

### Decisions
- D11: `UPSTREAM_AI_ADDR` and `UPSTREAM_AUDIT_ADDR` are refused when empty in every ENV (supersedes the earlier strict-only draft, confirmed by the human in round 2). Compose already sets both for team-domain; gitops baseline sets both.

### Author assumptions (not yet confirmed)
- `flags`, `resilience` and `jwtx` are not used: team-domain has no feature flag, no HTTP edge and no token verification, so there is nothing for them to cover. (agent)
