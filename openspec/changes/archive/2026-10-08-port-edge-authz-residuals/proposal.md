## Why

The security port (`port-security-hardening`, archived 2026-10-08) left the rest of three carried-over changes
(`service-authz-hardening`, `authz-hardening-wave2`, `gateway-and-ai-hardening`) open. A requirement-by-requirement
audit of agora's code on 2026-10-08 found these gaps still live:
- The gateway's edge chain (request id, fail-closed token check, logging, rate limit) is unary-only. As a result:
  - `StreamChat`, `POST /api/track` and `/api/admin/metrics` are never rate limited or logged;
  - an invalid bearer on `StreamChat` is silently downgraded to anonymous instead of 401.
- One browser can flood `analytics.events` through `/api/track`.
- AI generation calls go through the read path, which has a 5 s deadline and retries a generation on `Unavailable`.
- The gateway serves gRPC reflection to anyone, in every environment.
- After an upstream restart the gateway keeps dialling the old address for about 30 s, so every call answers 503. The
  e2e suite carries a 35 s settle wait to hide this.
- `make check-env` in team-gateway runs a test that does not exist, so it passes vacuously; `DIAL_TIMEOUT_SECONDS` is
  never read.
- Two product decisions the user took on 2026-10-08 are not reflected anywhere:
  - `ForceFailSaga` becomes admin-only. It is a second cancel path; buyers keep `CancelOrder`.
  - `CreateOrder` accepts only a USER principal.
- Ops and config residue:
  - the reset-token exposure flag is refused only in production, not staging;
  - a payout does not require a seller scope;
  - the gateway's staging/prod overlays set no `ENV`;
  - team-ai has no ingress NetworkPolicy;
  - the legacy `deploy/vault/bootstrap-vault.sh` (shared HS256 secret) is still in the repo;
  - `repo_doctor` does not check that documented env vars are used;
  - local MinIO uses the unpinned `minio/minio:latest`.

## What Changes

- **team-gateway**:
  - Run the same edge chain on Connect streams and on the plain-HTTP routes (`/api/track`, `/api/admin/metrics`): a
    validated request id, the fail-closed token check, a log line and the per-identity rate limit.
  - Cap a stream's request size.
  - Send AI generation calls once, with their own deadline `AI_CALL_TIMEOUT_SECONDS`.
  - Serve reflection only when `EDGE_REFLECTION_ENABLED=true`, and refuse that flag when `ENV` is staging or
    production.
  - Reconnect to a restarted upstream within seconds; this uses `DIAL_TIMEOUT_SECONDS`.
  - Make `make check-env` a real gate.
  - Add `OrderService/ForceFailSaga` to the edge admin policy.
- **team-order**:
  - `ForceFailSaga` is admin-only.
  - `CreateOrder` requires a USER principal.
- **team-identity**: `PASSWORD_RESET_EXPOSE_TOKEN` is refused in staging as well as production.
- **team-payment**: payouts (`RequestPayout`, `RequestWalletPayout`) require `listing.write`.
- **platform-gitops**:
  - `ENV` set in the gateway's staging and prod overlays.
  - team-ai ingress NetworkPolicy (gateway and Prometheus only).
  - The vault app comment points at `platform/vault-config`.
- **Root**:
  - Delete `deploy/vault/bootstrap-vault.sh` and fix its references.
  - `repo_doctor` checks that every env var a service README documents is referenced by that service's code.
- **platform-core**: pin the local MinIO image to the tag GitOps uses.
- **platform-e2e**: scenarios for all of the above, through the edge. The existing `ForceFailSaga` scenarios switch to
  the admin.

Repos touched: team-gateway, team-order, team-identity, team-payment, platform-gitops, platform-core (compose only),
platform-e2e, root. **No proto change.**

**BREAKING (edge behaviour):**
- A buyer calling `ForceFailSaga` now gets 403. The frontend never calls it.
- A SERVICE token calling `CreateOrder` gets 403. No service calls it.
- An invalid bearer on `StreamChat` now gets 401.

## Capabilities

### New Capabilities
- `edge-stream-and-http-policy`: what the gateway enforces on streams and plain-HTTP routes, the AI generation call
  policy, reflection, and recovery after an upstream restart.
- `repo-coherence`: the workspace checks `repo_doctor` enforces.

### Modified Capabilities
- `edge-route-policy`: the admin-gated procedure set gains `OrderService/ForceFailSaga`.
- `order-read-access`: `ForceFailSaga` is admin-only.
- `order-lifecycle-guards`: the cancel row of the transition table names the admin, not the buyer, for
  `ForceFailSaga`.
- `order-checkout-correctness`: only a USER principal can place an order.
- `payment-access-control`: payouts require the seller scope.
- `deploy-runtime`:
  - The gateway and identity strict-ENV guards.
  - The gateway overlays declare `ENV`.
  - team-ai ingress NetworkPolicy.

## Non-goals

- Enforcing `ai:use` on `ShoppingAssistant`/`StreamChat`: the user decided on 2026-10-08 to keep
  `AI_USE_SCOPE_REQUIRED` off.
- The team-ai half of `gateway-and-ai-hardening` and all of `llm-path-resilience`:
  - LLM routing, fallback, breaker, timeouts, usage/quota, history, PII redaction, tracing, eval gate;
  - StreamChat error mapping inside team-ai;
  - quota keyed by a server call id, `LLM_TRACE_CONTENT`;
  - recsys cold start, payload `listing_id`, `model_version`, and entrypoint registration.

  That is the next change, `ai-path-resilience`.
- Stream max duration and per-principal concurrent stream caps. The local chat backend is a mock that finishes
  instantly, so they cannot be verified end to end. team-ai's own timeouts arrive with `ai-path-resilience`.
- `/api/events/live`. It is authenticated and room-authorised today, and nothing broadcasts on it. It stays as is.
- Gateway invalid-token behaviour on unary calls is already fail-closed 401. The old wave-2 "downgrade to anonymous"
  requirement is dropped as a conflict.
- The order's shop-reply flag and the empty `seller_id` default on seller analytics. The live specs
  (`dispute-and-qa-access`, current analytics behaviour) are the decision; they are not reopened.
- NOT_FOUND vs PERMISSION_DENIED for strangers on payment reads. The current PERMISSION_DENIED stays.
- gRPC reflection inside services. They are not reachable from outside the cluster; that is left to the mTLS work.

## Relationship to the carried-over changes

Together with `port-security-hardening`, this change covers every remaining requirement of `service-authz-hardening`
and `authz-hardening-wave2`. It also covers the gateway requirements of `gateway-and-ai-hardening`. When this change is
archived:
- `service-authz-hardening` and `authz-hardening-wave2` are retired.
- `gateway-and-ai-hardening` keeps only its team-ai requirements, which move into `ai-path-resilience`.

## Impact

- **New env vars (team-gateway):**
  - `EDGE_REFLECTION_ENABLED` (default `false`)
  - `AI_CALL_TIMEOUT_SECONDS` (default `30`)
  - `STREAM_MAX_REQUEST_BYTES` (default `16384`)
  - `TRACK_RATE_LIMIT_RPS` / `TRACK_RATE_LIMIT_BURST` (defaults `5` / `20`)

  `DIAL_TIMEOUT_SECONDS` becomes live.
- **Tools that use gateway reflection** (grpcurl against :8080) need `EDGE_REFLECTION_ENABLED=true`. Nothing in the
  repo uses it, so local compose leaves it off.
- **e2e:**
  - `oic_saga_view.feature` and `group_a_steps.py` call `ForceFailSaga` as the admin.
  - The 35 s "container age" settle wait in `support/plp_support.py` / `wait-ready.sh` can be shortened once recovery is
    proven. That is a follow-up, not part of this gate.
