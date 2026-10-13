## Why

team-engagement is part of wave C of the go-sdk migration (ADR-0016, go-sdk v0.2.1). It carries its own copies of the platform layer and of the messaging code: Kafka bootstrap, publisher, relayer and/or consumer. These copies diverged from each other, as the survey of 2026-10-12 found. It has no panic recovery, and its config and enable flags differ from the other services.

initiative: go-platform-sdk

## What Changes

- **SDK and layout:** vendor `go-sdk/v0.2.1` and move to the ADR-0016 layout, following `team-order/` (the messaging pilot) and `team-domain/`:
  - `internal/app` is the composition root;
  - `internal/config` is built on SDK `config`;
  - `internal/rpc`, `internal/engagement`, `internal/store` and `internal/events` hold the transport, domain, storage and event code;
  - the platform copies are removed.
- **Outbox:** SDK `outbox.Store` on `outbox (migrated to the shared column set by a new migration)` plus the SDK `outbox.Relayer` as a lifecycle component (`OUTBOX_*` env via `config.Outbox`).
- **Consumer:** listing.events feed consumer.
- **Flags removed:** `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED` and `GRPC_REFLECTION_ENABLED`, wherever every deploy runs the capability. They also go from the compose block and from platform-gitops.
- **Database:** an unreachable database fails boot in deployed environments, following each service's current rule.
- **Workers:** they stop after the server (`lifecycle.StopAfter`).
- Its outbox (family C: `seq`, no attempts, advisory-lock single relayer, retention delete) is migrated to the shared SDK schema by a new forward-only migration that preserves unpublished rows and their order. Retention of published rows becomes a periodic cleanup component.
- **Tooling:** `Makefile` from `make/go-service.mk`; `Dockerfile` from the template (`GRPC_PORT=50054`).

No RPC, proto or event payload changes.

## Non-goals

- Business behaviour changes.
- Topic or DLQ renames.
- Other services.

## Impact

- **Repo:** `team-engagement`, its compose block, and `platform-gitops/{argocd/apps,envs/services}/team-engagement.yaml`.
- **Capability:** `engagement-runtime` (new).

## Non-functional requirements

- No regression in the existing tests.
- Events are never lost:
  - consumers commit only after handling or a successful DLQ produce;
  - relayers park poison rows.

## Data classification

Unchanged from today's team-engagement. No new data is collected.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| Event stream | Loss through a commit past an unhandled record | SDK consume semantics |
| Errors | Information disclosure | `grpcx.Status` |
