# team-identity — Identity & Authentication Microservice

`team-identity` is the authentication, authorization, and user identity authority in the marketplace polyrepo. It manages user accounts, bcrypt credential validation, password management & recovery, user shipping address books, and issues signed **HS256 JWT** access tokens.

In accordance with **ADR-0003**, the **Gateway** (`team-gateway`) is the sole edge verifier for JWTs. Downstream services never see credentials or raw passwords; they receive a trusted `platform.common.v1.Principal` propagated via gRPC metadata headers (`x-principal-*`).

---

## 1. Responsibilities & Architecture

- **User Authentication**: Account registration and login with bcrypt password hashing (`DefaultCost`).
- **Token Minting**: Generates signed HS256 JWTs carrying `sub` (user_id), `username`, `principal_type`, `scopes`, and expiration timestamp.
- **RBAC & Scopes (Basic Authz)**: Resolves user roles (`admin`, `seller`, `buyer`) into service permissions/scopes.
- **Password Lifecycle Management**: Authenticated password changes and token-based self-service password reset flows (SHA-256 hashed 15-minute expiration tokens).
- **Address Book Management**: Full CRUD operations for buyer shipping addresses with automatic default address tracking.
- **Database Ownership (Rule 3)**: Exclusively owns `identity_db` (Postgres port `5435`). No other service connects to this database.

---

## 2. Configuration & Environment Variables

Configuration is loaded from environment variables using struct tags (`internal/config`), guarded against drift by `TestEnvExampleInSync`.

| Variable | Type | Default | Description |
|---|---|---|---|
| `ENV` | `string` | `local` | Environment mode (`local`, `dev`, `prod`) |
| `LOG_LEVEL` | `string` | `info` | Logging verbosity (`debug`, `info`, `warn`, `error`) |
| `LOG_JSON` | `bool` | `true` | Log format in JSON format |
| `GRPC_HOST` | `string` | `0.0.0.0` | Bind host for gRPC server |
| `GRPC_PORT` | `int` | `50053` | gRPC listening port |
| `GRPC_REFLECTION_ENABLED` | `bool` | `true` | Enable gRPC Server Reflection |
| `SHUTDOWN_GRACE_SECONDS` | `float` | `10` | Grace period for server drain |
| `DATABASE_ENABLED` | `bool` | `true` | Enable Postgres database pool |
| `DATABASE_URL` | `string` | `""` | Connection string (`postgresql://identity_svc:identity_pass@localhost:5435/identity_db`) |
| `DB_MAX_CONNS` | `int32` | `10` | Maximum connections in pgxpool |
| `JWT_SECRET` | `string` | `""` | Secret key for HS256 signing (must match `team-gateway`) |
| `JWT_TTL_SECONDS` | `int` | `3600` | Access token lifespan in seconds (default 1 hour) |
| `OTEL_ENABLED` | `bool` | `false` | Enable OpenTelemetry tracing exporter |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `string` | `""` | OTLP gRPC endpoint (`localhost:4317`) |
| `OTEL_SERVICE_NAME` | `string` | `team-identity` | Service name in distributed traces |

---

## 3. Database Schema & Migrations

Database migrations are located in `migrations/` and managed with `golang-migrate`.

### `0001_users.up.sql`
```sql
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    username      TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    roles         TEXT[]      NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### `0002_addresses.up.sql`
```sql
CREATE TABLE IF NOT EXISTS user_addresses (
    id             TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_name TEXT NOT NULL,
    phone          TEXT NOT NULL,
    street         TEXT NOT NULL,
    ward           TEXT NOT NULL DEFAULT '',
    district       TEXT NOT NULL DEFAULT '',
    city           TEXT NOT NULL,
    is_default     BOOLEAN NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS user_addresses_user_idx ON user_addresses (user_id);
```

### `0003_password_reset_tokens.up.sql`
```sql
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    token_hash  TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used        BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS password_reset_tokens_user_idx ON password_reset_tokens (user_id);
```

---

## 4. RBAC Roles & Scopes

The `internal/authz` package defines the role-to-scope mapping:

| Role | Granted Scopes | Notes |
|---|---|---|
| `admin` | `listing.read`, `listing.write`, `search:read`, `engagement:read`, `engagement:write`, `admin` | Seeded via `EnsureAdmin` (`admin`/`admin123`) |
| `seller` | `listing.read`, `listing.write`, `search:read`, `engagement:read`, `engagement:write` | Self-assignable during registration |
| `buyer` | `listing.read`, `search:read`, `engagement:read`, `engagement:write` | Default self-assignable role |

---

## 5. gRPC Services & RPC Contracts

### 5.1 `platform.identity.v1.AuthService`

Public authentication endpoints (no principal header required to obtain token).

#### `Register`
- **Request**: `RegisterRequest { username, password, role }` (role: `"buyer"` or `"seller"`, minimum password length 4).
- **Response**: `RegisterResponse { result: AuthResult { token, principal, username } }`.
- **Status Codes**: `OK`, `InvalidArgument` (empty username / short password), `AlreadyExists` (username taken).

#### `Login`
- **Request**: `LoginRequest { username, password }`.
- **Response**: `LoginResponse { result: AuthResult { token, principal, username } }`.
- **Status Codes**: `OK`, `Unauthenticated` (user not found or invalid password hash).

#### `ChangePassword`
- **Request**: `ChangePasswordRequest { user_id, old_password, new_password }`.
- **Response**: `ChangePasswordResponse { success: bool }`.
- **Status Codes**: `OK`, `InvalidArgument`, `Unauthenticated` (incorrect old password), `NotFound`.

#### `RequestPasswordReset`
- **Request**: `RequestPasswordResetRequest { username }`.
- **Response**: `RequestPasswordResetResponse { reset_token, expires_at }`.
- **Logic**: Creates a UUID token, stores its SHA-256 hash in DB with 15-minute TTL.
- **Status Codes**: `OK`, `InvalidArgument`, `NotFound`.

#### `ResetPassword`
- **Request**: `ResetPasswordRequest { token, new_password }`.
- **Response**: `ResetPasswordResponse { success: bool }`.
- **Logic**: Verifies token hash exists, is not used, and has not expired; updates bcrypt password hash; marks token used.
- **Status Codes**: `OK`, `InvalidArgument` (invalid token, expired, or already used), `NotFound`.

---

### 5.2 `platform.identity.v1.AddressService`

Protected address book endpoints. Requires a valid `Principal` in the context (injected by gateway or auth interceptor).

#### `ListAddresses`
- **Request**: `ListAddressesRequest {}`.
- **Response**: `ListAddressesResponse { addresses: repeated Address }` (ordered by `is_default DESC, created_at DESC`).
- **Status Codes**: `OK`, `Unauthenticated`.

#### `CreateAddress`
- **Request**: `CreateAddressRequest { recipient_name, phone, street, ward, district, city, is_default }`.
- **Response**: `CreateAddressResponse { address: Address }`.
- **Logic**: If `is_default=true` or it's the user's first address, other addresses for that user have `is_default` reset to `false`.
- **Status Codes**: `OK`, `InvalidArgument`, `Unauthenticated`.

#### `UpdateAddress`
- **Request**: `UpdateAddressRequest { id, recipient_name, phone, street, ward, district, city, is_default }`.
- **Response**: `UpdateAddressResponse { address: Address }`.
- **Status Codes**: `OK`, `InvalidArgument`, `NotFound`, `Unauthenticated`.

#### `DeleteAddress`
- **Request**: `DeleteAddressRequest { id }`.
- **Response**: `DeleteAddressResponse {}`.
- **Status Codes**: `OK`, `NotFound`, `Unauthenticated`.

#### `SetDefaultAddress`
- **Request**: `SetDefaultAddressRequest { id }`.
- **Response**: `SetDefaultAddressResponse { address: Address }`.
- **Logic**: Atomically sets `is_default=false` on all user addresses, then `is_default=true` on the requested address.
- **Status Codes**: `OK`, `NotFound`, `Unauthenticated`.

---

## 6. How to Run & Test

### Local Development (Docker-first)

```bash
# 1. Start postgres-identity from platform-core
cd ../platform-core/infra && docker compose -p platform-core up -d postgres-identity

# 2. Configure environment
cd ../../team-identity
cp .env.example .env

# 3. Apply database migrations
make migrate

# 4. Run the service (via Docker or Go runner)
make run
```

### Verification via grpcurl

```bash
# Register a seller
grpcurl -plaintext -d '{"username":"seller1","password":"password123","role":"seller"}' \
  localhost:50053 platform.identity.v1.AuthService/Register

# Login
grpcurl -plaintext -d '{"username":"seller1","password":"password123"}' \
  localhost:50053 platform.identity.v1.AuthService/Login

# Health check
grpcurl -plaintext localhost:50053 grpc.health.v1.Health/Check
```

### Quality Gate

```bash
make check      # Runs env check, gofmt, go vet, and unit/integration tests
```
