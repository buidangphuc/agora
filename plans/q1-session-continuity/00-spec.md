# 00-spec — Session continuity: return after login, stay signed in

initiative: q1-session-continuity

Source: `plans/pm-review-2026-10/BACKLOG.md` Q1 (buyer-journey #5, growth-retention #8).

## Goal
A shopper who is sent to log in lands back on the page they came from, not `/`.
A shopper who logged in stays signed in across visits for up to 30 days of inactivity, with no hard logout after 1 hour.
Logging out or revoking a session ends that session's ability to stay signed in.

## User stories
1. **Return after login.**
   - **Given** a guest on `/listing/abc?variant=v2` who clicks "Thêm vào giỏ", **when** the gateway answers Unauthenticated, **then** they are sent to `/login?returnUrl=%2Flisting%2Fabc%3Fvariant%3Dv2`. After a successful login they land on `/listing/abc?variant=v2`.
   - **Edge:** if `returnUrl` is absolute (`https://evil.example`), protocol-relative (`//evil`), or points at `/login` or `/register`, the shopper lands on `/` (open-redirect guard).
   - **Edge:** every page guard that today does `redirect("/login")` passes its own path as `returnUrl`. This covers seller, account, checkout, favorites, notifications and admin. The register form carries `returnUrl` through too.
2. **Stay signed in.**
   - **Given** a shopper logged in yesterday, **when** they open the site after the access token expired, **then** the page renders signed in without a visible login. Middleware exchanges the refresh token for a new access token and a new refresh token.
   - **Edge:** if the refresh token has expired, the session is revoked, or the refresh fails, both cookies are cleared and the page renders anonymous, as it does today.
   - **Edge (reuse):** if a refresh token that was already rotated is presented again, the whole session is revoked and `SessionRevoked` is published, so a stolen refresh token cannot outlive the rotation.
3. **Logout ends the session.** **Given** a signed-in shopper, **when** they log out, **then** the server revokes their session, so the refresh token is dead, and both cookies are cleared.

## Non-goals
- A guest cart kept in a cookie and merged on login, or auto-replaying the add-to-cart after login. The shopper clicks again on the returned page.
- A "remember me" checkbox. Every login gets the 30-day sliding refresh.
- Changing the access-token TTL (stays `JWT_TTL_SECONDS`, default 3600) or the signing scheme (RS256, ADR-0006).
- Native or mobile clients, and OAuth/social login.

## Architecture decisions
- **Refresh token is opaque, not a JWT.** It is 32 random bytes, base64url-encoded. Only its SHA-256 hash is stored, on the existing `sessions` row (one refresh secret per session). Why: it is revocable server-side and never verifiable at the edge.
- **Rotation on every use, with a 30-day sliding expiry.** `refresh_expires_at = now + 30d` on issue and on each refresh. The previous hash is kept as `prev_refresh_hash`. Presenting it means reuse, which revokes the session and publishes `SessionRevoked` (existing event and topic). Why: this is the OAuth 2.0 Security BCP rotation-with-reuse-detection pattern.
- **Refresh keeps the same session id (`sid`).** The new access token carries the same `sid`, and `last_seen` is updated. Why: the existing denylist, ListSessions and RevokeSession keep working unchanged.
- **The refresh happens in Next.js middleware, server-side.** The refresh cookie `agora_refresh` is httpOnly, `sameSite: lax`, `path: /`, with `maxAge` of 30 days. Middleware runs only when the access cookie is missing or expired *and* the refresh cookie is present. Why: Server Components cannot write cookies, and middleware already owns stale-cookie clearing.
- **The gateway forwards `RefreshSession` as a public RPC.** It requires no Principal, like Login and Register. It is not retried. Why: the access token is expired by definition when it is called.
- **`returnUrl` is validated by one frontend helper, `safeReturnUrl()`.** The value must start with a single `/`, must not start with `//` or `/\`, must contain no scheme, and must not target `/login` or `/register`. Anything else falls back to `/`. Why: one guard, unit-tested, used by login, register and every page guard.

## Contracts
```proto
// platform/identity/v1/identity.proto
message AuthResult {
  string token = 1;
  platform.common.v1.Principal principal = 2;
  string username = 3;
  string refresh_token = 4;                         // opaque; empty when sessions are disabled
  google.protobuf.Timestamp refresh_expires_at = 5;
}

// add to AuthService
rpc RefreshSession(RefreshSessionRequest) returns (RefreshSessionResponse);
message RefreshSessionRequest { string refresh_token = 1; }
message RefreshSessionResponse { AuthResult result = 1; } // new access + rotated refresh token
// Errors: Unauthenticated for an unknown, expired, revoked or reused token (reuse also revokes the session).
```
Existing contracts consumed unchanged:
- `SessionService.RevokeSession`, used by logout.
- `identity.events/SessionRevoked`, published on reuse and consumed by the gateway denylist.
- `AuthService.Login` and `AuthService.Register`, which now also fill fields 4 and 5.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner (FEATURES.yaml repo) | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| contract | q1-session-refresh-contract | platform-core | – | provides: AuthService.RefreshSession, AuthResult.refresh_token | platform-core |
| sessions can be refreshed with rotation + reuse detection | identity-session-refresh | team-identity | q1-session-refresh-contract | uses: AuthService.RefreshSession, identity.events/SessionRevoked | team-identity, team-gateway |
| shopper stays signed in; logout revokes | frontend-session-continuity | team-frontend | identity-session-refresh | uses: AuthService.RefreshSession, SessionService.RevokeSession | team-frontend |
| shopper returns to the page they came from after login | frontend-login-return | team-frontend | – | – | team-frontend |

Hot files edited by more than one slice (initiative-build serialises those slices):
- `team-frontend/src/features/auth/actions.ts`: frontend-session-continuity and frontend-login-return.

## Out of initiative
- The ROADMAP and MASTER_CAPABILITY_MATRIX doc drift is handled in backlog Q38.

## NFR and data (initiative level; each slice's proposal.md refines them)
- **NFR:**
  - RefreshSession p95 < 100 ms at identity.
  - Middleware adds at most one gateway call per expired-access request.
  - Concurrent requests in the same render must not double-rotate. The middleware runs once per navigation, and identity tolerates a refresh of the token *just* rotated (grace window 10 s returns the same new pair) so parallel tabs don't trip reuse detection.
- **Personal data:**
  - Refresh-token hash, kept for the session lifetime of 30 days sliding. Purpose: authentication. Basis: contract.
  - Device and IP are already stored per session, unchanged.
  - No new fields are sent to analytics.
- **Risk routing expected:** auth, proto (auth-scope-reviewer, contract-boundary-reviewer).

## Conventions
- Mirror `team-identity/internal/service/auth.go` `issue()` for minting.
- Add a migration after `team-identity/migrations/0004_sessions.up.sql` (`refresh_hash`, `prev_refresh_hash`, `refresh_expires_at`, `rotated_at`).
- The gateway forwarder goes in `team-gateway/internal/edge/auth.go`, next to Login. Register the public route wherever Login is registered, and update `unrouted_test.go` and the identity `authz/scopes.go` coverage if they enumerate RPCs.
- Frontend:
  - Cookie names live in `team-frontend/src/lib/gateway/session-cookie.ts`.
  - The refresh goes in `team-frontend/src/middleware.ts`.
  - `safeReturnUrl` goes in `team-frontend/src/features/auth/`.
  - Tests are colocated as `*.test.ts`.
- Do not touch the JWKS and key rotation code.

## Locked
2026-10-14 — Confirmed by Bùi Đăng Phúc. Opaque rotating refresh token (30d sliding, reuse → revoke session, 10s grace), access TTL unchanged 1h, refresh in Next middleware, logout revokes server-side, safeReturnUrl guard; no guest cart merge.

## Amendments
- 2026-10-14 (spec-clarify round 1, confirmed by Bùi Đăng Phúc): grace replay returns a fresh access token and an empty refresh_token (not "the same new pair"); frontend calls RefreshSession without Authorization; middleware runs on Node runtime and sets x-pathname; RevokeSession denylist expiry fixed in identity-session-refresh. Details: openspec/changes/*/design.md Clarifications.
- 2026-10-14 (round 2): absolute session lifetime 90 days from created_at (confirmed by Bùi Đăng Phúc).
