## Why

**Deployment ≠ Release.** Today the only way to turn a feature on or off in this
marketplace is to ship a container: edit code/env, open a PR, rebuild the image, let
ArgoCD roll it out, wait for pods to become ready. That is *deployment* (GitOps ships
artifacts). It is the wrong tool for *release* — deciding, at runtime, whether an
already-deployed behavior is live. A release toggle must flip in seconds, with no PR,
no rebuild, no restart, so an on-call engineer can kill a misbehaving feature during an
incident. There is **no feature-flag capability today** (AGENTS.md §9 lists it as an
open seam — nothing exists yet).

This change adds the **runtime substrate** for release: a self-hosted **Flipt** flag
server, run in local compose and deployed via GitOps, reachable by the Go services and
the gateway/frontend. It does **not** wire any SDK or gate any feature — that is
`wire-openfeature` (change 2). This change only stands the server up and makes it
addressable.

**Why Flipt (vs alternatives).**
- **Self-hosted, no vendor lock-in, no data egress.** Flipt is a single Go binary
  (~30MB), gRPC + HTTP, stores flags in an embedded DB (SQLite) — it fits the existing
  "one small infra container on the shared network" pattern (like OpenSearch, Valkey,
  Redpanda) with no SaaS account or per-seat cost.
- **gRPC streaming → in-process evaluation.** SDKs keep an in-memory snapshot of flag
  state and re-sync over a stream, so a flag check is a local lookup (≈0ms), not a
  network round-trip per request — critical for gating a hot path like checkout.
- **The abstraction is the CNCF OpenFeature SDK, not Flipt's own client** (wired in
  change 2), so Flipt is a swappable *provider*. If we later outgrow it we replace the
  provider, not every call site.
- **Statsig / A/B experimentation is explicitly out of scope** (see Non-goals). Flipt is
  the internal, infra-owned release-toggle store; A/B analytics is a separate later
  decision.

## What Changes

- **platform-core** (`infra/docker-compose.yaml`) — add a `flipt` service to the shared
  local infra stack: image pinned, HTTP/UI + gRPC ports, a persisted **flag store**
  (SQLite on a named volume so toggles survive restarts), a healthcheck, on the existing
  `platform-core_default` network so every service and the gateway/frontend can reach it
  at `flipt:9000` (gRPC) / `flipt:8080` (HTTP).
- **platform-gitops** — add `platform/infra/flipt.yaml` (Deployment + Service + a PVC
  flag store) in the `marketplace` namespace, picked up by the **existing `infra-shared`
  ArgoCD Application** (sync-wave 0, `directory.recurse` over `platform/infra`). Because
  infra-shared is wave 0 and the services are wave 4, Flipt is healthy before anything
  that will evaluate a flag starts. A `flipt` Service exposes gRPC `:9000` and HTTP
  `:8080` in-cluster (`flipt.marketplace.svc`).
- **No app-code changes**, no proto change, no new gateway route, no SDK. Nothing
  evaluates a flag yet.

## Non-goals

- **No OpenFeature SDK wiring** in any service or the frontend, and **no flag gating any
  feature** — that is change 2 (`wire-openfeature`).
- **No Statsig** and **no A/B testing / experimentation / percentage rollouts / metrics** —
  Flipt here is a boolean/variant release-toggle store only; A/B analytics is a separate,
  later decision.
- **No proto/contract change** (`platform-core/packages/proto` untouched) — Flipt is infra,
  not a marketplace gRPC service.
- **No production secret management** for Flipt beyond the existing local dev posture
  (SQLite, security off locally); prod hardening (managed Postgres backend, auth) is a
  later concern.
