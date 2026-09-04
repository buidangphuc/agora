# platform-core

Central repo of the polyrepo platform: **the contract, the tooling, the templates,
the rules**. Team repos (`team-gateway`, `team-ai`, `team-domain`, `team-frontend`,
`team-data`) hold only business code and consume what lives here.

**Phase 0 (this repo, now): foundation only.** Local dev, no cloud, no CI/CD.
No services are built yet — this repo makes it possible to build them consistently.

## Fixed stack (opinionated — not pluggable)

| Concern | Choice |
|---|---|
| Contract / RPC | Protobuf + gRPC (source of truth in `packages/proto`) |
| Owned DB | PostgreSQL, one per service |
| Cache / rate-limit | Redis (shared) |
| Vector search | Qdrant (shared) — semantic search, later |
| Lexical search | OpenSearch (read-model) — `docs/ADR/0005` |
| Event streaming | Kafka (Redpanda locally) — `docs/ADR/0002` |
| Task queue | RabbitMQ (→ SQS in prod) — `docs/ADR/0002` |
| Observability | OpenTelemetry (exporter-swappable) — `docs/ADR/0004` |

## Quickstart

```bash
brew install bufbuild/buf/buf   # required tooling (docker also required)
make hooks-install              # enable the local merge-gate
make proto                      # lint + breaking-check + generate ./gen
make dev                        # start local infra (postgres-*, redis, qdrant, redpanda, rabbitmq, opensearch)
```

## Layout

```
packages/proto/   gRPC contract — the ONE source of truth
infra/            local docker-compose (fixed stack)
templates/        per-language service seeds (go, python, jvm, ts)
docs/             ARCHITECTURE (3 rules), AGENT_GUIDE, ADRs
tools/ci/         reusable CI workflow (dormant until remote)
gen/              generated code — verification only, gitignored
```

Read `docs/ARCHITECTURE.md` before touching anything. Agents: read `docs/AGENT_GUIDE.md`.
