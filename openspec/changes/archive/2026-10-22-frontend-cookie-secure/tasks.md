## 1. Code track

Shares `src/middleware.ts` and `src/features/auth/actions.ts` with frontend-session-client-context: rebase on it,
touch only the `secure` computation and `AUTH_EVT_OPTS`.

- [x] 1.1 `session-cookie.ts`: `isSecureRequest` returns true when `process.env.COOKIE_SECURE` trimmed lowercased is `true`, else the `x-forwarded-proto` rule; read per call (Scenarios: "Cookie secure: forced on without proxy header", "forced on beats a client header", "unset keeps the proxy rule", "unset and plain http", "non-true values are ignored", "value is case and space tolerant", "flag read at request time")
- [x] 1.2 `actions.ts`: replace constant `AUTH_EVT_OPTS` with a function taking `secure`; call it at the login and sign_up sites using `isSecureRequest(await headers())` (Scenarios: "forced on without proxy header", "auth event attributes unchanged", "failed login sets no auth event")
- [x] 1.3 `middleware.ts`: confirm line using `isSecureRequest(req.headers)` covers both rotated cookies, no other change (Scenarios: "middleware refresh forced Secure", "middleware refresh unset over https", "failed refresh sets nothing")
- [x] 1.4 Unit tests: extend `session-cookie.test.ts`, `actions.test.ts`, `middleware.test.ts` for every non-e2e scenario above
- [x] 1.5 `team-frontend/.env.example`: add commented `# COOKIE_SECURE=true` with a note (server-only, deployed TLS envs, never on plain http); `README.md`: list the var, remove the x-forwarded-proto known gap (Scenario: "local compose leaves it unset")
- [x] 1.6 `platform-gitops/envs/staging/services/team-frontend.yaml` and `envs/prod/services/team-frontend.yaml`: add `env: {COOKIE_SECURE: "true"}`; verify the chart merges it with base `env` (GATEWAY_URL, JWT_SECRET kept); add `platform-gitops/tests/test_frontend_cookie_secure_manifests.py` (Scenarios: "staging and prod set it", "local compose leaves it unset")
- [x] 1.7 Do not add `COOKIE_SECURE` to `docker-compose.services.yaml`, `platform-e2e/docker-compose.e2e.yaml` or `envs/local/values.yaml` (Scenario: "local compose leaves it unset")

## 2. E2E track

- [x] 2.1 e2e: login through the e2e stack (variable unset) with no forwarded header; assert `session`, `agora_refresh`, `agora_auth_evt` have no Secure flag and login succeeds (Scenario: "unset and plain http")
- [x] 2.2 e2e: same login with request header `x-forwarded-proto: https`; assert all three cookies are Secure (Scenarios: "unset keeps the proxy rule", "middleware refresh unset over https" via an expired-session reload)
