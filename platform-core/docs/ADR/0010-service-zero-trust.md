# ADR-0010 — Service-to-service zero-trust (interim NetworkPolicy)

**Status:** Accepted (interim) · **Date:** 2026-09-03 · **Relates to:** ADR-0006

## Context

Auth (ADR-0006) makes `team-gateway` the sole JWT verifier: it resolves a
`Principal` and forwards it downstream as trusted `x-principal-{id,type,scopes}`
gRPC metadata, rebuilt each hop so a client cannot spoof it. That guarantee holds
**only for traffic that goes through the gateway**. Services trust the
`x-principal-*` metadata of *any* caller that can reach their gRPC port
(`:50051–50059`) on the internal network — there is no service identity / mTLS. A
workload that reaches a service port directly can **forge an admin principal**. This
is a real SPOF/authz gap the review flagged, and it had no ADR.

## Decision

- **Interim: network-level isolation.** A gitops **NetworkPolicy** restricts each
  service's gRPC port so only `team-gateway` (and required intra-namespace peers such
  as an event consumer) may reach it. This removes the "any pod can forge a
  principal" path without app changes.
- **Follow-up (deferred, tracked): service-identity mTLS.** Real zero-trust —
  mutual TLS with per-service identity so a service can *cryptographically* trust the
  caller is the gateway (or an authorized peer) and reject forged `x-principal-*`.
  Not in this change; recorded here so it is not lost.

## Alternatives rejected

- **Do nothing / rely on network trust** — the status quo gap.
- **App-level shared secret between gateway and services** — reintroduces a shared
  secret (the exact anti-pattern ADR-0006 removed) and is weaker than mTLS.
- **Full mTLS now** — correct end state but a large cross-cutting rollout; the
  NetworkPolicy is the high-value interim step while mTLS is scheduled.

## Consequences

- Direct service-port access from arbitrary workloads is blocked in deployed
  environments; the forge-admin path is closed at the network layer. Local docker-
  compose is unaffected (single trusted network) — the gap there is accepted for dev.
- `x-principal-*` is still trusted at the app layer, so this is **defense-in-depth,
  not a full fix**; the mTLS follow-up remains required before untrusted workloads
  share the cluster. Adds a NetworkPolicy manifest to platform-gitops.
