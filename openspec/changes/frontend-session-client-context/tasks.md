## 1. Code track

### team-identity
- [ ] 1.1 Add a client-info parameter (ip, device) to `SessionRepository.RotateRefresh`; in the `s.RefreshHash == presentedHash` branch set `ip = COALESCE(NULLIF($n,''), ip)`, `device = COALESCE(NULLIF($m,''), device)` next to `last_seen`; mirror in `InMemorySessionRepository`. Grace, expired, reuse, revoke branches untouched. (Session client context: rotation records IP via repository, empty client keeps stored values, grace replay does not update, rejected refresh updates nothing)
- [ ] 1.2 `AuthService.RefreshSession` reads `ClientInfo` from ctx (as `recordLogin` does), clips ip to 64 and UA to 256, passes it to `RotateRefresh`. (Session client context: rotation records IP via repository, empty client keeps stored values)
- [ ] 1.3 Unit and repository tests for 1.1-1.2 in `auth_refresh_test.go`, `session_test.go`, `handler/auth_test.go`. (Session client context: rotation records IP via repository, empty client keeps stored values, grace replay does not update, rejected refresh updates nothing, refresh needs no principal and grants nothing)
- [ ] 1.4 docs-sync: team-identity README note that refresh updates session ip/device/last_seen.

### team-frontend
- [ ] 1.5 `refreshSession(refreshToken, client?)`: add `authInterceptor({ client })` (no token). (Session client context: middleware refresh forwards IP and UA, missing headers)
- [ ] 1.6 `middleware.ts` passes `clientContextFrom(req.headers)`; `logoutAction` passes `clientContextFrom(await headers())` to `refreshSession` and `revokeOwnSession`. Serialised with `frontend-cookie-secure` (same hot files). (Session client context: middleware refresh forwards IP and UA, logout refresh and revoke forward IP and UA, refresh failure behaviour unchanged)
- [ ] 1.7 `revokeOwnSession(sessionId, token, client?)` and `sessions.ts revokeSession` forward client context; `listSessions`/`listLoginHistory` do not. (Session client context: revoke another device forwards IP and UA, logout refresh and revoke forward IP and UA)
- [ ] 1.8 Unit tests: `refresh.test.ts`, `middleware.test.ts`, `actions.test.ts`, `sessions.test.ts`, `client-context.test.ts`. (all frontend scenarios)

## 2. E2E track

- [ ] 2.1 Feature in `platform-e2e`: log in with browser UA `AgoraTest/1`, expire the `session` cookie, reload as UA `AgoraTest/2`, open the sessions page, assert device `AgoraTest/2`. (Session client context: rotation updates device and IP)
- [ ] 2.2 `team-frontend/FEATURES.yaml` and `team-identity/FEATURES.yaml` entries (written by e2e-author): the one e2e scenario `green`; every `VERIFIED BY` scenario `not-testable` naming its unit test. (all scenarios)
- [ ] 2.3 `make -C platform-e2e spec-check CHANGE=frontend-session-client-context`.
