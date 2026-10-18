## Why

The web session is a single 1-hour access-token cookie (`session`). A shopper who logged in yesterday is
silently anonymous today, and logout only deletes the cookie, so the server-side session stays live.
Initiative `q1-session-continuity` adds an opaque rotating refresh token (30 d sliding) at identity; this
slice makes the web app use it and makes logout revoke the session server-side.

initiative: q1-session-continuity

## What changes

- `session-cookie.ts` gains `REFRESH_COOKIE = "agora_refresh"`.
- `loginAction` / `registerAction` (`src/features/auth/actions.ts`) store `result.refreshToken` in
  `agora_refresh` (httpOnly, `sameSite: lax`, `path: /`, `maxAge` 30 d = 2592000 s, `secure` set exactly when the
  `session` cookie is secure) next to the access cookie. Empty `refreshToken` (sessions
  disabled) sets no refresh cookie and login still succeeds.
- "Usable" access token means `isUsableToken()` of `lib/gateway/token-expiry.ts`: `exp` minus a 5 s skew is still in the
  future, decoded without signature verification (the gateway is the verifier). `now >= exp - 5 s` is not usable.
- `src/middleware.ts` runs on the Node runtime (`runtime: "nodejs"`, Next 15.5) and reuses the connect-node client; no new
  dependency. When the access cookie is missing or not usable AND `agora_refresh` exists, it calls
  `AuthService.RefreshSession` (public RPC, NO Authorization header, no retry), sets the new cookies on the response and
  injects the new access token into the request seen by the render, so the same render is signed in.
  - Success with a non-empty `refresh_token`: set both cookies.
  - Grace replay (identity returns an empty `refresh_token`): set only the access cookie, keep the existing `agora_refresh`.
  - Unauthenticated (or empty access token): clear both cookies, render anonymous.
  - Any error other than Unauthenticated (timeout, unavailable, internal, ...): render anonymous, keep both cookies unchanged.
- Middleware also sets an `x-pathname` request header (path + query) on every request, consumed by slice
  `frontend-login-return`.
- `logoutAction` reads the `sid` claim of the current access token. When the access token is not usable but
  `agora_refresh` exists, it first calls `RefreshSession` (no bearer) to obtain a fresh access token and its `sid`. It then
  calls `SessionService.RevokeSession({ sessionId })` (existing `revokeSession()` in `lib/gateway/sessions.ts`), clears both
  cookies and redirects to `/`. Any failure along the way never blocks local cookie clearing.
- Hot file: `actions.ts` is also edited by slice `frontend-login-return` (redirect target). The two slices
  are serialised by initiative-build; this slice only touches `setSession`, the token extraction in
  login/register, and `logoutAction`, and leaves the `redirect(...)` argument to the other slice.

## NFR

- Middleware adds at most one gateway call per navigation, and only when access is missing/expired and a
  refresh cookie is present. Requests with a usable access token add zero calls and no latency.
- Refresh call timeout 3 s, no retry; on any error other than Unauthenticated the render is anonymous but cookies are NOT
  cleared (transient outage must not log the shopper out), unlike Unauthenticated, which clears both.
- Refresh token never reaches client JS (httpOnly) and never appears in logs, URLs or analytics.
- Logout p95 unchanged to the shopper in the common case: revoke runs with a 3 s timeout, then cookies clear regardless.
  Refresh-only logout adds at most one more 3 s call (worst case 6 s).

## Data classification

| field | classification | purpose | basis | retention |
|---|---|---|---|---|
| `agora_refresh` cookie (opaque refresh token) | credential, sensitive | keep the shopper signed in | contract (authentication) | 30 d sliding in browser (maxAge reset on each rotation); deleted on logout/failed refresh |
| `session` cookie (access JWT) | credential, sensitive | authenticate gateway calls | contract | unchanged, 1 h |
| `sid` claim read at logout | pseudonymous id | identify session to revoke | contract | not stored by the frontend |

No new data is sent to analytics, logged, or placed in URLs.

## Threat model

- Assets: refresh token (long-lived credential), access token, session identity.
- Abuse cases: (1) XSS reads the refresh token, mitigated by httpOnly. (2) CSRF triggering refresh,
  mitigated by `sameSite: lax` and because refresh only mints cookies for the browser holding the cookie;
  no state-changing effect for an attacker. (3) Stolen/replayed old refresh token: identity detects
  reuse and revokes the session; the middleware then clears cookies on Unauthenticated. (4) Parallel tabs
  racing rotation: identity grace window of 10 s returns a fresh access token for the same sid with an empty
  refresh_token; the middleware keeps the existing cookie, so no false reuse. (5) Logout
  leaving a live server session: closed by RevokeSession.
- Residual risk: on logout with only a refresh cookie, if RefreshSession or RevokeSession fails transiently the cookies
  are cleared locally but the server-side session stays live until its 30 d sliding expiry. A copy of the refresh token
  held by an attacker would stay valid in that case. Accepted: logout must never trap the shopper signed in; no retry
  queue in this slice.
- Authz rule: RefreshSession is public (authorised by possession of the refresh token only); RevokeSession
  requires the caller's own Principal and operates on the caller's own session id.
