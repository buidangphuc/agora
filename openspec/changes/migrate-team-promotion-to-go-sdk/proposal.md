## Why

team-promotion is part of wave C of the go-sdk migration (ADR-0016, go-sdk v0.2.1). It carries its own copies of the platform layer and of the messaging code: Kafka bootstrap, publisher, relayer and/or consumer. These copies diverged from each other, as the survey of 2026-10-12 found. It has no panic recovery, and its config and enable flags differ from the other services.

initiative: go-platform-sdk

## What Changes

- **SDK and layout:** vendor `go-sdk/v0.2.1` and move to the ADR-0016 layout, following `team-order/` (the messaging pilot) and `team-domain/`:
  - `internal/app` is the composition root;
  - `internal/config` is built on SDK `config`;
  - `internal/rpc`, `internal/promotion`, `internal/store` and `internal/events` hold the transport, domain, storage and event code;
  - the platform copies are removed.
- **Flags removed:** `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED` and `GRPC_REFLECTION_ENABLED`, wherever every deploy runs the capability. They also go from the compose block and from platform-gitops.
- **Database:** an unreachable database fails boot in deployed environments, following each service's current rule.
- **Workers:** they stop after the server (`lifecycle.StopAfter`).
- Producer: `internal/producer` moves onto SDK `kafkax` (keeping its `traceparent` behaviour, which kafkax now provides). Flags: SDK `flags`; `FEATURE_FLAGS_ENABLED` stays (gitops runs it false).
- **Tooling:** `Makefile` from `make/go-service.mk`; `Dockerfile` from the template (`GRPC_PORT=50061`).

No RPC, proto or event payload changes.

## Non-goals

- Business behaviour changes.
- Topic or DLQ renames.
- Other services.

## Impact

- **Repo:** `team-promotion`, its compose block, and `platform-gitops/{argocd/apps,envs/services}/team-promotion.yaml`.
- **Capability:** `promotion-runtime` (new).

## Non-functional requirements

- No regression in the existing tests.
- Events are never lost:
  - consumers commit only after handling or a successful DLQ produce;
  - relayers park poison rows.

## Data classification

Unchanged from today's team-promotion. No new data is collected.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| Event stream | Loss through a commit past an unhandled record | SDK consume semantics |
| Errors | Information disclosure | `grpcx.Status` |
