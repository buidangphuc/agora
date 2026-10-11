## Context

See proposal.md. Today `authenticate_bearer_token` (static `AUTH_BEARER_TOKEN`, scopes from `AUTH_ROLES`) guards only the
completions routes; the tag routes have no dependency. team-ai does not verify JWTs: team-gateway is the only verifier
and forwards `x-principal-*` over gRPC only. The tag routes are plain HTTP (host port 8001 in compose), not routed by the
gateway, and no frontend or service calls them (team-search uses gRPC `ClassifyTags`).

## Decisions

- **D1: service/admin token, not gateway admin RPCs.** Exposing explore/promote through the gateway would need new
  `AIService` RPCs (a proto change), a forwarder and an edge policy, for operations that no UI performs. The existing
  internal-call mechanism (bearer token, constant-time compare, principal with scopes) is the smallest consistent fix.
  Alternative kept for later: admin RPCs when a taxonomy console exists.
- **D2: two tokens for two principals.** One static token cannot be both a read-only service and an admin, so
  `AUTH_BEARER_TOKEN` stays the service token (scopes = `AUTH_ROLES`, `ai.classify` for read) and the new
  `AUTH_ADMIN_BEARER_TOKEN` yields `admin` + `ai.classify`. The admin token is honoured only by the tag-route
  dependencies (not by `require_principal`, so completions and the gRPC bearer fallback do not gain admin access).
- **D3: default deny.** An empty `AUTH_ROLES` gives the service token no scope, so reads are 403 until `AUTH_ROLES`
  includes `ai.classify`; an unset admin token means nobody is admin. 401 for no/invalid token, 403 for missing scope.
- **D4: no user tokens.** A buyer's RS256 JWT is not a team-ai credential: it fails the static compare and gets 401.
  (Verifying it would put a second JWT verifier outside the gateway, which ADR-0006 forbids.)

## Why one scenario is not an end-to-end test

"Admin access is off unless configured and strong" depends on process configuration the running stack does not expose;
`VERIFIED BY` unit tests, `not-testable` entry.

## Deployment needs (team-ai, local compose and the e2e runner)

| Variable | Value | Why |
|---|---|---|
| `AUTH_BEARER_TOKEN` | a local service token | read-only service principal |
| `AUTH_ROLES` | `ai.classify` | the service token's scopes |
| `AUTH_ADMIN_BEARER_TOKEN` | a local admin token | explore/promote |

The e2e runner reads the same two values from `TEAM_AI_SERVICE_TOKEN` and `TEAM_AI_ADMIN_TOKEN` (defaults
`agora-local-team-ai-service` and `agora-local-team-ai-admin`, so compose should default to them with
`${TEAM_AI_SERVICE_TOKEN:-agora-local-team-ai-service}` etc.). In production these come from Vault, 24+ characters.

## Risks / Trade-offs

- Static shared tokens are coarse (no per-caller identity) → acceptable for an internal operator surface; rotation is a
  redeploy.
- Existing callers of the REST tag routes break → none in the repo (checked: only the e2e steps and the pipeline CLI,
  which calls the service in process).
