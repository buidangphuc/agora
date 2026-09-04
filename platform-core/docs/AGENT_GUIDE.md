# Agent Guide

Read this before working in any repo of the platform. Read `ARCHITECTURE.md` for the rules.

## What this repo is

`platform-core` — the contract (`packages/proto`), infra (`infra/`), service templates
(`templates/`), and rules (`docs/`). You generate code from the contract; you do not
invent contracts.

## Setup (once)

```bash
brew install bufbuild/buf/buf   # + docker
make hooks-install              # local merge-gate
make dev                        # local infra (redis, qdrant, postgres-*)
```

## Contract workflow

```bash
make proto      # buf lint + breaking-check + generate ./gen (verify only)
make check      # the merge-gate: proto lint + breaking. Must pass before push.
```

## How a service repo consumes the contract (later phases)

1. Vendor the proto module (git submodule/subtree) **pinned at a tag** (e.g. `proto/v0.1.0`).
2. Run `buf generate` **in your own repo** — generated code lands in your tree.
3. `platform-core` never writes into your repo. See `ADR/0001`.

## Rules you must not break

- **Do not edit generated code** (anything under a `generated/` or `gen/` dir).
- **Do not read another service's DB.** Cross-service data = gRPC call (Rule 3).
- **Do not change proto to suit one service.** Propose it in a PR to `platform-core`;
  a contract change is versioned and affects everyone.
- **Use the right broker for the flow** (`ADR/0002`): state-change **events** →
  Kafka topic `<domain>.events` (key = aggregate id), wrapped in a
  `platform.events.v1.EventEnvelope`; background **jobs/tasks** → RabbitMQ (→ SQS).
  Don't push events through RabbitMQ or jobs through Kafka.
- **Frontend/BFF calls only the Gateway** (Rule 1); **Gateway holds no business logic** (Rule 2).

## When you finish

- `make check` passes (pre-push hook enforces it).
- Push a branch, open a PR. PR title: `[service-name] short description`.
- If you needed a contract change you could not make, say so in the PR — do not
  work around it by editing generated code.
