## Why

Login and register forward the browser's IP and user agent (`clientContextFrom` + `authInterceptor({client})`), so
a new session records the real device. Refresh and revoke do not: `refreshSession()` (middleware, `logoutAction`)
and `revokeOwnSession()` / `sessions.ts revokeSession()` build bare transports, so the gateway sees the frontend
container as the peer. Identity also ignores the client context on `RefreshSession`, and `RotateRefresh` never
touches `ip` / `device`, so the "Active sessions" page shows the login-time device forever and never the device
that actually keeps the session alive. Q3 (human, "default"): update ip, device and last_seen on rotation.

initiative: privacy-audit-hardening

## What changes

team-frontend (code only, no proto change):
- `src/lib/gateway/refresh.ts` `refreshSession(refreshToken, client?)` adds `authInterceptor({ client })` to its
  transport (still no bearer). Callers pass `clientContextFrom(headers)`: `src/middleware.ts` (request headers) and
  `logoutAction` (`await headers()`).
- `src/lib/gateway/revoke.ts` `revokeOwnSession(sessionId, token, client?)` and the `revokeSession` server action path
  in `src/lib/gateway/sessions.ts` forward the same client context. `listSessions` / `listLoginHistory` are unchanged.
- IP derivation is unchanged: `clientContextFrom` takes the entry `TRUSTED_PROXY_HOPS` from the right of
  `X-Forwarded-For`, never the leftmost; invalid IPs are dropped; UA clipped to 256.

team-identity (no proto change; handler already calls `withClient(ctx)` for `RefreshSession`):
- `AuthService.RefreshSession` reads the `ClientInfo` from ctx and passes it to `SessionRepository.RotateRefresh`.
- On outcome `RotateRotated` only, the rotation `UPDATE` also sets `ip = <client ip, clipped 64>` and
  `device = <client UA, clipped 256>` (in addition to `last_seen = now`, already set). An empty forwarded value keeps
  the stored value (never blanks it). Grace replay, expired, reused, revoked and lifetime-exceeded outcomes change
  neither ip nor device nor last_seen. Both Postgres and in-memory repositories implement it.
- `SessionService.RevokeSession` is unchanged: it records no IP/UA, so forwarding on revoke is consistency and
  correct peer attribution at the gateway (rate limit and logs), not a stored value.

## NFR

- No new RPC, no extra round trip. p99 `RefreshSession` latency unchanged (+-5 ms): the same single-row `UPDATE`
  gains two columns.
- Middleware refresh keeps its 3 s timeout and failure semantics (S9/S10 of `frontend-session-continuity`).
- Header size bound: UA clipped to 256 at frontend and identity, IP to 64.

## Data classification

| field | purpose | consent / basis | retention |
|---|---|---|---|
| `sessions.ip` (updated on rotation) | security: show the user where the session is used, spot hijack | legitimate interest (security), DPO sign-off pending (Q1 item) | with the session row; purged 30 days after revoke/refresh expiry by `identity-session-retention` |
| `sessions.device` (UA, updated on rotation) | same | same | same |
| `sessions.last_seen` | same | same | same |
| `X-Forwarded-For` / `User-Agent` on refresh and revoke calls | transport of the above | same | not stored by frontend or gateway; gateway stamps `x-client-ip` / `x-client-user-agent` per call only |

No new personal field: refresh now refreshes values login already stores. Overwriting means the previous IP/device
of a session is lost (the login-time value survives in `login_history`, retention 180 days).

## Threat model

- Assets: the session row's ip/device (shown to the user, used to judge a hijack), the integrity of that signal.
- Abuse: (1) a client sends a forged `X-Forwarded-For` to make the sessions page show a false IP or to hide a stolen
  session. Mitigations: frontend derives the IP from the proxy-appended entry (`TRUSTED_PROXY_HOPS` from the right),
  never the leftmost; the gateway honours `X-Forwarded-For` only from its `TRUSTED_PROXIES` (the frontend) and builds
  `x-client-*` itself, ignoring any inbound `x-client-*`; a chain shorter than the hop count forwards no IP. Values
  are audit-only, never used for authorization. (2) A forged User-Agent: harmless (client controls it anyway), clipped.
  (3) An attacker holding a stolen refresh token rotates it from their own IP, which then displays, which is the
  desired signal (the legitimate user's next refresh hits reuse detection). (4) Direct gateway callers (no trusted
  proxy) cannot set the IP; the gateway uses its socket peer.
- Authz: refresh is public (the refresh token is the credential); revoke requires the caller's own principal and
  session ownership, unchanged. This slice adds no new authority.
