## Why

Access tokens live 1 hour (`JWT_TTL_SECONDS`), so every shopper is hard-logged-out hourly: `AuthService.issue()` in `team-identity/internal/service/auth.go` mints only an access token and there is no way to renew it. The initiative spec (`plans/q1-session-continuity/00-spec.md`) fixes this with an opaque, rotating refresh token stored on the existing `sessions` row. This slice builds the identity-side behaviour and the gateway forwarder. The proto contract (`AuthService.RefreshSession`, `AuthResult.refresh_token` = 4, `refresh_expires_at` = 5) is owned by `q1-session-refresh-contract`; the frontend consumer is `frontend-session-continuity`.

initiative: q1-session-continuity

## What Changes

- **team-identity**
  - Migration `0006_session_refresh` adds `refresh_hash`, `prev_refresh_hash`, `refresh_expires_at`, `rotated_at` to `sessions`, with a unique index on `refresh_hash` and a plain index on `prev_refresh_hash`.
  - `Login` and `Register` mint a refresh token (random 32 bytes from `crypto/rand`, base64url; no HMAC, no pepper, no rotation counter) next to the access token and return it in `AuthResult.refresh_token` / `refresh_expires_at` (now + 30 days). Only the SHA-256 hash is stored. When sessions are disabled (`s.sessions == nil`) both fields stay empty.
  - New `AuthService.RefreshSession`: looks up the session by hash, rotates the token, extends the expiry to now + 30 days, keeps the same `sid`, updates `last_seen`, returns a new access token plus the new refresh token. Absolute lifetime: once `now > created_at + 90 days` the call returns `Unauthenticated` (generic message) and revokes the session, regardless of the sliding expiry.
  - Reuse detection: presenting the previous hash after the 10 s grace window revokes the session and publishes `SessionRevoked` through the outbox (same transaction, as `SessionService.RevokeSession` does today). Inside the grace window (10 s, inclusive) a replay of the immediately previous token returns a fresh access token for the same `sid` with an empty `refresh_token` (the client keeps its current cookie); no rotation happens. A token older than one rotation gets `Unauthenticated` without revoking. The grace replay also loads the current user (scopes re-derived). If the owning user no longer exists (rotation or grace path), `Unauthenticated` and the session is revoked. Revoke clears both hashes, so every token of a revoked session, including a prev-hash replay inside the grace window, takes the unknown-token path (`Unauthenticated`). If the atomic revoke plus outbox write fails, the call returns `Internal`.
  - Unknown, expired, revoked or reused token: `Unauthenticated` with one generic message (no oracle).
  - `authz/scopes.go` coverage updated so `RefreshSession` is classified as public (no Principal), like `Login`.
- **team-gateway**
  - `AuthForwarder.RefreshSession` in `internal/edge/auth.go`, public, forwarded once through `callWrite` (no retry).
  - No bearer-skip exception: the frontend sends no `Authorization` header on `RefreshSession`. `unrouted_test.go` updated for the new procedure.
- **Fix in this slice:** `SessionService.RevokeSession` (handler layer, `team-identity/internal/handler/session.go`) sets the denylist `expires_at` = now + access TTL (today it is wrong).
- Not changed: JWKS and key rotation, access-token TTL, signing scheme, `SessionService`, `SessionRevoked` schema.

## NFR

- `RefreshSession` p95 < 100 ms at identity (non-gating target, not a pass/fail criterion; measured in the identity service test harness against local Postgres with 20 concurrent callers; one indexed lookup + one update + one RS256 signature).
- One gateway round trip per call; never retried (a retry of a rotation would look like reuse).
- Availability: identity unavailable -> `Unavailable` surfaces to the caller unchanged; the session is not revoked and the refresh token stays valid.
- Limits: refresh token max 256 bytes on input; empty or oversized -> `Unauthenticated` with the same generic message as an unknown token (never `InvalidArgument`; matches `q1-session-refresh-contract`). Grace window 10 s, refresh lifetime 30 d sliding, absolute session lifetime 90 d from `created_at` (constants, not env).
- The gateway edge rate limiter applies to the anonymous bucket as for Login: per-client-IP token bucket from `team-gateway/internal/edge/interceptors.go`, configured by `RATE_LIMIT_RPS` (default 20/s refill) and `RATE_LIMIT_BURST` (default 40, so N = 40); excess calls get `ResourceExhausted`.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `sessions.refresh_hash` / `prev_refresh_hash` (SHA-256 of an opaque secret) | credential-derived | authenticate a returning shopper | contract (account service) | until `refresh_expires_at` (30 d sliding); `prev_refresh_hash` overwritten on next rotation; both cleared (set NULL) when the session is revoked |
| `sessions.refresh_expires_at`, `rotated_at` | session metadata | expiry and grace-window decision | contract | life of the session row |
| raw `refresh_token` | credential | delivered once to the client | contract | never stored, never logged |
| device / IP on the session | personal | already stored per session, unchanged | unchanged | unchanged |

The raw token and its hash MUST NOT appear in logs, traces, metrics labels or events. No new data goes to analytics.

## Threat model

- **Assets:** refresh token (30-day bearer credential), session row, `sid` denylist.
- **Abuse cases:**
  - Stolen refresh token replayed after the legitimate client rotated it -> reuse detection revokes the whole session.
  - Brute force or guessing -> 256 bits of entropy, hash-only storage, gateway rate limit.
  - Token enumeration -> every failure is the same `Unauthenticated`; the design intent is one indexed lookup on every path so timing does not distinguish causes (not a tested guarantee).
  - Refresh of a session the user revoked (logout or Safety Center) -> `Unauthenticated`.
  - Race between two tabs -> grace replay returns an access token only, so no false revoke.
  - Stolen R1 replayed within 10 s of rotation: accepted residual risk. The replay yields only a fresh access token for the same `sid`, never the refresh chain; once the attacker's access token lapses (1 h) it cannot be renewed, and any later replay of R1 revokes the session.
  - Stolen token used for months by sliding renewal -> absolute 90-day lifetime from `created_at` forces re-login and revokes the session.
  - Replay of a prev token after logout -> revoke cleared both hashes, so it is an unknown token: `Unauthenticated`.
  - Refresh for a deleted user -> `Unauthenticated` and the session is revoked.
  - DB leak -> only SHA-256 hashes of 256-bit secrets.
- **Authz rule:** `RefreshSession` is public; possession of a live refresh token is the credential. It can only return a token for the session that owns the hash, with that session's user and current roles (scopes re-derived from the user row, not copied from the old token).
