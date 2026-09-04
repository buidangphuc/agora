# AGENTS.md — Marketplace polyrepo

Portable working guide for any coding agent (Antigravity, Claude Code, Cursor,
Codex, …) opening this workspace. It is the **map**: where code lives, the rules
you must not break, and the recipe for adding features. Read this first.

> Single source of truth for architecture is `platform-core/docs/` +
> `platform-core/docs/ADR/`. This file summarizes and points there — when a rule
> changes, edit the doc/ADR, not just this file.

---

## 1. What this is

An **AI-first marketplace** (Shopee-like) built as a **polyrepo**: one
`platform-core` holds the contract + infra + rules; each `team-*` sibling holds
exactly one service's business code. Services talk **gRPC**, defined once in
`platform-core/packages/proto`. Browsers reach services only through one edge
(`team-gateway`). Write-side emits **Kafka** events; a separate read-side service
consumes them (CQRS).

```
browser ─▶ team-frontend (Next.js SSR) ─▶ team-gateway (Connect edge) ─gRPC─▶ services
                                                                          │
   team-domain ──(Kafka: listing.events)──▶ team-search (OpenSearch read-model)
```

## 2. The repos (all exist and run)

| Repo | Lang | Role | gRPC port |
|---|---|---|---|
| **platform-core** | — | Contract (proto), infra (compose), ADRs, tooling, seed | — |
| **platform-e2e** | Python | pytest-bdd + Playwright E2E platform (POM, per-scenario World, tag-driven API seeding); drives team-frontend :3000 via the gateway :8080 | — |
| **team-identity** | Go | Register/login, issues RS256 JWT + serves JWKS (:50063), role→scopes, password reset | :50053 |
| **team-domain** | Go | Listing write-model + owner enforcement, emits Kafka events | :50051 |
| **team-search** | Go | Consumes `listing.events` → OpenSearch; search/suggest (read-model) | :50052 |
| **team-engagement** | Go | Favorites, stats, reviews, Q&A, disputes | :50054 |
| **team-order** | Go | Distributed Saga purchase, cart, SPX shipment & RMA returns | :50055 |
| **team-payment** | Go | Mock payment transactions, refund & seller wallet payouts | :50056 |
| **team-chat** | Go | Buyer-seller chat threads & real-time messaging | :50057 |
| **team-notification**| Go | In-app notification center | :50058 |
| **team-gateway** | Go | Connect edge: verify JWT once, rate-limit/CORS, forward to services | :8080 (HTTP) |
| **team-frontend** | TS | Next.js 14 App Router SSR; consumer home + `/seller` area + auth | :3000 |
| **team-ai** | Python | FastAPI AI service (RAG/completions/magic-listing) | :8000 |
| **team-analytics** | Go | Kafka worker (§6c): `analytics.events` → analytics warehouse (DB-per-service; no business RPC, health only) | :50059 |
| **team-promotion** | Go | Shop/platform vouchers + flash-sale campaigns; checkout redemption (reserve→commit/release); emits `promotion.events` | :50061 |
| **team-referral** | Go | Referral codes + invite tracking (owns its DB) | :50062 |
| **team-verification** | Go | Seller/user KYC verification submissions (owns its DB) | :50064 |
| **team-sharing** | Go | Shareable listing links + share tracking (owns its DB) | :50065 |
| **team-audit** | Go | Audit event log (owns its DB) | :50066 |

## 3. The rules you must not break

1. **Frontend/BFF talks only to the Gateway.** Never call a service directly. The
   frontend holds no business logic — UI shaping only.
2. **Gateway routes + orchestrates; holds no business logic.** It knows *who to
   call* and *deadlines*, not *what the answer is*. It verifies auth **once** and
   forwards a resolved principal downstream.
3. **Each service owns its DB.** No service holds another's DB connection string.
   Need another service's data? Call its gRPC — never join across DBs.
4. **Contract is source of truth.** A message/RPC is defined only in
   `platform-core/packages/proto`. Never hand-edit generated code. A proto change
   is a versioned PR to platform-core that affects everyone — don't fork the
   contract to suit one service.
5. **Right broker for the flow** (ADR-0002): state-change **events** → Kafka topic
   `<domain>.events` (key = aggregate id), wrapped in
   `platform.events.v1.EventEnvelope`. Background **jobs/tasks** → RabbitMQ (→ SQS
   in prod). Don't push events through RabbitMQ or jobs through Kafka.

## 4. How the pieces wire

- **Auth (ADR-0003 + ADR-0006).** `team-identity` signs an **RS256** JWT (roles:
  admin/seller/buyer → scopes) with a private key it alone holds, and publishes
  its public key(s) at `GET /.well-known/jwks.json` (:50063). `team-gateway` is
  the **only** JWT verifier: it fetches + caches identity's JWKS and verifies the
  bearer by `kid` (holding **no** signing secret), resolves a
  `Principal{id,type,scopes}`, and forwards it to services as trusted
  `x-principal-{id,type,scopes}` gRPC metadata (rebuilt each hop, so a client
  cannot spoof it). Anonymous → public scopes. Services trust the forwarded
  principal and gate RPCs with scope checks (`RequireScopes`). Identity reads
  `JWT_PRIVATE_KEY`+`JWT_KID` (Vault, identity-only); the gateway reads only
  `JWKS_URL`. There is no shared `JWT_SECRET`.
- **CQRS (ADR-0005).** `team-domain` is the write-model (source of truth). On
  create/update/delete it publishes a `ListingChanged` event to Kafka
  `listing.events`. `team-search` consumes and upserts/deletes into OpenSearch —
  a rebuildable read-model (replay the topic to rebuild; it holds nothing the
  events don't carry). Search/recommendation stay lexical for now; embeddings
  come later "when there's enough data".
- **Proto distribution (ADR-0001).** Each consumer repo **vendors** `proto/` and
  runs `buf generate` **in its own tree**. Generated code is gitignored.
  platform-core never writes into a service repo.

## 5. Local run (Docker-only)

Go and buf are **not installed on the host** — all Go/proto work runs in Docker.
Infra + services run on the compose network `platform-core_default` (compose
project name `platform-core`).

```bash
# 1. infra (from platform-core/infra)
docker compose -p platform-core up -d      # postgres-* redis qdrant redpanda rabbitmq opensearch otel-collector

# 2. each Go service: build + run its container on the platform-core_default network
#    (see each repo's Dockerfile / docker-compose.local.yaml)

# 3. frontend
cd team-frontend && npm run dev             # http://localhost:3000

# 4. seed demo data through the gateway
platform-core/tools/seed-marketplace.sh
```

Ports: gateway `:8080`, frontend `:3000`, domain `:50051`, search `:50052`,
identity `:50053`, engagement `:50054`, promotion `:50061`. Infra: postgres-listing `5433`,
postgres-search `5434`, postgres-identity `5435`, postgres-engagement `5436`,
postgres-promotion `5440`, postgres-notification `5441`,
redis `6379`, qdrant `6333/6334`, redpanda `19092` (host), rabbitmq `5672/15672`,
opensearch `9200`, otel-collector `4317`.

Gateway env (must match reality): `HTTP_PORT`, `UPSTREAM_{SEARCH,LISTING,IDENTITY,
ENGAGEMENT}_ADDR`, `JWKS_URL`, `RATE_LIMIT_RPS/BURST`, `CORS_ORIGINS`,
`OTEL_EXPORTER_OTLP_ENDPOINT`. (No shared `JWT_SECRET` — ADR-0006 made the gateway
a verify-only party that holds no signing material; it fetches identity's JWKS.)

## 6. Recipe — add a feature

**a) New field / RPC on an existing domain**
1. Edit the `.proto` in `platform-core/packages/proto`. Keep it **non-breaking**
   (add fields/RPCs; never renumber/remove). `buf lint` + `buf breaking` must pass.
2. Re-vendor `proto/` into the owning service repo + regenerate.
3. Implement handler/service/repository in that one repo. Add a DB migration if a
   new column/table is needed.
4. Route it in `team-gateway` (add a forwarder method) + surface in `team-frontend`.

**b) New capability on an existing domain's data** → stays in that domain's repo
(reads/writes its tables). No new repo.

**c) New bounded context** (owns new tables + its own lifecycle, e.g. orders,
payments) → **new `team-*` repo**. Copy the shape of an existing Go service
(config + bootstrap + gRPC transport + interceptors + migrations), add its proto
to platform-core, add a gateway forwarder.

**Decision rule — new repo or not:** a service = *one domain that owns its own
data*. New feature reads/writes an existing table → same repo. New feature owns a
new table + lifecycle → new repo. Scaling *load* = run more instances of a
service behind the gateway; it never requires a new repo.

## 7. Conventions

- **Proto-first, buf v2**, managed mode; STANDARD lint (except
  `ENUM_ZERO_VALUE_SUFFIX`), breaking check = FILE. All contract changes additive.
- **Go services mirror the FastAPI reference** template: reflection-based config
  with `env:`/`default:` struct tags + a `.env.example` drift gate; bootstrap
  Resources/lifecycle (pgxpool + health); gRPC transport + interceptors;
  in-process gRPC tests on ephemeral ports. `gofmt` + `go vet` + `go test` clean.
- **Go 1.22 dependency pins** (do not bump without checking Go version): franz-go
  `v1.18.0`, opensearch-go/v2 `v2.3.0`, golang-jwt/v5 `v5.2.1`, rs/cors `v1.11.0`,
  golang.org/x/time `v0.5.0`.
- **Frontend**: Next.js 14 App Router + Tailwind, Connect-ES v1, a server-only
  gateway module, per-request clients built with the caller's token from the
  httpOnly `session` cookie. `next build` + `tsc` clean.
- **Financially sensitive** (payments / wallet / boost): **mock/placeholder only**
  — never implement real money movement.
- **Commits** authored as `Bùi Đăng phúc <phuc.buidang@batdongsan.com.vn>`, and
  end the commit message with:
  `Co-Authored-By: <your agent> <noreply@…>`.
  Commit per phase; don't push unless asked.

## 8. Mapping code before editing

Use **codegraph** MCP tools if available (`.codegraph/` present) to trace flow +
symbols before writing. Otherwise fall back to `rg`, `sed`, `nl`, and focused
tests. Don't reverse-engineer wiring from scratch — the ADRs + this map tell you
where things are.

## 9. Deliberately not built yet (open seams)

Still open: **real** payments/wallet/boost (mock only — policy §7), search
ranking + recommendation surfacing (waiting on data), RabbitMQ task wiring,
service-to-service **zero-trust** (mTLS / services reject non-gateway
`x-principal-*` — ADR-0010 adds the interim NetworkPolicy), token refresh/revoke,
automated key-rotation scheduling.

(Now built — no longer open seams: product images upload, buyer↔seller chat,
orders + distributed saga + returns, mock payments/wallet/payout, in-app
notifications, and team-ai wired via the gateway. See §2.)

RS256/JWKS issuer-verifier rotation is now **built** (ADR-0006): identity signs
RS256 + serves JWKS; the gateway verifies by `kid` against the cached keyset.
Automated key-rotation scheduling and downstream zero-trust self-verify remain
deferred (above).

## 9b. Spec-driven development (OpenSpec) — how to add a feature

New requirements flow through **OpenSpec** (`openspec/`), not ad-hoc. speckit is
retired. Pipeline: `requirement → change → (code ∥ e2e) → gate → archive`, designed
so many changes/agents run in parallel (each change is an isolated
`openspec/changes/<id>/`). Full plan: `SPEC_DRIVEN_WORKFLOW.md`.

- Propose: `/opsx:propose "<idea>"` (or author `openspec/changes/<id>/{proposal,tasks}.md` + `specs/<capability>/spec.md`); `openspec validate <id> --strict`.
- Build in parallel with the **`spec-dispatch`** skill: code track (per repo, obey rules §3) ∥ e2e track (**`spec-to-e2e`** skill → updates the owning repo's `FEATURES.yaml` + a `.feature` in `platform-e2e`).
- Every user-facing `#### Scenario:` must become an automated e2e (shift-left). Gate before archive: `make -C platform-e2e spec-check CHANGE=<id>` (manifests valid + every change scenario has a green test) + `pytest` green + lint.
- Archive: `openspec archive <id>` folds the delta into `openspec/specs/`.
- Conventions for OpenSpec live in `openspec/config.yaml`; e2e conventions in `platform-e2e/AGENTS.md` + `docs/FEATURE_MANIFEST.md`.

## 10. Deeper docs

- `platform-core/docs/ROADMAP.md` — core ecommerce feature plan (phases 3–7).
- `platform-core/docs/ARCHITECTURE.md` — the 3 rules + protocol-per-hop.
- `platform-core/docs/AGENT_GUIDE.md` — contract workflow + merge gate.
- `platform-core/docs/ADR/000{1..5}-*.md` — proto distribution, async broker,
  auth model, observability, search read-model.
- Each repo's `README.md` + `.agents/*/SKILL.md` (where present).
