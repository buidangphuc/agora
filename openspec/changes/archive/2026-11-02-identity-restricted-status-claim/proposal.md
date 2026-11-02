## Why

A restricted seller and a plain buyer carry the same session JWT shape (`sub`, `name`, `typ`, `scopes`, `sid`): restriction only drops `listing.write`. The seller app (:3001) therefore cannot tell the two apart and shows one combined 403 notice. Decided 2026-10-31: the token carries the restriction so the seller app can show a notice per case (follow-up PR `seller-restricted-inline-notice`).

## What Changes

- team-identity signs a `status` claim with the value `"restricted"` when the user's status is RESTRICTED, at login, register and refresh. Tokens of active users are unchanged (claim omitted). SUSPENDED and BANNED users never receive a token, so no other value exists.
- The claim is added in team-identity's own claims type, which embeds `jwtx.Claims`; the shared go-sdk stays at v1.0.2 (team-identity is the only user of `jwtx.Claims`).
- team-gateway is unchanged: its verifier ignores unknown claims and backends keep enforcing restriction through scopes, so no principal metadata is added.
- Docs listing the claims: ADR-0003, `team-identity/README.md`.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

`team-identity/internal/token/jwt.go` (+ test), `team-identity/internal/identity/{auth,refresh}.go` (+ tests), the two docs above.

## Non-goals

Restriction expiry (RESTRICTED stays without `until`), forwarding status to backends, the seller app UI, the go-sdk release.

## Data classification

The account status of the token's own subject is added to a token only that subject and the edge read. It is not logged or stored anywhere new. Personal data category unchanged (account state already returned by `AccountAdminService`).
