## Why

Q1 put device, IP and refresh hashes on `sessions`, and Q2 stores IP and user agent in `login_history`. Nothing deletes them: revoked or long-expired sessions and old login history live forever. The initiative (`plans/privacy-audit-hardening/00-spec.md`, Clarifications Q1 and Q2) fixes this with a retention sweeper inside team-identity.

initiative: privacy-audit-hardening

## What Changes

- **team-identity**
  - New `SessionSweeper` (package `internal/service` or `internal/events` next to the relayer, same Run/SweepOnce shape as team-domain `ReservationSweeper`), started as a ticker addon from `internal/bootstrap/lifecycle.go` and drained in `CloseResources` before the pool closes.
  - Migration `0008_session_revoked_at.{up,down}.sql`: nullable `sessions.revoked_at TIMESTAMPTZ`, plus a partial index supporting the purge. Every revoke path in `repository/session.go` (`RevokeSession`, reuse revoke, lifetime-exceeded revoke) sets `revoked_at = now`. Today a revoke leaves no timestamp, so "30 days after revoke" cannot be computed without it.
  - `SessionRepository` gains `PurgeSessions(ctx, cutoff, limit)` and `PurgeLoginHistory(ctx, before, limit)` (Postgres and in-memory); each call deletes at most `limit` rows in one statement/transaction and returns the count.
  - A session is purge-eligible when it is NOT active and its end timestamp is older than `SESSION_RETENTION`:
    - revoked: end = `COALESCE(revoked_at, last_seen)` (legacy revoked rows without `revoked_at` fall back to `last_seen`);
    - not revoked: end = `LEAST(COALESCE(refresh_expires_at, created_at + 90 days), created_at + 90 days)` (refresh expiry or the 90-day absolute max age, whichever is earlier; legacy rows without a refresh expiry use the absolute max age).
  - `login_history` rows with `created_at` older than `LOGIN_HISTORY_RETENTION` are deleted.
  - Config (`internal/config/config.go`, `.env.example`, README table): `SESSION_RETENTION` (Go duration, default `720h` = 30 d), `LOGIN_HISTORY_RETENTION` (default `4320h` = 180 d), `SESSION_SWEEP_INTERVAL` (default `1h`), `SESSION_SWEEP_BATCH_SIZE` (default `500`), `SESSION_SWEEP_ENABLED` (default `true`). Non-positive or unparsable durations and non-positive batch size fail config load (a silent fallback could mean never purging or purging immediately).
- **platform-e2e**: `platform-e2e/compose/session-retention.override.yaml` sets `SESSION_RETENTION=5s`, `LOGIN_HISTORY_RETENTION=5s`, `SESSION_SWEEP_INTERVAL=2s` on `team-identity` (keys by service name, like `order-inventory.override.yaml`).
- Not changed: proto, access/refresh token lifetimes, `SessionRevoked` events, `ListSessions` and `ListLoginHistory` contracts, audit rows in team-audit (DPO item).

## NFR

- Cadence: one sweep every `SESSION_SWEEP_INTERVAL` (1 h); a sweep loops batches of 500 until a batch returns fewer than 500 rows or the context ends. One batch per transaction, so locks are short.
- Active sessions (not revoked, effective expiry in the future) are never selected, locked or modified. The purge selects candidate ids with `FOR UPDATE SKIP LOCKED` so it never blocks a concurrent `RefreshSession` rotation.
- p99 `RefreshSession` latency unchanged (+/- 5 ms) while a sweep runs.
- Availability: a sweep error is logged and retried next tick; it never affects serving or readiness. Idempotent; a missed or doubled tick is harmless.
- Limits: retention minimum is 1 s (validated), batch size 1..10000.

## Data classification

| Field | Purpose | Basis | Retention |
|---|---|---|---|
| `sessions.device`, `sessions.ip` | Account Safety Center, abuse detection | Legitimate interest, security (DPO to confirm, human item) | Deleted 30 d after revoke or refresh expiry (Q1) |
| `sessions.refresh_hash`, `prev_refresh_hash` | Refresh token verification and reuse detection | Contract / security | Cleared at revoke already; row deleted 30 d after revoke or expiry |
| `sessions.revoked_at` (new) | Retention clock | Security | Same row lifetime |
| `login_history.ip`, `user_agent` | Login audit shown to the user, abuse detection | Legitimate interest, security (DPO to confirm) | Deleted after 180 d (Q2) |

No new personal data is collected.

## Threat model

- Assets: session and login-history rows (IP, device, UA, refresh hashes).
- Abuse cases: (1) the sweeper deletes a live session and logs a user out: guarded by the eligibility predicate plus tests for active rows; (2) a mis-set env (`SESSION_RETENTION=0`) wipes all revoked sessions or disables purging: rejected at config load; (3) the sweeper holds locks and stalls refresh: `SKIP LOCKED`, small batches; (4) deleting a revoked session removes reuse detection for its old tokens: hashes are already cleared on revoke, and after 30 days a replayed token is an unknown token, `Unauthenticated`, same as before; (5) deleting rows removes forensic evidence: audit copies stay in team-audit.
- Authz: no RPC is added; the sweeper is an internal process with the service's own DB credentials. No Principal involved.
