## MODIFIED Requirements

### Requirement: Login and Register issue an opaque refresh token

`AuthService.Login` and `AuthService.Register` SHALL always create a session row and return in `AuthResult` a `refresh_token` of 32 random bytes encoded base64url (43 characters) and `refresh_expires_at` equal to issue time plus 30 days. The service SHALL store only the SHA-256 hash of the token on the session row. A service SHALL NOT be constructible without its session store, so there is no sessionless login.

#### Scenario: Login returns a refresh token
- **WHEN** a user logs in with valid credentials
- **THEN** `AuthResult.refresh_token` is a 43-character base64url string and `refresh_expires_at` is 30 days (+/- 5 s) after now
- **VERIFIED BY**: team-identity `internal/identity` `TestLoginIssuesRefreshToken`

#### Scenario: Register returns a refresh token
- **WHEN** a new buyer registers
- **THEN** `AuthResult.refresh_token` is non-empty and its session row holds a hash that differs from the token
- **VERIFIED BY**: team-identity `internal/identity` `TestRegisterIssuesRefreshToken`

#### Scenario: Only the hash is persisted
- **WHEN** a refresh token is issued and the session row is read back
- **THEN** `sessions.refresh_hash` equals the hex SHA-256 of the token and the raw token appears in no column of the row
- **VERIFIED BY**: team-identity `internal/store` `TestRefreshHashOnlyStored`

#### Scenario: Token and hash never reach logs, spans or outbox
- **WHEN** Login, a rotation, a grace replay and a reuse revocation run with log, span and outbox capture enabled
- **THEN** zero captured log lines, span attributes and outbox payloads contain the raw token or its hash
- **VERIFIED BY**: team-identity `internal/rpc` `TestRefreshSecretsNeverLogged`

#### Scenario: Sessions disabled yields no refresh token
- **WHEN** `NewAuthService` receives a nil session store, and in a second case a nil audit gate
- **THEN** each call returns an error and no service value
- **VERIFIED BY**: team-identity `internal/identity` `TestNewAuthServiceRequiresSessionStoreAndGate`

#### Scenario: Failed login issues nothing
- **WHEN** a login fails with a wrong password
- **THEN** the call returns `Unauthenticated` and no session row is created
- **VERIFIED BY**: team-identity `internal/identity` `TestFailedLoginCreatesNoSession`
