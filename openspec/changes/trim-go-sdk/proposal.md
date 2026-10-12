## Why

The SDK grew helpers that only one or two services need, or none. Every SDK tag then pulls every service into a re-sync, which is the cost the team wants to avoid (v0.2.3 and its 15-service re-sync were withdrawn on 2026-10-13).

Usage across the 16 migrated services:

- `resilience`: imported by no service.
- `flags`: imported by team-order and team-promotion only. Before the migration both had their own `internal/featureflags`.
- `auth.RequireScopesAllowAnonymous` (v0.2.2): vendored by no service. team-domain, team-search and team-analytics keep their local `requireScopes` (a few lines each).
- `flags.NewWithProvider` (v0.2.1): used by no service.

initiative: go-platform-sdk

## What Changes

- **BREAKING** Remove `pkg/flags` and the `config.Flags` group. Feature flags are service code again (team-order, team-promotion own `internal/featureflags`).
- **BREAKING** Remove `pkg/resilience`. A service that needs retry or breaking uses failsafe-go directly.
- **BREAKING** Remove `auth.RequireScopesAllowAnonymous`.
- `go mod tidy` drops `open-feature/*` and `failsafe-go` from the SDK module.
- ADR-0016 gains an admission rule (what may enter the SDK) and a bump rule (a service moves to a newer tag only when it uses something the tag adds; an SDK release never re-syncs services). The phase-4 "connection SDK" list (redisx, ratelimit, quota, mongox, blobx, queue, tasks, webhooks) is dropped: each lands only when the admission rule holds.
- Released as `go-sdk/v0.3.0`, local tag.

## Non-goals

Re-syncing any service to v0.3.0. Services stay on the tag they vendor. team-order and team-promotion stop importing `pkg/flags` in their own migration changes while staying on v0.2.1.

## Impact

`platform-core/packages/go-sdk` (`pkg/flags`, `pkg/resilience`, `pkg/auth`, `pkg/config`, `go.mod`, README), `platform-core/docs/ADR/0016-go-service-convention-and-sdk.md`, `platform-core/README.md`, `platform-core/FEATURES.yaml`.

## Non-functional requirements

None.

## Data classification

internal; unchanged.
