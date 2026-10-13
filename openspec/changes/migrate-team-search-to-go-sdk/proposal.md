## Why

team-search is part of wave C of the go-sdk migration (ADR-0016, go-sdk v0.2.1). It carries its own copies of the platform layer and of the messaging code: Kafka bootstrap, publisher, relayer and/or consumer. These copies diverged from each other, as the survey of 2026-10-12 found. It has no panic recovery, and its config and enable flags differ from the other services.

initiative: go-platform-sdk

## What Changes

- **SDK and layout:** vendor `go-sdk/v0.2.1` and move to the ADR-0016 layout, following `team-order/` (the messaging pilot) and `team-domain/`:
  - `internal/app` is the composition root;
  - `internal/config` is built on SDK `config`;
  - `internal/rpc`, `internal/search`, `internal/store` and `internal/events` hold the transport, domain, storage and event code;
  - the platform copies are removed.
- **Consumer:** indexer consumer on listing.events (binary `cmd/indexer`).
- **Flags removed:** `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED` and `GRPC_REFLECTION_ENABLED`, wherever every deploy runs the capability. They also go from the compose block and from platform-gitops.
- **Database:** an unreachable database fails boot in deployed environments, following each service's current rule.
- **Workers:** they stop after the server (`lifecycle.StopAfter`).
- Both binaries (`server` and `indexer`) get their own `internal/app` entry points (`app.RunServer`, `app.RunIndexer`). Keep the DLQ topic name as today (`listing.events.dlq`); renaming it is out of scope.
- **Tooling:** `Makefile` from `make/go-service.mk`; `Dockerfile` from the template (`GRPC_PORT=50052`).

No RPC, proto or event payload changes.

## Non-goals

- Business behaviour changes.
- Topic or DLQ renames.
- Other services.

## Impact

- **Repo:** `team-search`, its compose block, and `platform-gitops/{argocd/apps,envs/services}/team-search.yaml`.
- **Capability:** `search-runtime` (new).

## Non-functional requirements

- No regression in the existing tests.
- Events are never lost:
  - consumers commit only after handling or a successful DLQ produce;
  - relayers park poison rows.

## Data classification

Unchanged from today's team-search. No new data is collected.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| Event stream | Loss through a commit past an unhandled record | SDK consume semantics |
| Errors | Information disclosure | `grpcx.Status` |
