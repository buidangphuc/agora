## Why

A review of the 15 migrated services (2026-10-13) found business-agnostic infrastructure that is still re-implemented in several services instead of going through go-sdk: principal-type gates, JWT/JWKS wire types, the Idempotency-Key rule, the pgx open-with-local-fallback, strict-env checks, duration parsing, empty env values, pagination, ticker workers, HTTP servers, batch consumers, group-less consumers, dial options, retry predicates and test fixtures (free ports, polling, recording publishers, shared Postgres, Kafka produce/read). Some services cannot switch to the SDK today because the SDK lacks a seam (`grpcx.Status` cannot keep a fixed client message; `upstream.Dial` cannot set dial options; `consume` is per-record only; `kafkax` always uses a group).

The owner asked for one SDK release that gathers all of this, then for services to adopt it. ADR-0016 says the SDK holds business-agnostic infrastructure.

initiative: go-platform-sdk

## What Changes

- `auth`: principal type constants, `RequireService`, `RequireUser` (`RequireScopesAllowAnonymous` already exists).
- New `jwtx`: JWT claims and JWKS wire types shared by the issuer (team-identity) and the verifier (team-gateway).
- New `idempotency`: the `idempotency-key` metadata key and its validation rule.
- **BREAKING** `grpcx.ErrorTable` maps to `grpcx.Rule{Code, Message}`; an empty Message keeps today's behaviour (`err.Error()`). `grpcx.Code(c)` builds a Rule.
- `pgx.OpenOrLocal`: falls back (nil pool) only when the runtime is local.
- `config`: `Runtime.IsStrict()`, `PositiveDuration`, and `keepempty` for explicitly empty values.
- New `pagination`: `Clamp`, `EncodeOffset`, `DecodeOffset`.
- `lifecycle`: `Every` (ticker worker) and `HTTP` (graceful HTTP server).
- `flags.NewOrDisabled`.
- `consume`: batch consumer and `Dedupe.Processed`; `kafkax`: group-less consumer client; `upstream.WithDialOptions`; `resilience.RetryOptions.RetryIf`.
- `testkit`: `FreePort`, `Eventually`, `Recorder`, `SharedPostgres`/`Schema`, `Produce`/`ReadAll`.
- `sync-go-sdk.sh` keeps the go-sdk `require` line.
- Released as `go-sdk/v1.0.0` after the owner approves the contents.

## Non-goals

- Re-syncing services: each service adopts v1.0.0 in its own change.
- An EventEnvelope helper: it would make the SDK depend on the generated protos.

## Impact

`platform-core/packages/go-sdk` (packages above, `examples/hello`), `platform-core/templates/_service-go`, `platform-core/tools/sync-go-sdk.sh`, `platform-core/FEATURES.yaml`, ADR-0016 package list.

## Non-functional requirements

No new third-party dependency except what franz-go, pgx and grpc already bring.

## Data classification

internal; unchanged.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| Authorization | Elevation of privilege: a service gate admits the wrong principal type | `RequireService`/`RequireUser` check type before scopes; tests cover user, service and anonymous callers |
| Durable data | Tampering/loss: a deployed service silently runs on in-memory storage | `OpenOrLocal` falls back only when `Runtime.IsLocal()`; any other env returns the error |
| Error text | Information disclosure: driver text reaches the client | unmatched errors still become `Internal` with a fixed message; a Rule message replaces the error text |
