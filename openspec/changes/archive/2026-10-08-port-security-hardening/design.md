## Context

agora's services already forward `x-principal-{id,type,scopes}` from the gateway, and service-to-service calls set a
SERVICE principal in each caller's `internal/upstream` client. The retired `full_team_repo` had the hardening below.
Its code is in `~/Documents/agora-archive/full_team_repo/bundles`, but agora diverged (outbox relayers, a rewritten
gateway, a placements engine in team-ai). The port is therefore by behaviour, adapted to agora's helpers, not a copy.
See proposal.md for the gaps.

## Goals / Non-Goals

**Goals:**
- Close the edge-reachable holes first.
- Keep every legitimate caller working: the frontend, the inter-service saga and the e2e suite.

**Non-Goals:**
- The correctness work (next wave).
- `ai:use` enforcement.
- Streaming gateway interceptors.

## Decisions

- **Unroute at the edge, and gate in the service.** Internal RPCs are removed from the gateway, so they answer 501,
  and they are also service-only in their service. The gateway is the outer layer, and the service stays
  authoritative, per AGENTS.md rule 2.
  - Alternative considered: gate only in the service. Rejected, because the gateway forwards only user principals,
    so a browser could never use these RPCs legitimately anyway.
- **Scopes are minted by the calling service, per call.**
  - team-order marks each upstream call with exactly one scope: `promotion.reserve` for the voucher saga,
    `listing.write` for stock, `listing.read` as the background default.
  - team-order drops the forwarded bearer on marked calls.
  - `promotion.reserve`, `audit.write` and the other service-only scopes are pinned by a team-identity drift test
    as granted to no role.
- **Preview namespace instead of a separate RPC.**
  - `ValidateAndReserve` stays one RPC. A user caller is downgraded to a preview bound to its own id
    (`preview:<id>:`), which the frontend already uses.
  - Alternative considered: a new `PreviewVoucher` RPC. Rejected because it needs a proto change and buys nothing.
- **Visibility is decided by the owning service.**
  - team-domain decides draft visibility for listing reads.
  - team-search applies the same policy to its read model. The indexer already stores `status`, and rejected
    listings are deleted from the index.
- **Shop-reply ownership comes from engagement's own `seller_listings` projection** (fed by `listing.events`), not a
  new gRPC dependency on team-domain.
- **One error sanitiser per process.**
  - Each service maps unknown errors to a generic `internal error` and logs the cause.
  - The gateway re-sanitises internal and unavailable errors as defence in depth.
- **Boot guards read the existing `ENV` variable.**
  - platform-gitops sets `ENV` in the staging and prod overlays, so the guards engage there and never locally.
  - team-ai reads `ENVIRONMENT` and has no `staging` value, so it is left for a follow-up.

## Risks / Trade-offs

- **Existing tokens:** tokens minted before the scope grant lack `ai:use` and `recommendations:read` until re-login.
  → Nothing enforces `ai:use` yet, and `Recommend` gates on existing scopes.
- **e2e assumptions:**
  - e2e steps that wrote audit events from a user, or named the dispute defendant by username, break.
  - → They are rewritten to the new contract in the e2e track.
- **Self-purchase:** a seller buying their own listing in an e2e seed now fails. → Steps use distinct buyer and
  seller accounts.
- **Staging deploys:** a deploy with `ENV=staging` and dev secrets (dev JWT key, `minioadmin`) refuses to boot.
  → This is intended, and is documented in the proposal impact.

## Migration Plan

- No data migration.
- Deploy the services in any order:
  - The gateway unrouting is independent.
  - team-promotion's service-only saga gates must not reach an environment before team-order's
    `promotion.reserve` principal, so deploy them together.
- Rollback is a revert of the per-repo commits.
