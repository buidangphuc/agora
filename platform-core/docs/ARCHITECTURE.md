# Architecture

The platform is a **polyrepo**: one `platform-core` (this repo) holds the contract,
tooling, templates, and rules; each `team-*` repo holds one service's business code.
Services talk over **gRPC**, defined once in `packages/proto`.

```
Web + WAP (browser) ─▶ team-frontend (Next.js SSR) ─┐
                                                     ├─▶ team-gateway (Go) ─gRPC─▶ services
Native App ──────────────────────────────────────── ┘        (edge)               (Python/JVM)
```

## The 3 rules (do not break)

**Rule 1 — Frontend/BFF only talks to the Gateway.**
The Next.js/BFF layer calls **only** `team-gateway`, never a service directly.
It holds no business logic — shaping for the UI, nothing more.

**Rule 2 — Gateway routes and orchestrates; it holds no business logic.**
The Gateway knows *who to call* and *timeouts*, not *what the answer is*. Its
outward REST/JSON (for native app) stays a **generic** 1:1 reflection of gRPC —
the moment a client needs tailored aggregation, that logic goes in a BFF, never
in the Gateway.

**Rule 3 — Each service owns its DB schema.**
No service holds another service's DB connection string. Want another service's
data? Call its gRPC — **never** join across DBs.

## Protocol per hop

| Hop | Protocol |
|---|---|
| Browser (web/wap) → Next.js | HTTP/SSR |
| Next.js (server-side) → Gateway | gRPC-web / Connect |
| Native app → Gateway | JSON/REST (generic, Connect) |
| Gateway → service | gRPC |
| Service ↔ service (sync) | gRPC |
| Service → service (async event) | Kafka event (`<domain>.events`) |
| Task / job dispatch | RabbitMQ (→ SQS in prod) |

Browsers cannot speak raw gRPC — the Gateway edge adapts (gRPC-web/Connect + SSE
for streaming). Everything east-west that is synchronous is gRPC.

## Async, events, and read-models

Not everything is a synchronous gRPC call. Two brokers, split by role (ADR-0002):

- **Kafka = event streaming.** A service publishes state-change events to
  `<domain>.events` (key = aggregate id) wrapped in a `platform.events.v1.EventEnvelope`
  (carries `principal` + `traceparent`/`request_id`). Other services consume them.
- **RabbitMQ = task/work queues** for background jobs (→ SQS in prod).

This enables **CQRS read-models** (ADR-0005): a service owns the write-model
(source of truth in its DB) and emits events; a *different* service consumes them
to maintain a derived, rebuildable read-model. First instance: `team-domain` writes
listings → `team-search` indexes them into OpenSearch for buyer search. A read-model
is rebuilt by replaying its topic, so it holds nothing the events don't carry.

## Consistency hardening at the purchase seam

The purchase path is made durable so state cannot silently diverge across the
order/domain/payment seam (SA review criticals):

- **Durable saga + compensation** (`ADR/0007`): team-order persists reservation/saga
  state before external effects; compensations run on a fresh context and are
  retried/parked, never discarded.
- **Inventory reservation model** (`ADR/0008`): `ReserveStock` is idempotent on a
  caller-supplied `reservation_id` (retries never double-decrement), and reservations
  carry a TTL swept back to available stock if a checkout crashes.
- **Payment↔order event integration** (`ADR/0009`): settle is event-carried —
  team-payment emits `platform.payment.v1.PaymentSettled` on `payment.events` via its
  transactional outbox; team-order consumes it idempotently to reach PAID (no
  fire-and-forget RPC dual-write). Consumers commit offsets only after the handler
  succeeds, retrying then routing poison records to `<topic>.dlq`; read-model docs
  carry a monotonic `version` guard.
- **Service zero-trust** (`ADR/0010`): an interim gitops NetworkPolicy restricts each
  service's gRPC port to team-gateway (+ required intra-namespace peers), closing the
  forge-a-principal path at the network layer while mTLS is scheduled.

## Contract is the source of truth

`packages/proto` is the only place a message or RPC is defined. Generated code is
never hand-edited. Contract changes are **versioned** and consumed deliberately —
see `ADR/0001-proto-distribution.md`.

## Cross-cutting standards

- **Auth** (`ADR/0003`): a resolved `platform.common.v1.Principal` (`id`, `type`,
  `scopes`) flows on every call; interceptors resolve it from gRPC metadata.
  Scope checks gate RPCs. JWT/cookie is an open extension.
- **Observability** (`ADR/0004`): OpenTelemetry. Interceptors propagate W3C
  `traceparent` + a bridged `x-request-id` on every hop. Exporter is swappable.
- **DB-per-service** (Rule 3), **fixed stack** (see README).

## What is intentionally NOT here (Phase 0)

No JWT/cookie auth, no cloud/CI. Those are later phases — this repo only makes
them buildable in a consistent way. (Async brokers are now chosen — ADR-0002 —
and the first services + search read-model exist as sibling repos.)
