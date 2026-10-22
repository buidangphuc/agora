# go-sdk migration — plan (agreed 2026-10-22)

## Why
PR #18 (2026-10-13) migrated all 15 Go services to go-sdk v1, but it merged into `feat/go-sdk-v1-e2e`, not main.
PR #40 landed only `platform-core/packages/go-sdk` on main. On main today 0/15 services import the SDK or have
`third_party/go-sdk`; each still hand-rolls config/env check, logging, tracing, health, lifecycle, outbox (6 services),
Kafka consumers (~10), idempotency / `processed_events` (11).

A re-merge of the feat branch is not an option: the trial merge had 209 conflicted files (76 real code), because main
changed the same files for Q1–Q10 and privacy-audit-hardening. Each service is ported again on current main; the feat
branch (`origin/feat/go-sdk-v1-e2e`, merge-base `0afde5b6`) is a reference only.

## Decisions (user, 2026-10-22)
1. Services depend on the SDK as a **tagged Go module**, not vendored copies. The module path changes from
   `github.com/buidangphuc/platform-core/packages/go-sdk` (no such GitHub repo, so it cannot be fetched) to
   `github.com/buidangphuc/agora/platform-core/packages/go-sdk`. The agora repo is public: no token in docker builds.
2. **Low-conflict services first**, in parallel with other work; high-churn services after C4 (q6 disputes) and q11
   reach main.
3. One OpenSpec change and one PR per service (or a small group of tiny services).
4. SDK goes to **v1.0.1** first (Step 0), absorbing the helpers main grew after the migration; services adopt v1.0.1.

## Steps
### Step 0 — SDK v1.0.1 (branch `feat/go-sdk-module-path`)
One OpenSpec change on `go-platform-sdk` (e.g. `go-sdk-v1-0-1`), one PR, then **the user** cuts tag
`platform-core/packages/go-sdk/v1.0.1` on main. It closes the gaps between SDK v1.0.0 and what main's services
hand-roll since 2026-10-13 (found 2026-10-22 by grepping origin/main), so no service has to keep its own copy:

| # | gap | services hand-rolling it on main | SDK v1.0.1 change |
|---|---|---|---|
| G1 | module path `github.com/buidangphuc/platform-core/...` cannot be fetched (no such repo); spec requires vendoring via `sync-go-sdk.sh` | all | rename to `github.com/buidangphuc/agora/platform-core/packages/go-sdk` (go.mod + 52 files); tagged module, no `third_party`, no `replace`; remove/rewrite `sync-go-sdk.sh` + test, README, `go-service.mk`/`.golangci.yml` notes, FEATURES lines ~804/810/825/4349/4560/4691/4700/5081; ADR superseding ADR-0016's distribution part |
| G2 | dependency on/off flags (user, 2026-10-22: "if a service imports the database, it uses it"). SDK v1 already has none for Postgres and Kafka; services still carry them, all set `=true` in compose and gitops | DATABASE_ENABLED: domain, engagement, identity, order, payment, promotion, search; KAFKA_ENABLED: 11 services; OUTBOX_ENABLED: chat, domain, identity, order; FEATURE_FLAGS_ENABLED: order, promotion; PAYMENT_SETTLEMENT_CONSUMER_ENABLED: payment; IDENTITY_CONSUMER_ENABLED: domain; SESSION_SWEEP_ENABLED: identity (user 2026-10-22: always sweep) | SDK adds **no** enable flag (keep "always on"; spec + ADR say so explicitly). Each service migration deletes these flags, their runtime in-memory / noop fallbacks (`RequireDurableStorage`, `NewInMemory*` in main/bootstrap, noop publishers) and the env lines in `.env.example`, compose and `platform-gitops/envs/services/*.yaml`. In-memory repos stay only as test fakes. Specs that describe the flag or the fallback are MODIFIED in the same change (found 2026-10-22: deploy-runtime, go-platform-sdk, listing-write, tracking, the order/search durable-storage requirements, `ops/boot_guard_search.feature` + `visibility_boot_guard_steps.py`) |
| G3 | real mock-data modes (the service serves mock data: `MOCK_PAYMENTS`, team-ai `CHAT_BACKEND=mock`) must be refused in a strict ENV, and the guard must run **before** DB validation (bug fixed in #48). The ungated in-memory "mock mode" fallbacks of audit, referral, sharing, verification, notification disappear with G2 | payment, gateway, order, domain, search | `config` helper `RequireOffInStrict(name, on)` run by `Load` before group validation; services declare their mock-data flags through it |
| G4 | audit-first client: call team-audit before a privileged/money action, fail closed with `Unavailable` "audit trail unavailable", `UPSTREAM_AUDIT_ADDR` | domain, engagement, identity, order, payment, promotion, verification (gateway forwards) | new package (e.g. `pkg/auditx`): client, fail-closed wrapper, settings group |
| G5 | Kafka key / user id redaction in logs (`key_fp`, privacy-audit-hardening); consumer `err` text may embed user ids | domain, engagement, identity | `consume`/`kafkax` log a key fingerprint by default (decide HMAC vs plain hash: register item 7 says the 48-bit unsalted fp is weak) and never the raw key or payload |
| G6 | a unique-key violation surfaces as `Internal` 500 (duplicate `CreateVoucher` returns 500 instead of `AlreadyExists`) | promotion (likely others) | `postgres`/`grpcx` error table maps SQLSTATE 23505 to `AlreadyExists` |

Checks: `go build`, `go vet`, `go test -short ./...` (testcontainers suites), spec_lint/validate, `ci_local.sh`.
Each gap is one requirement with scenarios and `VERIFIED BY` unit tests.

### Step 1 — low-conflict services (start after the tag exists)
Start after tag v1.0.1 exists. Commits on main since the migration (2026-10-22): sharing 2, referral 2, chat 4, verification 9, search 9, audit 14,
notification 14.
Order: sharing, referral → chat, verification → search, audit, notification.

### Step 2 — high-churn services (after C4 + q11 are on main)
analytics 25, promotion 25, payment 37, engagement 49, domain 54, gateway 60, order 71, identity 72.
These carry Q1–Q10 behaviour in exactly the files the SDK replaces (outbox, consumers, idempotency, config,
session/refresh, moderation, disputes, order lifecycle): port behaviour-preserving, one service per PR.

## Per-service recipe
1. Worktree from origin/main; `go get github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.1`.
2. Adopt the ADR-0016 layout and SDK packages the feat branch used for that service
   (`git grep -h -o "packages/go-sdk/pkg/[a-z]*" origin/feat/go-sdk-v1-e2e -- "team-<svc>/*.go"`).
3. Replace hand-rolled code with the SDK package; keep every current spec scenario green (main's behaviour wins over
   the feat branch's version of the same file).
4. Checks: `go vet ./...`, `go test ./... -count=1` (Postgres tests with a throwaway DB, skips are not passes),
   `.env.example` drift test, docker image build, the service's e2e files on the mainx stack, `ci_local.sh`.
5. PR; the user merges.

## Gap table (2026-10-22)
| service | commits on main since 2026-10-13 | SDK packages used on the feat branch |
|---|---|---|
| sharing | 2 | auth config grpcx healthx lifecycle logx migrate otelx postgres testkit |
| referral | 2 | + pagination |
| chat | 4 | + kafkax outbox reqctx |
| verification | 9 | auth config grpcx healthx lifecycle logx migrate otelx postgres testkit |
| search | 9 | + consume kafkax upstream |
| audit | 14 | + pagination |
| notification | 14 | + consume kafkax reqctx upstream |
| analytics | 25 | consume kafkax (no postgres) |
| promotion | 25 | + flags kafkax reqctx upstream |
| payment | 37 | + outbox consume upstream pagination |
| engagement | 49 | + outbox consume upstream reqctx |
| domain | 54 | + outbox kafkax reqctx |
| gateway | 60 | idempotency jwtx resilience kafkax reqctx |
| order | 71 | all 18 packages incl. flags idempotency resilience |
| identity | 72 | + jwtx outbox |

## Kept flags (not dependency toggles)
- `OTEL_ENABLED`: exporter on/off (SDK spec "telemetry is a no-op unless enabled"); local stacks often have no collector.
- `GRPC_REFLECTION_ENABLED`, `SERVER_REFLECTION_ENABLED`, `EDGE_REFLECTION_ENABLED`: debug surface, must stay off in production.
- `SEED_ADMIN_ENABLED` (identity): seeds an admin account; behaviour, not a dependency.

## Open questions
- Keep `make/go-service.mk` / `.golangci.yml` copies per service, or reference them?
- G5: HMAC with a per-env secret (needs Vault wiring) or keep a plain hash?
- G3: should `Load` refuse every unknown `*_MOCK`/`MOCK_*` flag in strict ENV, or only declared ones?
