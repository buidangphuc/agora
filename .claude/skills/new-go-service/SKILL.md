---
name: new-go-service
description: Scaffold a new Go gRPC service (team-<name>) by stamping the sanctioned seed at platform-core/templates/_service-go (handler → service → repository layering, auth+tracing interceptors, health/reflection, pinned proto-vendor submodule). Use when adding a brand-new backend service. Pairs with the gitops-scaffold-service skill (Helm/ArgoCD infra) — this does the service code.
disable-model-invocation: true
---

# Scaffold a new Go service

Create `team-<name>/` as a sibling repo that matches the existing Go services
(model on `team-payment` / `team-order`). **Read `AGENTS.md` first** and obey §3
(service owns its DB; talks to others only via gRPC; contract lives in
platform-core). This skill scaffolds the **service code**; run
`gitops-scaffold-service` for the Helm/ArgoCD infra, and `proto-change` /
platform-core for the contract.

## Inputs to confirm with the user
- **name** (e.g. `wishlist`) → repo `team-wishlist`, Go module
  `github.com/buidangphuc/team-wishlist`.
- **gRPC port** — next free `:5005x` (see the port table in `AGENTS.md` §2; don't
  collide).
- **proto package** — `platform/<name>/v1` (must already exist or be added in
  platform-core via the `proto-change` skill first).
- Does it **own a DB**? (→ `migrations/` + `internal/repository`) and does it
  **emit Kafka events** or **consume** any? (ADR-0002 / §5).

## Steps → verify

> Stamp from the sanctioned seed, don't hand-copy a sibling. The generator is
> `platform-core/templates/_service-go/` — its `README.md` is the authoritative
> instantiation procedure (module rename, pinned proto submodule per ADR-0001,
> codegen, merge gate). Follow it; the steps below add only the platform wiring
> the README doesn't know about. (Existing services like `team-payment` predate
> the template and vendor proto under `proto/` instead of `proto-vendor/` — mirror
> the **template**, not them, for a new service.)

1. **Stamp the template.** Copy `platform-core/templates/_service-go/` → `team-<name>/`.
   Then follow `_service-go/README.md` → "Instantiate a new service":
   - Rename the module: replace `github.com/your-org/team-service` everywhere
     (`go.mod`, imports, `buf.gen.yaml` `go_package_prefix`) with
     `github.com/buidangphuc/team-<name>`.
   - Pin the proto submodule under `proto-vendor/` at a tag (see
     `proto-vendor/README.md`), then `make proto && go mod tidy`.
   - `cp .env.example .env`; set the service's `:5005x` port (next free in
     `AGENTS.md` §2 — don't collide) and OTEL/DB env.
   → verify: `grep -rn "your-org/team-service" team-<name>` returns nothing;
     `generated/` populated; `make check` (gofmt, vet, test) green.

2. **Replace the seed RPC with your contract.** The template ships a
   `platform.listing.v1.ListingService` stub. Swap in your service's proto package
   `platform/<name>/v1` (add it in platform-core first via the `proto-change`
   skill if it doesn't exist). Keep the layering: `handler` (wire adapter) →
   `service` (logic) → `repository` (storage port). Never hand-edit `generated/`
   (the `guard-generated` hook blocks it).
   → verify: handlers implement the generated server; `make check` green.

3. **Scope-gate every RPC.** Each RPC that mutates or exposes protected data gets
   `interceptor.RequireScopes(...)`. The template's auth interceptor already
   resolves the forwarded principal — trust `x-principal-*`, never verify the JWT
   here (only the gateway does, `AGENTS.md` §4).
   → verify: `auth-scope-reviewer` subagent reports CLEAN.

4. **Own your DB (if any).** Fill `migrations/` (`golang-migrate`). Reach other
   services' data over gRPC via `internal/upstream/` clients — never their DB
   (§3 rule 3).
   → verify: migrations apply; no other service's connection string appears.

5. **Register in the stack.** Add the service to `docker-compose.services.yaml`
   (its `:5005x` port + env, and its DB if it owns one).
   → verify: `docker compose -f docker-compose.services.yaml config` is valid.

6. **FEATURES.yaml + gateway route.** Add the capability to
   `team-<name>/FEATURES.yaml`; if browser-facing, add a route in `team-gateway`
   (gateway orchestrates + sets deadlines, holds no business logic — §3 rule 2).
   → verify: `contract-boundary-reviewer` subagent reports CLEAN.

7. **Smoke test.** Keep the template's server test shape (health + reflection are
   pre-wired); add a per-RPC test modeled on
   `team-payment/internal/grpcserver/server_test.go`.
   → verify: `make check` / `go test ./...` green.

## Then
- Run `gitops-scaffold-service` to add the Helm sub-chart + ArgoCD wiring.
- If this service introduced/changed any proto, that change belongs in
  platform-core — use the `proto-change` skill.
