## Why

team-domain (listing write-model: listings, categories, storefronts, bundles, stock reservations, moderation, seller enforcement hold) still hand-rolls config, logging, tracing, health, shutdown, the outbox relayer, the Kafka consumer, the audit and team-ai clients and error mapping. Plan `plans/go-sdk-migration/PLAN.md` (agreed 2026-10-22) ports every Go service onto `go-sdk` v1.0.2 as a tagged module, one change per service, after C4 (q6 disputes) and q11 reached the integration branch. This change is the team-domain port, behaviour-preserving, on top of main.

Decisions already taken by the user (2026-10-22/23) that this change applies to team-domain:
- Depend on the tag `github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2`: no `replace`, no `third_party`, shared `make/go-service.mk` and SDK `.golangci.yml` referenced, not copied.
- Importing a dependency means using it: `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED` and `IDENTITY_CONSUMER_ENABLED` are deleted together with `events.NoopPublisher`, the `database disabled` branch in `main.go` and the in-memory repositories in runtime code.
- Behaviour changes that follow from using the SDK are accepted (HMAC `key_fp`, fixed 2 s audit bound, outbox without an attempt limit, `SHUTDOWN_GRACE` as a duration, SDK principal error texts).

initiative: go-sdk-migration

## What Changes

- **ADDED** capability `domain-go-sdk`: tagged SDK dependency and ADR-0016 layout; settings on SDK groups with `ENV` required and strict-ENV refusals; no dependency flags and no runtime fallbacks; readiness (Postgres only, Kafka is not a readiness check) and liveness split; every service binary has a `migrate up` subcommand and never migrates at boot; upstream addresses required in every `ENV`; ordered start and stop; SDK server chain, principal checks and error table (23505 to `AlreadyExists`, `Canceled`, `DeadlineExceeded`); `pagination.Clamp` with unchanged keyset cursors; `server migrate up` with the outbox moved to the SDK v1 shape (migrations 0013-0016); SDK upstream clients with `auditx` fail-closed audit.
- **REMOVED + ADDED** `listing-write`: "Relayer failure handling and back-compat gating" (Kafka-off gating, attempt-limit parking) becomes "The outbox relayer retries transient failures and parks permanent ones" (SDK relayer: no attempt limit, per-aggregate order).
- **REMOVED + ADDED (renamed)** `seller-enforcement-hold` "Event handling is idempotent and fail-safe" becomes "Event handling is idempotent, fail-safe and always on": the scenario "Consumer disabled does nothing" is replaced by "Consumer is always running"; the other scenarios keep their titles, and their VERIFIED BY package paths move to the new packages (`internal/consumer` to `internal/events`, `internal/repository` to `internal/store`).
- **MODIFIED** `domain-consumer-logging` "Consumer logs carry a key fingerprint, not the record key": `key_fp` is the first 12 hex characters of HMAC-SHA256 keyed by `LOG_KEY_FP_SECRET`, not an unkeyed SHA-256.
- **MODIFIED** `listing-admin-audit` "team-domain presents a service principal to team-audit": the call bound is the SDK's fixed 2 s (the variable `UPSTREAM_CALL_TIMEOUT_SECONDS` is deleted) and `UPSTREAM_AUDIT_ADDR` is required in every `ENV` (no nil writer, no unset-audit mode). Same requirement file: "Admin-scope update of a listing is audited first" and "Non-admin owner path is unaudited and independent of team-audit" lose the unset-address wording; the scenarios "No audit upstream configured" keep their titles (openspec archive cannot drop a scenario from a MODIFIED block) and now assert the config refusal.
- **MODIFIED** `listing-moderation` "Admin moderates a listing" and `listing-status-ownership` "Every content write is auto-screened before publish": the unset-upstream fallbacks are gone (`UPSTREAM_AUDIT_ADDR` and `UPSTREAM_AI_ADDR` required in every `ENV`); the scenarios "Moderation: No audit upstream configured" and "Auto-screen: Unconfigured team-ai fails safe" keep their titles and assert the config refusal; the other scenarios are copied unchanged.
- **ADDED** `listing-write` "Published outbox rows are purged after 7 days" (SDK `Store.Purge` on a schedule in the service).
- Shared deploy-runtime requirements (for example "Services refuse unsafe configuration in staging and production") are MODIFIED only by change `deploy-go-sdk-v1`; this change carries no copy of them. The service bullet for it is listed under the Deploy track.
- Env surface of team-domain: deleted `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `IDENTITY_CONSUMER_ENABLED`, `OUTBOX_MAX_ATTEMPTS`, `UPSTREAM_CALL_TIMEOUT_SECONDS`; renamed `SHUTDOWN_GRACE_SECONDS` to `SHUTDOWN_GRACE` and `OUTBOX_CLAIM_LOCK_SECONDS` to `OUTBOX_CLAIM_LOCK` (Go durations); added `LOG_KEY_FP_SECRET`, `SHUTDOWN_DRAIN_DELAY`; `UPSTREAM_AI_ADDR` and `UPSTREAM_AUDIT_ADDR` become required in every `ENV` (local compose already sets both); `GRPC_REFLECTION_ENABLED` stays but defaults to false and is refused in a strict ENV; `ENV` has no default; `GRPC_PORT` has no default and is required (50051 in every deployment, so the gitops baseline and the inline argocd block must set `env.GRPC_PORT`); `OTEL_SERVICE_NAME` has no default in config and `app.Run` passes `team-domain` to `otelx.Init`, which uses it when the variable is empty (spans and logs keep the name `team-domain`). Kept unchanged: `RESERVATION_TTL`, `RESERVATION_SWEEP_INTERVAL`, `UPSTREAM_AI_TIMEOUT_SECONDS`, `KAFKA_LISTING_TOPIC`, `KAFKA_IDENTITY_TOPIC`, `KAFKA_IDENTITY_GROUP`, `STORAGE_*`, `SCREEN_TERMS_FILE`, `OTEL_*`.
- Specs outside this change that name old package paths in `VERIFIED BY` lines keep their text; design.md D9 lists the mapping the archive step applies.
- Compose and gitops edits are not in the code tasks; tasks.md section "Deploy track (done by change deploy-go-sdk-v1)" lists the exact lines.

## Impact

Code: `team-domain/**` (new `internal/{app,config,rpc,listing,store,events,objects,screen,audit}`; deleted `internal/{bootstrap,grpcserver,interceptor,observability,handler,repository,service,consumer,upstream,storage}`), `team-domain/{go.mod,go.sum,Makefile,Dockerfile,.env.example,README.md,FEATURES.yaml,migrations/0013..0016}`.
Specs: `domain-go-sdk` (new), `listing-write`, `seller-enforcement-hold`, `domain-consumer-logging`, `listing-admin-audit`, `listing-moderation`, `listing-status-ownership`.
Wire contract: unchanged (proto, `listing.events` and `identity.events` envelopes, `ListingChanged`, `ListingStockChanged`, keyset cursors). Database: only the outbox table columns change, additively.

## NFR

- Availability: liveness answers from memory (no I/O) and stays SERVING while Postgres or Kafka is down; readiness (`""`) covers Postgres only and flips to NOT_SERVING within two check intervals (10 s at `healthx.DefaultInterval` 5 s) of a failed check; a Kafka outage does not change readiness (rows stay pending in the outbox). A dependency outage does not restart pods.
- Boot: an unreachable database stops start-up within the 5 s ping bound of `postgres.Open` (scenario threshold 5 s plus 1 s margin), in every `ENV`.
- Shutdown: within `SHUTDOWN_DRAIN_DELAY` + `SHUTDOWN_GRACE` (default 10 s) in-flight RPCs finish, then the relayer, sweeper and consumer stop, then the producer and pool close.
- Latency: no gating latency number is set for reads and writes; the SDK chain adds no I/O per RPC (non-gating note: no baseline was measured, no benchmark task exists, and any regression is judged at the final e2e run). team-ai screening stays bounded by `UPSTREAM_AI_TIMEOUT_SECONDS` (2 s default) and the inbound deadline minus 500 ms; an audit write is bounded by 2 s.
- Outbox: `ListingChanged` publish latency p99 under 2 s at `OUTBOX_POLL_INTERVAL=1s`; delivery stays at least once, ordered per listing id; a parked (permanent-failure) row blocks only its own listing id.
- Limits: page size default 50, maximum 100 (unchanged); `BatchGetStorefronts` maximum 100 ids (unchanged); consumer attempts 5 with 100 ms to 30 s backoff, then `identity.events.dlq`.
- Migration: migrations 0013-0016 add columns and a sequence without a table rewrite; the backfill runs in batches of 10000 rows; the migration is applied only by the `migrate up` subcommand (compose `team-domain-migrate`, Argo PreSync Job), never at service boot; the first rollout follows the order in design.md D6.
- Outbox retention: `published` rows older than 7 days are purged on a schedule by the service.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `seller_id`, `user_id` (listing owner, event keys, audit `actor_id`, `UserStatusChanged.user_id`) | personal identifier (pseudonymous) | owner checks, seller hold, audit trail | legitimate interest (operating the marketplace, fraud and abuse control) | listing rows: life of the listing; `processed_events`: 14 days (existing purge); `seller_status_applied`: one row per seller, unchanged |
| Kafka record key in logs (`key_fp`) | derived, keyed pseudonym | correlate consumer log lines without the raw user id | legitimate interest | platform log retention; the HMAC secret `LOG_KEY_FP_SECRET` lives in Vault (staging/prod), never in the repo, never logged |
| listing title, description, images, moderation note and reason | user-generated content (may contain personal data a seller types) | publish, screen, moderate | contract (seller publishes) and legitimate interest (trust and safety) | life of the listing; moderation note is not placed in events (unchanged) |
| outbox `payload`, `request_id`, `traceparent` | event bytes with the same fields as the topic | transactional publish | legitimate interest | `published` rows purged after 7 days by `Store.Purge` on a schedule in the service; `failed` rows until requeued (the 0016 migration requeues those parked before the upgrade) |
| `x-request-id`, `traceparent` (now stamped on every record and outbound call; continuity verified by scenario "Listing audit: A request id reaches the audit call and the produced record") | operational identifiers | trace continuity | legitimate interest | platform log and trace retention |

No new personal field is collected. No DPO sign-off is needed (user rule): basis legitimate interest, platform log retention.

## Threat model

- Assets: listing and reservation data (stock integrity), seller enforcement state (held listings), moderation decisions and their audit trail, outbox events consumed by search/order, the HMAC secret for `key_fp`.
- Abuse case 1, forged principal: the gateway forwards `x-principal-*`; a pod that can reach the gRPC port could forge an admin. Control unchanged: network policy (`allowFrom`), SDK `auth` interceptor resolves only metadata, handlers call `auth.RequireScopes`, `auth.RequireService` for `ReserveStock`, `ReleaseStock`, `CommitReservation`; a user principal gets `PermissionDenied`.
- Abuse case 2, admin write without audit: an admin update, delete or moderation must not apply when team-audit is down or its address is unset. Control: `auditx.Gate.Do` writes first and returns `Unavailable` "audit trail unavailable" with the cause never in the message; every `ENV` refuses an empty `UPSTREAM_AUDIT_ADDR`.
- Abuse case 3, silent unsafe boot: a mislabelled `ENV` or removed flag letting the service run without Postgres or Kafka (events silently never relayed). Control: no enable flags, `ENV` required, `postgres.Open` fails boot, strict ENV refuses empty `KAFKA_BROKERS`, `LOG_KEY_FP_SECRET`, default object-store keys and reflection, and every `ENV` refuses an empty `UPSTREAM_AI_ADDR` or `UPSTREAM_AUDIT_ADDR` (no unscreened or unaudited mode).
- Abuse case 4, user id leak through logs: consumer logs could expose `user_id` through the Kafka key or error text. Control: only the keyed `key_fp` is logged; handler errors are logged by the SDK loop without the raw key; the HMAC secret comes from env.
- Abuse case 5, debug surface: gRPC reflection discloses the API. Control: default false, refused when `ENV` is staging or prod.
- Abuse case 6, poisoned event stream: a malformed `UserStatusChanged` must not block the consumer or hold listings. Control: permanent errors skip retries and go to `identity.events.dlq`; stale or redelivered events are skipped by `processed_events` and `seller_status_applied` in the same transaction.
- Authz rule: unchanged matrix. `listing.read` for reads, `listing.write` for owner writes (owner or `admin` scope for update and delete), `admin` scope for `ModerateListing` and `ListModerationQueue`, a SERVICE principal with `listing.write` for the reservation RPCs. No principal gives `Unauthenticated`; a missing scope gives `PermissionDenied`.
