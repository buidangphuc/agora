## 1. Code track (team-frontend)

Depends on identity-session-refresh; regenerate `src/generated` from the proto that contains RefreshSession first.
`actions.ts` is shared with frontend-login-return: rebase on it and do not change its `redirect()` targets.

- [x] 1.1 Add `REFRESH_COOKIE` and shared cookie-option helpers (access 3600 s, refresh 2592000 s) to `src/lib/gateway/session-cookie.ts` (S1, S2)
- [x] 1.2 Extend `setSession` in `actions.ts` to set `agora_refresh` from `result.refreshToken`, skip when empty; use it in login and register (S1-S4)
- [x] 1.3 Add `refreshSession(refreshToken)` helper on the connect-node client (public, no Authorization header, 3 s timeout, no retry) in `src/lib/gateway/` (S10, S12)
- [x] 1.4 Middleware (`runtime: "nodejs"`): refresh when access missing/unusable per `isUsableToken` and refresh cookie present; set cookies on response, forward new access token to the render; empty `refresh_token` sets only access cookie; clear both on Unauthenticated/empty access; leave cookies untouched on any other error; set `x-pathname` (path+query) on every request (S5-S12, S19-S21, S23)
- [x] 1.5 `logoutAction`: decode `sid` from the usable access token; if unusable and refresh cookie exists, call `refreshSession` first to get the sid; call `revokeSession` with a 3 s timeout, swallow any failure, delete both cookies, redirect `/` (S13-S15, S17, S24, S25)
- [x] 1.6 Unit tests colocated: `actions.test.ts` (S1-S4, S13-S15, S17, S24, S25), `middleware.test.ts` (S5-S12, S19-S21, S23)

## 2. E2E track

- [x] 2.1 e2e: login, expire/remove `session` cookie, reload a page and assert signed-in render and rotated cookies (S5, S6)
- [x] 2.2 e2e: usable access token produces no refresh call; no refresh cookie stays anonymous (S7, S8)
- [x] 2.3 e2e: revoked/garbage `agora_refresh` clears both cookies, anonymous render (S9, S11); gateway outage keeps cookies (S10)
- [x] 2.4 e2e: logout then replay the old refresh cookie is rejected (S13, S16); revoke outage still logs out locally (S14); logout with only the refresh cookie revokes the session (S15, S24)
- [x] 2.5 e2e: login/register cookie attributes (S1-S3)
- [x] 2.6 e2e: two parallel requests with the same `agora_refresh` and no `session` both render signed in and the session is not revoked (S18, S19)
- [x] 2.7 e2e: `x-pathname` reaches the render with path and query (S23, S26)
