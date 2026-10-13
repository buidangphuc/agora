## Why

Every SDK tag pulled every service into a re-sync (v0.2.3 and its 15-service re-sync were withdrawn on 2026-10-13). ADR-0016 had no rule for what belongs in the SDK or when a service moves to a new tag.

`auth.RequireScopesAllowAnonymous` (v0.2.2) is vendored by no service. team-domain, team-search and team-analytics keep their local `requireScopes`, a service-specific authorization choice rather than shared infrastructure.

initiative: go-platform-sdk

## What Changes

- **BREAKING** Remove `auth.RequireScopesAllowAnonymous`.
- ADR-0016 states what belongs in the SDK: business-agnostic infrastructure (logging, tracing, config, connections to databases, brokers and queues, shared policies such as rate limiting, feature flags, retry). Business rules and service-specific helpers stay in services; the split need not be perfect.
- ADR-0016 bump rule: a service moves to a newer tag only when it uses something the tag adds; a change that cuts an SDK tag never re-syncs services; every SDK tag is cut only after the owner approves it.
- No version is tagged by this change; the next tag is proposed to the owner first.

## Non-goals

Re-syncing any service. Services stay on the tag they vendor.

## Impact

`platform-core/packages/go-sdk` (`pkg/auth`, README), `platform-core/docs/ADR/0016-go-service-convention-and-sdk.md`, `platform-core/README.md`, `platform-core/FEATURES.yaml`.

## Non-functional requirements

None.

## Data classification

internal; unchanged.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| Public RPCs | Elevation of privilege: a service drops its anonymous scope check when `RequireScopesAllowAnonymous` goes away | No service vendors it; team-domain, team-search and team-analytics keep their local `requireScopes` and its tests |
