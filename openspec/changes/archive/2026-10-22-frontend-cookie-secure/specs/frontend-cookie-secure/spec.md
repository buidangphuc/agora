## ADDED Requirements

### Requirement: COOKIE_SECURE forces the Secure attribute

`isSecureRequest(headers)` SHALL return true when the server-only env `COOKIE_SECURE`, read at call time, equals
`true` (case-insensitive, trimmed); otherwise it SHALL return true exactly when `x-forwarded-proto` is `https`.
Every cookie the frontend sets for authentication (`session`, `agora_refresh`, `agora_auth_evt`) SHALL take its
`secure` flag from it.

#### Scenario: Cookie secure: forced on without proxy header
- **WHEN** `COOKIE_SECURE=true` and a login succeeds on a request with no `x-forwarded-proto`
- **THEN** every `Set-Cookie` for `session`, `agora_refresh` and `agora_auth_evt` SHALL carry the `Secure` attribute
- **VERIFIED BY**: `team-frontend/src/lib/gateway/session-cookie.test.ts` and `src/features/auth/actions.test.ts` (unit; the e2e stack runs with the variable unset by design)

#### Scenario: Cookie secure: forced on beats a client header (permission)
- **WHEN** `COOKIE_SECURE=true` and the request carries `x-forwarded-proto: http`
- **THEN** `isSecureRequest` SHALL return true and the cookies SHALL be `Secure`
- **VERIFIED BY**: `team-frontend/src/lib/gateway/session-cookie.test.ts` (unit)

#### Scenario: Cookie secure: unset keeps the proxy rule
- **WHEN** `COOKIE_SECURE` is unset and a login succeeds on a request with `x-forwarded-proto: https`
- **THEN** the system SHALL set `session`, `agora_refresh` and `agora_auth_evt` with the `Secure` attribute

#### Scenario: Cookie secure: unset and plain http (boundary)
- **WHEN** `COOKIE_SECURE` is unset and a login succeeds on a request with no `x-forwarded-proto`
- **THEN** the system SHALL set all three cookies without the `Secure` attribute, so local http login keeps working

#### Scenario: Cookie secure: non-true values are ignored (boundary)
- **WHEN** `COOKIE_SECURE` is `false`, `1`, `yes` or the empty string and the request has no `x-forwarded-proto: https`
- **THEN** `isSecureRequest` SHALL return false
- **VERIFIED BY**: `team-frontend/src/lib/gateway/session-cookie.test.ts` (unit; parametrised)

#### Scenario: Cookie secure: value is case and space tolerant (boundary)
- **WHEN** `COOKIE_SECURE` is ` TRUE ` and the request has no `x-forwarded-proto`
- **THEN** `isSecureRequest` SHALL return true
- **VERIFIED BY**: `team-frontend/src/lib/gateway/session-cookie.test.ts` (unit)

#### Scenario: Cookie secure: flag read at request time
- **WHEN** `COOKIE_SECURE` changes in `process.env` between two calls in one process
- **THEN** the second call SHALL reflect the new value (no module-level caching, no build-time inlining)
- **VERIFIED BY**: `team-frontend/src/lib/gateway/session-cookie.test.ts` (unit)

### Requirement: Silent refresh honours COOKIE_SECURE

The middleware that rotates `session` and `agora_refresh` SHALL use `isSecureRequest` for both cookies it sets.

#### Scenario: Cookie secure: middleware refresh forced Secure
- **WHEN** `COOKIE_SECURE=true`, a request has an expired `session` and a valid `agora_refresh`, and `RefreshSession` succeeds without `x-forwarded-proto`
- **THEN** the response SHALL set the new `session` and rotated `agora_refresh` both with `Secure`
- **VERIFIED BY**: `team-frontend/src/middleware.test.ts` (unit)

#### Scenario: Cookie secure: middleware refresh unset over https
- **WHEN** `COOKIE_SECURE` is unset, `x-forwarded-proto: https` is present and a silent refresh succeeds
- **THEN** the response SHALL set `session` and `agora_refresh` with `Secure`
- **AND** the same refresh without the header SHALL set neither with `Secure`

#### Scenario: Cookie secure: failed refresh sets nothing (error)
- **WHEN** `COOKIE_SECURE=true` and `RefreshSession` fails with Unauthenticated
- **THEN** the system SHALL delete `session` and `agora_refresh` as before, setting no cookie with a value
- **VERIFIED BY**: `team-frontend/src/middleware.test.ts` (unit)

### Requirement: Auth-event cookie keeps its other attributes

`agora_auth_evt` SHALL stay readable by script and short-lived.

#### Scenario: Cookie secure: auth event attributes unchanged
- **WHEN** login or sign_up succeeds under any `COOKIE_SECURE` value
- **THEN** `agora_auth_evt` SHALL have value `login` or `sign_up`, `maxAge` 60, `sameSite` lax, `path` `/`, not `HttpOnly`, differing from before only in `Secure`
- **VERIFIED BY**: `team-frontend/src/features/auth/actions.test.ts` (unit)

#### Scenario: Cookie secure: failed login sets no auth event (error)
- **WHEN** `AuthService.Login` fails with Unauthenticated
- **THEN** the system SHALL set no `agora_auth_evt` cookie
- **VERIFIED BY**: `team-frontend/src/features/auth/actions.test.ts` (unit)

### Requirement: Deployed envs enable it, local does not

The gitops values for staging and prod SHALL set `COOKIE_SECURE` to `true`, and local compose and local values SHALL NOT define it.

#### Scenario: Cookie secure: staging and prod set it
- **WHEN** the gitops values for `team-frontend` are rendered for staging and for prod
- **THEN** the container env SHALL contain `COOKIE_SECURE: "true"` in both, and a missing key SHALL fail the manifest test
- **VERIFIED BY**: `platform-gitops/tests/test_frontend_cookie_secure_manifests.py` (manifest test)

#### Scenario: Cookie secure: local compose leaves it unset (boundary)
- **WHEN** `docker-compose.services.yaml`, `platform-e2e/docker-compose.e2e.yaml` and `platform-gitops/envs/local/values.yaml` are read
- **THEN** none SHALL define `COOKIE_SECURE`, and `team-frontend/.env.example` SHALL document it commented out or set to `false`
- **VERIFIED BY**: `platform-gitops/tests/test_frontend_cookie_secure_manifests.py` (manifest test)
