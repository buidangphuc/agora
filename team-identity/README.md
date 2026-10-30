# team-identity

Go gRPC service that owns the **identity bounded context**: users and bcrypt credentials, RS256
token issuance, the JWKS endpoint, role-to-scope resolution, shipping addresses, sessions and
password reset. It is the sole token issuer (ADR-0003, ADR-0006): it holds the RSA private key and
publishes only the public key; `team-gateway` verifies tokens against the JWKS. It owns `identity_db`
(Rule 3: no other service connects to it). Status: deployed in the local compose stack, port `:50053`
(gRPC) and `:50063` (HTTP JWKS).

## Contract

gRPC on `GRPC_PORT` (default `50053`). Authorization is enforced in handlers only through
`interceptor.RequirePrincipal`, which reads the gateway-forwarded `x-principal-id` /
`x-principal-type` / `x-principal-scopes` metadata and rejects missing or anonymous principals
with `UNAUTHENTICATED`. Scope gating for the public RPCs is the gateway's job; this service does
not check scopes except in `GetPublicProfiles` (`identity.read`) and `AccountAdminService` (`admin`).

| Service / RPC | Rule in this service |
|---|---|
| `AuthService.Register` | No principal check. Role `seller` or `buyer`; anything else (including `admin`) becomes `buyer`. Username non-empty, password >= 4 chars. Creates a session and returns a token with `sid`. |
| `AuthService.Login` | No principal check. Creates a session, returns token. Wrong credentials: `UNAUTHENTICATED`. |
| `AuthService.ChangePassword` | `RequirePrincipal` and a `PRINCIPAL_TYPE_USER` principal (`PERMISSION_DENIED` for service principals). Always targets the principal's id; `user_id` in the request body is ignored. Verifies the old password. |
| `AuthService.RequestPasswordReset` | No principal check. Returns `expires_at` (15 min TTL, SHA-256 hash stored); the raw reset token is returned in the response only when `PASSWORD_RESET_EXPOSE_TOKEN=true` (dev/e2e; startup refuses it when `ENV=prod`). |
| `AuthService.ResetPassword` | No principal check. Token must exist, be unused and unexpired. |
| `AuthService.RefreshSession` | No principal check; the refresh token is the credential. Rotates the refresh token (10 s grace replay, reuse revokes the session). `PERMISSION_DENIED` for a `SUSPENDED` or `BANNED` account. |
| `AddressService.List/Create/Update/Delete/SetDefault` | `RequirePrincipal`; every query is scoped to the caller's id. |
| `SessionService.ListSessions`, `ListLoginHistory` | `RequirePrincipal`; scoped to the caller. History page size default 20, max 100, cursor is a numeric offset. |
| `SessionService.RevokeSession` | `RequirePrincipal`; only the caller's own session (`NOT_FOUND` otherwise). Writes a `SessionRevoked` outbox row in the same transaction. |
| `PublicProfileService.GetPublicProfiles` | Service principal (`PRINCIPAL_TYPE_SERVICE`) with scope `identity.read` only; otherwise `PERMISSION_DENIED`. Max 100 ids (`InvalidArgument` above). Returns `{user_id, display_name}` where display name is the username. |
| `AccountAdminService.ListUsers` | Scope `admin` (`PERMISSION_DENIED` otherwise); username prefix or exact id, optional status filter. |
| `AccountAdminService.SetUserStatus` | Scope `admin`. Writes a `user.set_status` audit entry to team-audit first (fails `UNAVAILABLE` with no change if it is down). Refuses the caller's own id or another admin (`FAILED_PRECONDITION`). Moving to `RESTRICTED`, `SUSPENDED` or `BANNED` revokes all of the user's sessions. |
| `grpc.health.v1.Health` | Open. |
| `ServerReflection` | Registered when `GRPC_REFLECTION_ENABLED=true`. |

HTTP on `JWKS_HTTP_PORT` (default `50063`): `GET /.well-known/jwks.json`, unauthenticated, serves the
signer's single public key (`kid` = `JWT_KID`), `Cache-Control: public, max-age=300`.

Role to scopes (`internal/authz/scopes.go`):

| Role | Scopes |
|---|---|
| `buyer` | `listing.read`, `search:read`, `search:write`, `engagement:read`, `engagement:write`, `recommendations:read`, `ai:use` |
| `seller` | buyer scopes plus `listing.write` |
| `admin` | seller scopes plus `admin`, `order.admin` |

Token claims (`internal/token/jwt.go`): RS256, header `kid`; `sub`, `name`, `typ` (`user`), `scopes`,
`sid` (session id, omitted if no session), `iat`, `exp` (`JWT_TTL_SECONDS`). `identity.read` is not in
any role, so no user token can carry it, and nothing in this service mints service tokens.

Upstream: team-audit `AuditService.WriteAuditEvent` (`UPSTREAM_AUDIT_ADDR`), used by `SetUserStatus`. Vendored contracts only: `platform.common.v1.Principal`,
`platform.events.v1.EventEnvelope`, `platform.identity.v1.*`.

## Events

| Direction | Topic | Type | Key |
|---|---|---|---|
| Produces | `identity.events` (`IDENTITY_EVENTS_TOPIC`) | `platform.identity.v1.SessionRevoked` in an `EventEnvelope` | user id |
| Produces | `identity.events` | `platform.identity.v1.UserStatusChanged` in an `EventEnvelope` (on every `SetUserStatus` call) | user id |
| Consumes | none | | |

`RevokeSession` inserts the outbox row in the same transaction as `sessions.revoked = true`. The
SDK outbox relayer (`pkg/outbox`) always runs and publishes pending rows to `IDENTITY_EVENTS_TOPIC`
keyed by the user id, with `event-id`, `event-type`, `x-request-id` and `traceparent` headers. A broker
outage never parks a row: transient failures retry without a limit and only a permanent record error
parks one. Published rows older than 7 days (`OUTBOX_PURGE_RETENTION`, checked every
`OUTBOX_PURGE_INTERVAL`) are purged; pending and failed rows are never purged. The envelope carries an expiry of session creation time plus `JWT_TTL_SECONDS`, which is how
the gateway knows how long to deny that session's token.

## Data

Database `identity_db`. Migrations in `migrations/` (golang-migrate, up/down pairs):

| Migration | Tables |
|---|---|
| `0001_users` | `users` (id, username unique, password_hash, roles[], created_at) |
| `0002_addresses` | `user_addresses` |
| `0003_password_reset_tokens` | `password_reset_tokens` (PK = SHA-256 token hash) |
| `0004_sessions` | `sessions`, `login_history` |
| `0005_identity_outbox_events` | `identity_outbox_events` (claimed with `FOR UPDATE SKIP LOCKED`) |
| `0006_session_refresh` | adds `sessions.refresh_hash`, `prev_refresh_hash`, `refresh_expires_at`, `rotated_at` |
| `0007_user_status` | adds `users.status` (`active`, `restricted`, `suspended`, `banned`), `status_reason`, `status_until`, `status_changed_at` |

Retention: `sessions` and `login_history` rows are purged by a background sweeper once older than
`SESSION_RETENTION` (default 30d) and `LOGIN_HISTORY_RETENTION` (default 180d), in batches every
`SESSION_SWEEP_INTERVAL`. Invalid (zero, negative, unparseable) values stop startup.

All user-owned tables reference `users(id)` with `ON DELETE CASCADE`. Migrations are never run by the
service at boot. Run `server migrate up` (the `team-identity-migrate` job in the root compose does
this) before the service starts; standalone use `make migrate`. A dirty `schema_migrations` state
fails the subcommand and needs manual repair.

Deploy order: run `migrate up` first, then roll out the service. Migration `0019` requeues outbox rows
that earlier versions parked as `failed`. To requeue parked rows again by hand:

```sql
UPDATE identity_outbox_events SET status='pending', attempts=0, available_at=now(), locked_until=NULL, error=NULL WHERE status='failed';
```

## Configuration

Read by `internal/config/config.go`. `make check-env` (part of `make check`) fails if `.env.example`
drifts from the config structs. The service has no enable flags: Postgres, Kafka, the outbox relayer
and the session sweeper are always on.

| Variable | Default | Notes |
|---|---|---|
| `ENV` | required | `local`, `test`, `staging` or `production` |
| `LOG_LEVEL` | `info` | |
| `LOG_JSON` | `true` | |
| `GRPC_HOST` | `0.0.0.0` | Also the bind host for JWKS |
| `GRPC_PORT` | required | Compose uses `50053` |
| `GRPC_REFLECTION_ENABLED` | `false` | Set `GRPC_REFLECTION_ENABLED=true` to serve reflection; accepted only when `ENV` is `local` or `test` |
| `SHUTDOWN_GRACE` | `10s` | |
| `SHUTDOWN_DRAIN_DELAY` | `0s` | Wait after health reports NOT_SERVING, before gRPC stops |
| `DATABASE_URL` | empty | Required |
| `DB_MAX_CONNS` | `10` | |
| `KAFKA_BROKERS` | empty | Comma-separated |
| `IDENTITY_EVENTS_TOPIC` | `identity.events` | |
| `OUTBOX_POLL_INTERVAL` | `1s` | Go duration |
| `OUTBOX_BATCH_SIZE` | `100` | |
| `OUTBOX_CLAIM_LOCK` | `60s` | |
| `OUTBOX_PURGE_RETENTION` | `168h` | Published rows older than this (7 days) are purged; must be > 0 |
| `OUTBOX_PURGE_INTERVAL` | `1h` | Must be > 0 |
| `SESSION_RETENTION` | `720h` | Go duration, must be > 0; sessions older than this are purged |
| `LOGIN_HISTORY_RETENTION` | `4320h` | Go duration, must be > 0; login_history rows older than this are purged |
| `SESSION_SWEEP_INTERVAL` | `1h` | Go duration, must be > 0 |
| `SESSION_SWEEP_BATCH_SIZE` | `500` | Rows deleted per batch, must be > 0 |
| `JWT_PRIVATE_KEY` | empty | Required. PEM RSA key (PKCS#1 or PKCS#8); literal `\n` and surrounding quotes are accepted |
| `JWT_KID` | empty | Required |
| `JWKS_HTTP_PORT` | `50063` | |
| `JWT_TTL_SECONDS` | `3600` | Must be > 0 |
| `SEED_ADMIN_ENABLED` | `false` | Opt-in first admin |
| `SEED_ADMIN_USERNAME` | `admin` | |
| `SEED_ADMIN_PASSWORD` | empty | Required, >= 12 chars, when seeding is enabled |
| `PASSWORD_RESET_EXPOSE_TOKEN` | `false` | Return the raw reset token from `RequestPasswordReset`. Dev/e2e only; config load fails when `ENV=prod` |
| `UPSTREAM_AUDIT_ADDR` | empty | Required. team-audit gRPC address; user-status changes fail closed when it is down |
| `LOG_KEY_FP_SECRET` | empty | |
| `OTEL_ENABLED` | `false` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | `.env.example` sets `http://localhost:4317` |
| `OTEL_SERVICE_NAME` | empty | |

Admin seed: with `SEED_ADMIN_ENABLED=true` the user is created only if absent and an existing
password is never overwritten. A missing or short password aborts startup. A seeding failure after
that (for example a DB error) is only logged as a warning. The local compose stack enables it with a
dev password; deployment manifests must not (platform-gitops has a check). Older databases may still
hold an `admin` / `admin123` row from earlier versions that seeded unconditionally: delete or
rotate it (`users.password_hash` is bcrypt). Tokens already issued stay valid until they expire.

## Run locally

Whole stack (service, migrate job, redpanda, shared Postgres), from the repo root:

```bash
docker compose up -d --build team-identity
```

Compose wires `KAFKA_BROKERS`, `JWKS_HTTP_PORT=50063`, a dev signing key and `SEED_ADMIN_*`.
The shared `postgres` container serves `identity_db` in-network as `postgres:5432`.

Standalone, with the service's own Postgres on host port `5435`:

```bash
docker compose -f docker-compose.local.yaml up -d postgres-identity
cp .env.example .env     # then set JWT_PRIVATE_KEY, see below
make proto               # generated/ is gitignored
make migrate             # runs `server migrate up`
make run
```

`.env.example` ships `JWT_PRIVATE_KEY=dev-rsa-private-key-pem-change-me`, which is not a PEM and
makes startup fail. Generate a key with `openssl genrsa 2048` and put it in `.env` as one line with
`\n` escapes.

Smoke checks:

```bash
grpcurl -plaintext localhost:50053 grpc.health.v1.Health/Check
curl -i http://localhost:50063/.well-known/jwks.json
grpcurl -plaintext -d '{"username":"seller_dan","password":"SecurePassword123","role":"seller"}' \
  localhost:50053 platform.identity.v1.AuthService/Register
```

## Build, test and lint

| Command | What it does |
|---|---|
| `make proto` | `buf generate` from the vendored `proto/` into `generated/` (needs `buf`; the plugins are remote on buf.build) |
| `make check` | The merge gate: `check-env`, format, lint (shared `.golangci.yml`), `go vet ./...`, `go test ./...` |
| `make check-env` | `TestEnvExampleInSync` only |
| `make test` | `go test ./...` |

Postgres-backed tests start a throwaway container through the SDK `testkit` and need docker. Treat
`make check` as the gate.

## Spec and verification

`FEATURES.yaml` (121 features: login, register, password reset, addresses, logout, the RS256/JWKS
set including key rotation, and session revocation/device+IP) maps each feature to its platform-e2e
scenario in `covered_by`. Verify with `make -C platform-e2e features-check`, and for an OpenSpec
change `make -C platform-e2e spec-check CHANGE=<id>`. Changes go through OpenSpec
(`openspec/changes/<id>` at the repo root): propose, update `FEATURES.yaml`, add the e2e scenario,
then implement, per the root README's ASDLC. `PublicProfileService` has no `FEATURES.yaml` entry.

## Gotchas

- `generated/` is gitignored; run `make proto` before building or testing.
- `proto/` is vendored from platform-core. Never edit it here.
- The Dockerfile sets `GRPC_PORT=50053` and `EXPOSE`s only `50053`, not the JWKS port. Compose does
  not depend on `EXPOSE`.
- `RevokeSession` only records and relays an event. Enforcement happens at the gateway, which must
  consume `identity.events`; the refresh path also rejects revoked sessions (`NOT revoked` in the lookup), but nothing else reads `sessions.revoked` to gate requests.
- With Kafka unreachable, outbox rows stay pending and are relayed once the broker is back; login and readiness are unaffected.
- Health: `liveness` always serves while the process is up; the empty service name follows Postgres and drops to NOT_SERVING first on shutdown. An unknown service name returns `NOT_FOUND`.
- JWKS publishes exactly one key (the signer's). `BuildJWKS` accepts several, but `internal/app` passes one,
  so rotation means redeploying with a new `JWT_KID` and key.
- In-memory repositories exist for tests only; the server always uses Postgres.

## Known gaps

- `sessions.last_seen` is set at creation; refresh rotation also updates the session ip, device and last_seen from the request client context. An empty client context on refresh keeps the stored ip and device unchanged; no other call updates last_seen.
- Identity trusts `x-principal-*` metadata without verification (ADR-0010 interim: NetworkPolicy only).
  Anything that can reach `:50053` directly can impersonate any principal.
- There is no out-of-band delivery (email/SMS) of the reset token, so with the default
  `PASSWORD_RESET_EXPOSE_TOKEN=false` a caller of `RequestPasswordReset` never receives it. With it
  `true` (dev/e2e) the RPC allows takeover of any account by username.
- Unknown usernames in `RequestPasswordReset` return `NOT_FOUND`, which allows username enumeration; the
  same applies to `Register` conflicts (`ALREADY_EXISTS`).
- Display name is the username; there is no separate profile field.

## Links

- Root rules: [`../AGENTS.md`](../AGENTS.md)
- ADRs in `../platform-core/docs/ADR/`: `0002-async-broker.md`, `0003-auth-model.md`,
  `0006-rs256-jwks-auth.md`, `0010-service-zero-trust.md`
