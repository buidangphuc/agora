## Why

team-search is one of 15 Go services that hand-roll config, logging, tracing, health, lifecycle, auth, Postgres and Kafka consumption. The go-sdk migration (`plans/go-sdk-migration/PLAN.md`, user decisions of 2026-10-22 and 2026-10-23) ports each service onto the tagged SDK `github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2`. Search is a Step 1 service. On main it carries four things the user ruled out:

- `DATABASE_ENABLED` and `KAFKA_ENABLED`, which let the query server run on an in-memory saved-search store and let the indexer refuse to start, plus `RequireDurableStorage`, the per-ENV guard that exists only because of the in-memory fallback.
- A hand-written Kafka consumer (`internal/consumer/consumer.go`) that logs the raw record key (a listing id) at every retry and park.
- A hand-written tracer, request-id interceptor, principal interceptor and health server, and a `database/sql` Postgres open that is not shared with any other service.
- `GRPC_REFLECTION_ENABLED` defaulting to true, and an optional tag classifier (`UPSTREAM_AI_ADDR` empty means no classification).

Importing a dependency means using it: after this change the server cannot start without Postgres and OpenSearch, the indexer cannot start without Kafka, OpenSearch and team-ai, and no in-memory implementation ships in the binaries.

initiative: go-sdk-migration

## What Changes

- Require go-sdk `v1.0.2` in `team-search/go.mod`: no `replace`, no `third_party`. The Makefile references `../platform-core/make/go-service.mk` and `../platform-core/packages/go-sdk/.golangci.yml`; golangci-lint is pinned as a `tool` line like the SDK `go.mod`.
- Adopt the ADR-0016 layout: `cmd/server/main.go` and `cmd/indexer/main.go` call `internal/app` (`Run`, `RunIndexer`); `internal/config`, `internal/rpc` (the former `handler`, `interceptor`, `grpcserver`), `internal/store` (saved searches on pgx), `internal/events` (the former `consumer`), and the unchanged domain packages `index`, `retrieval`, `taxonomy`, `tombstone`. `bootstrap`, `observability`, `interceptor`, `grpcserver`, `repository` and `config/envcheck.go` are deleted.
- **REMOVED (behaviour)** `DATABASE_ENABLED`, `KAFKA_ENABLED`, `RequireDurableStorage`, the in-memory saved-search repository outside tests, the nil-classifier path, the `saved == nil` "not configured" path. **BREAKING** for any deployment that sets them: the keys are simply no longer read (compose and gitops lines are removed by `deploy-go-sdk-v1`).
- **CHANGED** `ENV` is required (no `local` default); `GRPC_PORT` is required; `SHUTDOWN_GRACE_SECONDS` becomes `SHUTDOWN_GRACE` (Go duration, also declared by the indexer); `DB_MAX_CONNS` is a new server key from the SDK Postgres group; `OTEL_SERVICE_NAME` keeps its per-binary default (`team-search`, `team-search-indexer`) set in code; `OPENSEARCH_URL` keeps its local default and an empty value is still refused; `GRPC_HOST`, `OPENSEARCH_INDEX`, `EMBEDDING_DIM`, `LOG_LEVEL`, `LOG_JSON`, `OTEL_ENABLED` and `OTEL_EXPORTER_OTLP_ENDPOINT` are kept (mapped to the SDK groups or service fields) and covered by `TestEnvExampleInSync`; `GRPC_REFLECTION_ENABLED` defaults to false and is refused unless `ENV` is `local` or `test`; `TOMBSTONE_TTL` and `TOMBSTONE_PURGE_INTERVAL` that cannot be parsed or are not positive refuse boot instead of falling back to the default.
- **CHANGED** the indexer logs `key_fp` (12-hex HMAC-SHA256 under `LOG_KEY_FP_SECRET`, required in a strict ENV) instead of the raw listing id; the DLQ topic, retry count (5) and backoff (100 ms to 30 s) are unchanged.
- **CHANGED** unexpected handler errors return `Internal` with the message `internal error` (previously per-RPC texts such as `search failed`); the gRPC codes are unchanged.
- **ADDED** `/server migrate up` subcommand (SDK `pkg/migrate`, embedded migrations); the server never migrates at boot. Kafka is not a readiness check.
- **ADDED** capability `search-service-runtime` (config, boot, migrate, health, saved-search persistence and auth, indexer consumption, shutdown, module and image). **MODIFIED** `search-read-model-deletes` "Tombstones are retained for a configurable time, then purged" (invalid value). The shared `deploy-runtime` requirement "Services refuse unsafe configuration in staging and production" is modified only by `deploy-go-sdk-v1`; the team-search bullet is listed in `tasks.md` task 3.0.
- Deploy work (compose block and migrate service, gitops baseline, argocd inline values, PreSync migrate Job, strict ENV overlays, indexer manifest) is listed in `tasks.md` under "Deploy track" and done by change `deploy-go-sdk-v1`, not here.

## Impact

Code: everything under `team-search/` except `proto/` and `migrations/*.sql` (a `migrations/embed.go` is added). Specs: `search-service-runtime` (new), `search-read-model-deletes`. No proto change, no new RPC, no new event. E2E: `team-search/FEATURES.yaml` entry `ops.search-boot-guard-prod-storage` and `platform-e2e/tests/e2e/features/ops/boot_guard_search.feature` change (the in-memory leg disappears).

## NFR

- Migrate: `/server migrate up` applies all pending embedded migrations in under 30 s on the current schema and exits non-zero on failure; running it twice is a no-op.
- Boot: an unreachable Postgres makes `postgres.Open` return within its 5 s ping bound, so `Run` returns an error in under 10 s in every ENV; an unreachable OpenSearch makes `EnsureIndex` fail boot the same way.
- Health: readiness (`""`) flips to NOT_SERVING within one `healthx.DefaultInterval` (5 s) of Postgres or OpenSearch failing; the `liveness` service answers from memory (p99 under 5 ms) and stays SERVING while a dependency is down, so a dependency outage never restarts pods.
- Query latency: SearchListings and Suggest p95 SHALL NOT exceed the pre-migration p95 by more than 10 percent on the same query set and index (baseline recorded by task 1.0.1 on main before the code lands, same mainx stack; compared in the single final e2e run, task 2.4); no added network hop; request-id, recovery, logging and auth interceptors run in process; the request path adds no database or network call for `SearchListings` and `Suggest` (they read OpenSearch only), so their latency budget is unchanged.
- Indexer: at-least-once, in order per partition. A record is retried 5 times (100 ms doubling to 30 s), then published to `listing.events.dlq`; if that publish fails the offset does not advance and the process exits non-zero.
- Shutdown: SIGTERM drains health first, then stops the gRPC server within `SHUTDOWN_GRACE` (default 10 s); the indexer stops its consumer and tombstone loop within the same grace.
- Limits unchanged: page size default 10, maximum 50 (saved searches and search), suggest default 5, maximum 20.

## Data classification

| Field | Where | Purpose | Consent basis | Retention |
|---|---|---|---|---|
| principal id (`user_id`) | `saved_searches.user_id`, request logs, traces | owner key of a saved search; support and abuse tracing | legitimate interest (operating the saved-search feature the user asked for) | saved search: until the user deletes it (unchanged); logs: platform log retention (TBD, platform default; see design.md Open issues) |
| search query text and filters (`query`, `filters`) | `saved_searches` only | let the user re-run a search | legitimate interest (user-requested feature) | until the user deletes the saved search (unchanged) |
| listing id (Kafka record key) | indexer logs | correlate a failed record | legitimate interest; not personal data | logged only as `key_fp` (12-hex HMAC under `LOG_KEY_FP_SECRET`), platform log retention (TBD, platform default); raw key never logged; DLQ record keeps the raw key (unchanged) |
| DLQ headers (source partition, offset, consumer group) | `listing.events.dlq` records | let an operator replay a parked record | legitimate interest; contain no personal data | same retention as the DLQ topic; the raw key is unchanged |
| listing text (title, description) | OpenSearch read-model; sent to team-ai and the model server | indexing, classification, embedding | legitimate interest; public listing content | read-model lifecycle (tombstone TTL, unchanged) |

Search query text is never logged by the SDK gRPC logging interceptor (method, code, duration, request id, principal id only). DPO sign-off is not required (user rule 2026-10-23).

## Threat model

- Assets: saved searches (owner-scoped personal state), the OpenSearch read-model, the DLQ topic, `LOG_KEY_FP_SECRET`, the gRPC surface.
- Abuse 1, IDOR on saved searches: a user passes another user's saved-search id. Control: every store call is scoped by `user_id`; a foreign id returns `NotFound`, indistinguishable from a missing one.
- Abuse 2, privilege confusion: an anonymous or service principal calls saved-search RPCs. Control: `auth.RequireUser` runs before the scope gate; anonymous gets `Unauthenticated`, a service gets `PermissionDenied`, a user without `search:read` or `search:write` gets `PermissionDenied`.
- Abuse 3, silent non-durability: a deployment boots with an in-memory store and loses saved searches on restart. Control: no in-memory implementation in the binaries; unreachable Postgres fails boot in every ENV.
- Abuse 4, reflection or mock left on in production. Control: `GRPC_REFLECTION_ENABLED` is refused by `Load` in a strict ENV before validation; the service declares no `MOCK_DATA` adapter.
- Abuse 5, log inference of user behaviour from record keys. Control: `key_fp` is keyed with `LOG_KEY_FP_SECRET`, so a log reader cannot brute-force listing ids without the secret.
- Abuse 6, draft disclosure through search. Control: unchanged visibility rules (`effectiveFilters`), now over `auth.Principal`.
- Abuse 7, `LOG_KEY_FP_SECRET` rotation or leak. Control: the secret lives in Vault only; rotating it changes every `key_fp`, so correlation of one listing id across the rotation is lost by design (rotate on suspected leak, accept the break); a leaked secret lets a log reader brute-force listing ids, which are not personal data.
- Abuse 8, schema race or drift: two replicas migrating at boot. Control: the server never migrates at boot; only the `migrate up` subcommand (one PreSync Job) changes the schema.
- Authz rule: `SearchListings` and `Suggest` follow the existing scope and visibility rules; all four saved-search RPCs require an authenticated user principal plus `search:read` (list, run) or `search:write` (save, delete); the health service and `liveness` carry no data and need no principal.
