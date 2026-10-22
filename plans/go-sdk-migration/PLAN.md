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

## Steps
### Step 0 — module path PR (branch `feat/go-sdk-module-path`)
- OpenSpec change (e.g. `go-sdk-agora-module-path`) on `go-platform-sdk`:
  - REMOVE/MODIFY "Services vendor a pinned SDK copy" (sync-go-sdk.sh, `third_party/go-sdk`, `replace`).
  - ADD: module path is `github.com/buidangphuc/agora/platform-core/packages/go-sdk`; releases are tags
    `platform-core/packages/go-sdk/vX.Y.Z` on agora main; a service `require`s a tag, no `replace`, no `third_party`.
  - Keep the shared lint/make files: decide whether a small script still copies `make/go-service.mk` and
    `.golangci.yml` into a service, or each service references them.
- Code: rename the module in `platform-core/packages/go-sdk/go.mod` and all 52 files that import it; `go build`,
  `go vet`, `go test -short ./...`.
- Remove or rewrite `platform-core/tools/sync-go-sdk.sh` + `sync-go-sdk_test.sh`; update `packages/go-sdk/README.md`,
  `make/go-service.mk` header, `.golangci.yml` comment, platform-core/FEATURES.yaml entries that cite them
  (lines ~804, 810, 825, 4349, 4560, 4691, 4700, 5081 on 2026-10-22).
- ADR: new ADR superseding the distribution part of ADR-0016 (vendoring → tagged module).
- After merge: **the user** cuts tag `platform-core/packages/go-sdk/v1.0.0` (or v1.0.1) on main. Old `go-sdk/v*` tags
  stay but no service uses them.

### Step 1 — low-conflict services (start after the tag exists)
Commits on main since the migration (2026-10-22): sharing 2, referral 2, chat 4, verification 9, search 9, audit 14,
notification 14.
Order: sharing, referral → chat, verification → search, audit, notification.

### Step 2 — high-churn services (after C4 + q11 are on main)
analytics 25, promotion 25, payment 37, engagement 49, domain 54, gateway 60, order 71, identity 72.
These carry Q1–Q10 behaviour in exactly the files the SDK replaces (outbox, consumers, idempotency, config,
session/refresh, moderation, disputes, order lifecycle): port behaviour-preserving, one service per PR.

## Per-service recipe
1. Worktree from origin/main; `go get github.com/buidangphuc/agora/platform-core/packages/go-sdk@<tag>`.
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

## Open questions
- Keep `make/go-service.mk` / `.golangci.yml` copies per service, or reference them?
- Which tag name: re-use v1.0.0 under the new prefix, or v1.0.1?
