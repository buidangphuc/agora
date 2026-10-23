## Why

team-engagement hand-rolls what `platform-core/packages/go-sdk` v1.0.2 ships: settings and the `.env.example` drift gate, slog setup, tracing, health, graceful shutdown, Postgres pool, outbox relayer, Kafka consumer with retry and DLQ, upstream gRPC clients with service principals, the audit client and the principal interceptors (about 7000 lines, 20 packages). It also carries four things the user ruled out on 2026-10-22: dependency enable flags (`DATABASE_ENABLED`, `KAFKA_ENABLED`), runtime fallbacks (a nil order or audit client that quietly turns features off, `Unimplemented` answers when a service is not wired), a single health status that ties liveness to Postgres, and consumer logs that fingerprint user ids with an unkeyed SHA-256 (a 12-hex prefix of a hash of a user id can be reversed by trying known ids).

Source: `plans/go-sdk-migration/PLAN.md` (Step 2, high-churn services, after C4 and q11 reached the branch), `plans/go-sdk-migration/00-spec.md`, and the user's binding recipe of 2026-10-23 recorded in `design.md`.

initiative: go-sdk-migration

## What Changes

- **BREAKING (config)**: `go.mod` requires `github.com/buidangphuc/agora/platform-core/packages/go-sdk v1.0.2` (no `replace`, no `third_party`). `ENV` becomes required and `GRPC_PORT` stays required (the Dockerfile supplies 50054). `DATABASE_ENABLED`, `KAFKA_ENABLED`, `ENGAGEMENT_OUTBOX_RELAY_INTERVAL` and `UPSTREAM_CALL_TIMEOUT_SECONDS` are deleted (every call to team-order and team-audit is bounded by the SDK's fixed 2 s); `SHUTDOWN_GRACE_SECONDS` becomes `SHUTDOWN_GRACE` (duration) and `OUTBOX_POLL_INTERVAL`, `OUTBOX_BATCH_SIZE`, `OUTBOX_CLAIM_LOCK`, `LOG_KEY_FP_SECRET`, `SHUTDOWN_DRAIN_DELAY` appear. `GRPC_REFLECTION_ENABLED` defaults to false and is refused whenever `ENV` is not `local` or `test`. `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR` become required in every `ENV` (no mode without an upstream).
- **Migrations**: the binary gets a `migrate up` subcommand (SDK `pkg/migrate`, embedded migrations, loads only `DATABASE_URL`); `app.Run` never migrates. The image doubles as the migrate image (compose `team-engagement-migrate`, gitops PreSync Job: Deploy track).
- **Layout** (ADR-0016): `cmd/server/main.go` calls `internal/app.Run(ctx, settings)`; packages `config`, `rpc` (was `handler` and `interceptor`), `engagement` (was `service`), `store` (was `repository`), `events` (was `consumer` and the hand-written relayer), `upstream`. `bootstrap`, `grpcserver` and `observability` are deleted.
- **Runtime fallbacks deleted**: a missing order or audit address is refused at `Load` instead of disabling a feature (nil order client, nil audit client); a handler without its service no longer answers `Unimplemented`. In-memory repositories stay in `_test.go` files and `internal/store` test helpers only.
- **Health**: readiness is the overall status (follows Postgres only; Kafka is not a readiness check), liveness is the `liveness` service and does not follow any dependency.
- **Outbox**: facts are written as full `EventEnvelope` rows into new table `engagement_outbox_events` (migration `0014`, copies unpublished rows from the old `outbox` table and requeues rows parked as `failed`) and published by `outbox.Relayer`; published rows are purged after 7 days by `Store.Purge` on an hourly schedule. The first rollout scales the old pods to 0 before the migration and the new pods (one Recreate, never two relayers). The old `outbox` table stays, unwritten, for a later cleanup change. Ordering is per record key (listing, seller, dispute id), not global.
- **Consumer**: `listing.events` runs on `consume.NewConsumer` (5 attempts, DLQ `listing.events.dlq`); logs carry `key_fp`, an HMAC-SHA256 under `LOG_KEY_FP_SECRET`.
- **Audit**: `HideReview`, `ResolveDispute`, `ResolveReport` run through `auditx.Gate`; refusal text is `audit trail unavailable`; the three "audit upstream unset" scenarios become config refusals.
- **Paging and gates (no change in behaviour, now stated)**: the gate of each of the 36 RPCs (5 admin, 14 write, 10 read, 7 public) and the cursor error of each list RPC are specified as they are today. One accepted change: `ListQuestionsByListing` caps the page size at 100 instead of falling back to 20.
- **Errors**: one `grpcx.ErrorTable` replaces per-handler `status.Error` chains for sentinel errors and keeps today's codes and messages; SQLSTATE 23505 maps to `AlreadyExists`, context errors to `Canceled` and `DeadlineExceeded`, unmapped errors to `Internal` without driver text.
- **Specs**: ADDED `engagement-sdk-runtime` (includes the follow feed requirement, which had no main spec); MODIFIED `consumer-key-redaction` (HMAC fingerprint), `review-integrity` (no nil-verifier fallback, 2 s bound), `engagement-facts` (order per record key), `review-moderation`, `dispute-audit`, `report-resolution` (audit address required, no unset mode, 2 s bound).
- **Deploy** (not in code tasks, see `tasks.md` "Deploy track"): compose block and migrate service, gitops baseline, PreSync migrate Job, ArgoCD inline block, staging and prod overlays, `probes.livenessService: liveness`, Vault key (human task). Shared deploy-runtime requirements are MODIFIED only by change deploy-go-sdk-v1.

## Impact

Code: `team-engagement/**`. Specs: `engagement-sdk-runtime` (new), `consumer-key-redaction`, `review-integrity`, `engagement-facts`, `review-moderation`, `dispute-audit`, `report-resolution`. Wire: no proto change; `engagement.events` records keep the `EventEnvelope` shape, event ids and keys. Existing `VERIFIED BY` package paths move with the layout (mapping table in `design.md`); test function names are kept.

## NFR

- Boot: an unreachable database fails `app.Run` within the 5 s ping bound of `postgres.Open`, before the gRPC port opens.
- Liveness `Check("liveness")` answers from memory (p99 under 5 ms) and stays SERVING while Postgres is down; readiness flips to NOT_SERVING within two health intervals after Postgres fails (scenarios `Engagement SDK: Liveness answers fast`, `Database outage flips readiness but not liveness`).
- Fact latency: a committed fact reaches `engagement.events` within 2 s at the default `OUTBOX_POLL_INTERVAL` of 1 s (local compose sets 500 ms), and is published at least once even when Kafka is down at write time (scenario `A committed fact reaches the topic within 2 s`).
- Consumer: a record that fails 5 attempts reaches `listing.events.dlq` and its offset advances; a record that can be neither handled nor parked stops the service and the offset does not advance.
- Audit-first RPCs return `Unavailable` within 3 s when team-audit does not answer (the audit write is bounded by the gate's fixed 2 s); team-order calls are bounded by 2 s.
- `migrate up` on an empty database applies `0001` to `0014` and exits 0 (a second run is a no-op); on an unreachable database it exits non-zero within the SDK's connect bound.
- Shutdown: after SIGTERM the server drains within `SHUTDOWN_GRACE` (default 10 s); background workers stop only after the gRPC server returned.
- Availability: no new dependency; Postgres, team-order and Kafka dependencies are the same as before, now declared without enable flags.
- Limits: page sizes unchanged (favourites, follows, feed, recently viewed, collection items 50/100; reviews, disputes and reports 20/100) except `ListQuestionsByListing`, now 20/100 with a cap instead of a reset; report note 500 runes.

## Data classification

No new personal field is collected, stored or logged. Fields touched by this change:

| Field | Where | Purpose | Basis | Retention |
|---|---|---|---|---|
| user id, seller id (pseudonymous ids) | outbox rows and envelopes, `seller_listings`, consumer logs | route and attribute engagement facts | legitimate interest (operating the marketplace) | outbox: published rows purged after 7 days; logs: platform log retention |
| Kafka record key (a user or seller id) | consumer logs | correlate failures | legitimate interest | logged only as `key_fp` (12 hex of HMAC-SHA256), never raw; platform log retention |
| request id, trace id | outbox rows, headers, logs | tracing | legitimate interest | same as above |
| review text, report reporter id and note, dispute reason and evidence urls | existing tables, unchanged | existing features | unchanged | reports pseudonymised after `REPORT_RETENTION_DAYS` (730) as today |
| `LOG_KEY_FP_SECRET` (secret, not personal) | environment from Vault in staging and prod | key for the fingerprint | n/a | rotated with the Vault entry |

Facts never carry review text (existing rule, kept). Decision (user rule): no DPO sign-off needed; basis legitimate interest, platform log retention.

## Threat model

- Assets: engagement facts (who favourited, followed, reviewed), review and dispute integrity, the admin actions (hide review, resolve dispute, resolve report) and their audit trail, user ids in logs.
- Abuse 1, key reversal: an unkeyed 12-hex SHA-256 of a user id lets anyone with log access confirm a guessed id. Control: HMAC under `LOG_KEY_FP_SECRET`, required in strict `ENV`; consume also scrubs the key from error text.
- Abuse 2, silent downgrade: a mislabelled or half-configured deployment runs without Postgres, Kafka, team-order or team-audit and quietly skips verification or audit. Control: no enable flags and no nil-client fallbacks; `Load` refuses missing `DATABASE_URL`, `UPSTREAM_ORDER_ADDR`, `UPSTREAM_AUDIT_ADDR`, and in strict `ENV` also `KAFKA_BROKERS`, `LOG_KEY_FP_SECRET`.
- Abuse 3, admin action without trail: an audit outage must not let a hide, resolve or report-close through. Control: `auditx.Gate` writes first and returns `Unavailable` `audit trail unavailable` otherwise; the privileged state change never runs.
- Abuse 4, fact loss or duplication on migration: unpublished facts in the old outbox could be dropped at cut-over. Control: migration `0014` copies them with their event ids after the old pods are scaled to 0 (so none is written to the old table after the copy); consumers dedupe on event id.
- Abuse 5, schema detail leak: a unique-key violation or driver error text reaching a client. Control: error table maps 23505 to `AlreadyExists` with a fixed message and every unmapped error to a fixed `Internal` message.
- Abuse 6, reflection on in a deployed environment. Control: default off, refused unless `ENV` is `local` or `test`.
- Abuse 7, a new RPC shipped without a gate: Control: `TestEveryRpcHasItsGate` walks the generated service descriptor against the gate table.
- Authz rule (unchanged): the gateway verifies the caller and forwards `x-principal-*`; this service enforces ownership and `admin` scope per RPC (`HideReview`, `ResolveDispute`, `ListDisputes`, `ListReports`, `ResolveReport` need admin; dispute reads and writes are limited to the order's parties or admin; shop replies to the listing owner). Calls to team-order (`GetOrder`, `GetReturn`) carry the service principal `service-team-engagement` with `order.read`, calls to team-audit the same id with `audit.write` (unchanged from today).
