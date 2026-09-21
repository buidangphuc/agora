# team-identity — Identity, Authentication & Security Microservice

`team-identity` is the central user authentication, authorization, and identity authority in the Agora Marketplace polyrepo platform. It manages user credentials, bcrypt password hashing, shipping address books, active device sessions, login audit trails, and token-based self-service password recovery.

In accordance with **ADR-0003 and ADR-0006**:
- **Authoritative Token Issuer**: `team-identity` holds the RSA private key in memory and mints **RS256 JWT** access tokens carrying the user identity and resolved permissions (`scopes`).
- **JWKS Publication (ADR-0006)**: It exposes a dedicated, lightweight HTTP listener on port `:50063` serving its public keys at `GET /.well-known/jwks.json` so the edge gateway (`team-gateway`) can verify tokens without sharing private secrets.
- **Database Ownership (Rule 3)**: Exclusively owns and manages `identity_db` (PostgreSQL on port `5435`). No other service holds database credentials or joins across this database.

---

## 1. Service Overview & Core Responsibilities

```
+-----------------------------------------------------------------------------------+
|                                   team-identity                                   |
|                                                                                   |
|  [User & Credentials]        [Token Minting & JWKS]       [RBAC & Scope Engine]   |
|  - Registration & Login      - RS256 Private Key Sign     - Roles: buyer, seller, |
|  - Bcrypt password hashing   - JWKS Server (Port :50063)    admin                 |
|  - Default admin auto-seed   - Key ID (kid) rotation      - Scope union mapping   |
|                                                                                   |
|  [Address Book Management]   [Active Session Tracker]     [Password Recovery]     |
|  - Full CRUD operations      - Multi-device sessions      - 15-minute token TTL   |
|  - Default address switch    - Session revocation         - SHA-256 token hash    |
|  - CASCADE with user delete  - Login audit history        - One-time usage flag   |
+-----------------------------------------------------------------------------------+
```

### Core Responsibilities
1. **User Authentication**: Validates user credentials, securely hashes passwords using `bcrypt` (DefaultCost), and auto-seeds initial system administrators.
2. **Asymmetric Token Minting (RS256)**: Generates cryptographic JWT tokens signed with a PEM-encoded RSA private key, stamped with key ID (`kid`), expiration, user ID (`sub`), username, principal type, and authorized scopes.
3. **Public Key Set Publishing (JWKS)**: Serves RFC 7517/7518 compliant JSON Web Key Sets at `/.well-known/jwks.json` on HTTP port `:50063`.
4. **Role & Permission Resolution**: Translates application roles (`admin`, `seller`, `buyer`) into fine-grained service scopes (e.g. `listing.read`, `listing.write`, `search:read`, `engagement:write`, `admin`).
5. **Shipping Address Book**: Manages user shipping addresses with atomic default address switching and CASCADE deletion.
6. **Account Safety & Session Center**: Tracks active client sessions (device info, IP, last seen, revocation) and records paginated login history.
7. **Password Recovery Lifecycle**: Manages authenticated password changes and secure self-service password reset flows using hashed one-time tokens.

---

## 2. Technology Stack & Key Libraries

| Component / Library | Version | Role & Description |
|---|---|---|
| **Go Runtime** | `1.22` | Core programming language runtime |
| **`google.golang.org/grpc`** | `v1.66.0` | High-performance gRPC server handling identity contracts |
| **`google.golang.org/protobuf`** | `v1.34.2` | Protocol Buffers runtime and generated identity messages |
| **`github.com/jackc/pgx/v5`** | `v5.6.0` | PostgreSQL driver and connection pooling (`pgxpool`) |
| **`github.com/jackc/puddle/v2`** | `v2.2.1` | Underlying connection pool engine for `pgx` |
| **`github.com/golang-jwt/jwt/v5`** | `v5.2.1` | RS256 token signing and claim generation |
| **`golang.org/x/crypto`** | `v0.24.0` | `bcrypt` password hashing and cryptographic helpers |
| **`github.com/google/uuid`** | `v1.6.0` | Cryptographically secure UUIDv4 generation for user IDs and tokens |
| **`go.opentelemetry.io/otel`** | `v1.28.0` | OpenTelemetry distributed tracing integration |
| **`otelgrpc` (contrib)** | `v0.53.0` | gRPC server interceptor for distributed tracing propagation |

---

## 3. Detailed Architecture Diagram

```mermaid
flowchart TD
    subgraph Clients ["Callers & Downstream"]
        GW_RPC["team-gateway gRPC Client (:8080)"]
        GW_JWKS["team-gateway JWKS Client"]
    end

    subgraph IdentityService ["team-identity Microservice"]
        subgraph GRPCEndpoint ["gRPC Server (:50053)"]
            Tracing["Tracing Interceptor (OTel)"]
            Logging["Logging Interceptor (slog)"]
            AuthCheck["Auth / Principal Interceptor"]
            
            H_Auth["AuthHandler (AuthService)"]
            H_Addr["AddressHandler (AddressService)"]
            H_Sess["SessionHandler (SessionService)"]
            H_Health["HealthHandler (grpc_health_v1)"]
        end
        
        subgraph HTTPEndpoint ["HTTP JWKS Server (:50063)"]
            JWKSHandler["GET /.well-known/jwks.json"]
        end

        subgraph CoreLogic ["Domain Services & Security Engine"]
            AuthSvc["AuthService (Business Logic)"]
            Signer["RSA Token Signer (RS256 + kid)"]
            Authz["Authz Engine (Role -> Scopes)"]
        end

        subgraph DataAccess ["Repository Layer (pgxpool)"]
            UserRepo["PostgresUserRepository"]
            AddrRepo["PostgresAddressRepository"]
            SessRepo["PostgresSessionRepository"]
        end
    end

    subgraph Database ["PostgreSQL Database (identity_db :5435)"]
        T_Users["users"]
        T_Addresses["user_addresses"]
        T_Tokens["password_reset_tokens"]
        T_Sessions["sessions"]
        T_History["login_history"]
    end

    GW_RPC -->|gRPC Calls| Tracing
    Tracing --> Logging
    Logging --> AuthCheck
    
    AuthCheck --> H_Auth
    AuthCheck --> H_Addr
    AuthCheck --> H_Sess
    AuthCheck --> H_Health

    H_Auth --> AuthSvc
    H_Addr --> AddrRepo
    H_Sess --> SessRepo

    AuthSvc --> UserRepo
    AuthSvc --> Signer
    AuthSvc --> Authz

    Signer -.->|RSA Public Key| JWKSHandler
    GW_JWKS -->|HTTP GET| JWKSHandler

    UserRepo --> T_Users
    UserRepo --> T_Tokens
    AddrRepo --> T_Addresses
    SessRepo --> T_Sessions
    SessRepo --> T_History
```

---

## 4. Internal Package Structure & Responsibilities

```
team-identity/
├── cmd/server/
│   └── main.go              # Entrypoint: spins up gRPC server (:50053) & HTTP JWKS server (:50063)
├── internal/
│   ├── config/
│   │   ├── config.go        # Settings grouped by Runtime, Server, Database, JWT, Observability
│   │   └── envcheck.go      # Verifies parity between config declarations and .env.example
│   ├── authz/
│   │   └── scopes.go        # Role-to-scope resolution (admin, seller, buyer) & role normalization
│   ├── token/
│   │   ├── jwt.go           # RS256 token minting using RSA private key & kid injection
│   │   └── jwks.go          # RFC 7517 JWKS JSON builder & HTTP handler at /.well-known/jwks.json
│   ├── service/
│   │   └── auth.go          # Core auth business logic: register, login, password update, reset token
│   ├── handler/
│   │   ├── auth.go          # gRPC handler for platform.identity.v1.AuthService
│   │   ├── address.go       # gRPC handler for platform.identity.v1.AddressService
│   │   └── session.go       # gRPC handler for platform.identity.v1.SessionService
│   ├── interceptor/
│   │   ├── auth.go          # Inbound context principal extraction (RequirePrincipal, RequireScopes)
│   │   ├── stream.go        # Streaming interceptor wrappers
│   │   └── tracing.go       # Server tracing & span enrichment
│   ├── repository/
│   │   ├── users.go         # UserRepository interfaces and domain entity structs
│   │   ├── users_pg.go      # PostgreSQL implementation for user profiles & reset tokens
│   │   ├── address.go       # PostgreSQL implementation for user shipping addresses
│   │   └── session.go       # PostgreSQL implementation for active sessions & login history
│   ├── bootstrap/
│   │   ├── resources.go     # Initializes pgxpool database connection and gRPC health checks
│   │   └── lifecycle.go     # Clean resource teardown and connection draining
│   └── observability/
│       └── tracer.go        # OpenTelemetry OTLP trace exporter initialization
└── migrations/
    ├── 0001_users.up.sql              # Users table with unique username and role array
    ├── 0002_addresses.up.sql          # User addresses table with foreign key to users
    ├── 0003_password_reset_tokens.up.sql # Password reset tokens with SHA-256 hash
    └── 0004_sessions.up.sql           # Active sessions and login history audit tables
```

---

## 5. Data Models & Database Schema

`team-identity` exclusively owns `identity_db`. All foreign keys use `ON DELETE CASCADE` referencing `users(id)`.

```mermaid
erDiagram
    users ||--o{ user_addresses : owns
    users ||--o{ password_reset_tokens : requests
    users ||--o{ sessions : maintains
    users ||--o{ login_history : records

    users {
        text id PK "UUIDv4"
        text username UK "Unique account username"
        text password_hash "Bcrypt hash"
        text_array roles "Array of roles (buyer, seller, admin)"
        timestamptz created_at "Registration timestamp"
    }

    user_addresses {
        text id PK "UUIDv4"
        text user_id FK "References users(id)"
        text recipient_name "Recipient full name"
        text phone "Contact telephone number"
        text street "Street address line"
        text ward "Ward / Commune"
        text district "District"
        text city "City / Province"
        boolean is_default "Default shipping flag"
        timestamptz created_at "Creation timestamp"
        timestamptz updated_at "Update timestamp"
    }

    password_reset_tokens {
        text token_hash PK "SHA-256 hex digest of raw token"
        text user_id FK "References users(id)"
        timestamptz expires_at "Expiration timestamp (15 mins)"
        boolean used "One-time usage flag"
        timestamptz created_at "Creation timestamp"
    }

    sessions {
        text id PK "UUIDv4 session id"
        text user_id FK "References users(id)"
        text device "Device / Browser User-Agent"
        text ip "Client IP Address"
        timestamptz created_at "Session initialization time"
        timestamptz last_seen "Last activity timestamp"
        boolean revoked "Explicit revocation flag"
    }

    login_history {
        text id PK "UUIDv4 event id"
        text user_id FK "References users(id)"
        text ip "Client IP Address"
        text user_agent "Client User-Agent"
        boolean success "Login success status"
        timestamptz created_at "Attempt timestamp"
    }
```

---

## 6. API Contracts & Exposed Endpoints

### 6.1 gRPC Services (Port `:50053`)

#### 1. `platform.identity.v1.AuthService` (Public Endpoints)
- **`Register(RegisterRequest) -> RegisterResponse`**: Creates a user with role `buyer` or `seller` (admin is seeded only), hashes password with bcrypt, and mints an RS256 JWT access token.
- **`Login(LoginRequest) -> LoginResponse`**: Validates credentials against stored bcrypt hash, returns an RS256 JWT access token with resolved scopes.
- **`ChangePassword(ChangePasswordRequest) -> ChangePasswordResponse`**: Verifies user old password and updates bcrypt hash with new password.
- **`RequestPasswordReset(RequestPasswordResetRequest) -> RequestPasswordResetResponse`**: Generates a UUID token, stores its SHA-256 hash in DB with 15-minute expiration, and returns the raw token.
- **`ResetPassword(ResetPasswordRequest) -> ResetPasswordResponse`**: Validates that the SHA-256 token hash exists, is unused, and not expired; updates bcrypt password hash; marks token used.

#### 2. `platform.identity.v1.AddressService` (Protected Endpoints — Requires Principal)
- **`ListAddresses(ListAddressesRequest) -> ListAddressesResponse`**: Lists shipping addresses for the caller, sorted by `is_default DESC, created_at DESC`.
- **`CreateAddress(CreateAddressRequest) -> CreateAddressResponse`**: Creates a new shipping address. If marked default or if first address, resets other user addresses to non-default.
- **`UpdateAddress(UpdateAddressRequest) -> UpdateAddressResponse`**: Updates address details and manages default flag.
- **`DeleteAddress(DeleteAddressRequest) -> DeleteAddressResponse`**: Removes an address from the user's address book.
- **`SetDefaultAddress(SetDefaultAddressRequest) -> SetDefaultAddressResponse`**: Atomically sets `is_default=true` on the target address while clearing it on all other addresses of the user.

#### 3. `platform.identity.v1.SessionService` (Protected Endpoints — Requires Principal)
- **`ListSessions(ListSessionsRequest) -> ListSessionsResponse`**: Lists all active sessions for the authenticated user.
- **`RevokeSession(RevokeSessionRequest) -> RevokeSessionResponse`**: Revokes an active session by session ID.
- **`ListLoginHistory(ListLoginHistoryRequest) -> ListLoginHistoryResponse`**: Returns paginated login audit history with cursor support.

#### 4. Auxiliary gRPC Services
- `grpc.health.v1.Health`: Standard gRPC health check (`Check`, `Watch`).
- `grpc.reflection.v1alpha.ServerReflection`: gRPC Server Reflection for debugging tools like `grpcurl`.

---

### 6.2 HTTP JWKS Endpoint (Port `:50063`)

- **`GET /.well-known/jwks.json`**
  - Public, read-only, unauthenticated endpoint.
  - Serves standard RFC 7517 / RFC 7518 JWKS JSON.
  - Headers: `Content-Type: application/json`, `Cache-Control: public, max-age=300`.
  - Example Response:
    ```json
    {
      "keys": [
        {
          "kty": "RSA",
          "use": "sig",
          "alg": "RS256",
          "kid": "identity-key-2026",
          "n": "u1b7w...[base64url-encoded-modulus]",
          "e": "AQAB"
        }
      ]
    }
    ```

---

## 7. Security & Flow Mechanisms

### 7.1 RS256 Private Key Signing & JWKS Key Rotation (ADR-0006)

```mermaid
sequenceDiagram
    autonumber
    actor Buyer as User / Client
    participant GW as team-gateway
    participant ID_gRPC as team-identity gRPC (:50053)
    participant ID_JWKS as team-identity JWKS (:50063)
    participant DB as identity_db

    Buyer->>GW: POST /platform.identity.v1.AuthService/Login
    GW->>ID_gRPC: gRPC Login(username, password)
    ID_gRPC->>DB: Query user by username
    DB-->>ID_gRPC: Return user & password_hash
    ID_gRPC->>ID_gRPC: bcrypt.CompareHashAndPassword()
    ID_gRPC->>ID_gRPC: Calculate scopes for user roles
    ID_gRPC->>ID_gRPC: Sign RS256 JWT (stamping kid in header)
    ID_gRPC-->>GW: AuthResult (token + principal)
    GW-->>Buyer: LoginResponse (token)

    Note over GW,ID_JWKS: Gateway verifies future requests by fetching JWKS
    GW->>ID_JWKS: GET /.well-known/jwks.json
    ID_JWKS-->>GW: 200 OK (Public Key Set)
```

1. **Private Key Isolation**: `team-identity` reads `JWT_PRIVATE_KEY` (PEM-encoded RSA key) and `JWT_KID` at startup. The private key never leaves this service.
2. **Key Rotation Readiness**: `BuildJWKS` accepts multiple public keys, allowing a new key to be published to JWKS before it begins signing tokens, ensuring zero-downtime rotation.

### 7.2 Role-Based Access Control (RBAC) & Scope Mapping

Scopes are computed deterministically from the user's assigned roles:

| Role | Assigned System Scopes | Notes |
|---|---|---|
| `admin` | `listing.read`, `listing.write`, `search:read`, `search:write`, `engagement:read`, `engagement:write`, `admin` | Seeded via `EnsureAdmin` (`admin`/`admin123`) |
| `seller` | `listing.read`, `listing.write`, `search:read`, `search:write`, `engagement:read`, `engagement:write` | Self-assignable upon registration |
| `buyer` | `listing.read`, `search:read`, `search:write`, `engagement:read`, `engagement:write` | Default registration role |

### 7.3 Token Claims Structure
Tokens minted by `team-identity` carry:
```json
{
  "sub": "usr_9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
  "name": "buyer_alice",
  "typ": "user",
  "scopes": ["listing.read", "search:read", "search:write", "engagement:read", "engagement:write"],
  "iat": 1774140000,
  "exp": 1774143600
}
```

### 7.4 Self-Service Password Reset Flow
1. **Request Reset**: Client calls `RequestPasswordReset(username)`. Service generates a raw UUIDv4 token, stores its SHA-256 hash in `password_reset_tokens` table with `expires_at = now + 15m` and `used = false`, and returns the raw token.
2. **Execute Reset**: Client calls `ResetPassword(raw_token, new_password)`. Service computes SHA-256 hash of `raw_token`, looks up token in DB, verifies `used == false` and `now < expires_at`, updates `users.password_hash` with new bcrypt hash, and marks `used = true`.

---

## 8. Environment Configuration Reference

| Environment Variable | Type | Default | Description |
|---|---|---|---|
| `ENV` | `string` | `local` | Deployment environment (`local`, `dev`, `prod`) |
| `LOG_LEVEL` | `string` | `info` | Structured logging verbosity (`debug`, `info`, `warn`, `error`) |
| `LOG_JSON` | `bool` | `true` | Emit structured JSON log format |
| `GRPC_HOST` | `string` | `0.0.0.0` | Bind host for gRPC server |
| `GRPC_PORT` | `int` | `50053` | gRPC listening port |
| `GRPC_REFLECTION_ENABLED` | `bool` | `true` | Enable gRPC Server Reflection |
| `SHUTDOWN_GRACE_SECONDS` | `float` | `10` | Grace period for server connection drain |
| `DATABASE_ENABLED` | `bool` | `true` | Enable PostgreSQL database connection |
| `DATABASE_URL` | `string` | `""` | PostgreSQL connection string (`postgresql://identity_svc:identity_pass@localhost:5435/identity_db`) |
| `DB_MAX_CONNS` | `int32` | `10` | Maximum connections in pgxpool |
| `JWT_PRIVATE_KEY` | `string` | `""` | PEM-encoded RSA private key for RS256 signing |
| `JWT_KID` | `string` | `""` | Key ID stamped in JWT headers (e.g. `identity-key-2026`) |
| `JWKS_HTTP_PORT` | `int` | `50063` | Listening port for HTTP JWKS server |
| `JWT_TTL_SECONDS` | `int` | `3600` | Access token lifespan in seconds (default 1 hour) |
| `OTEL_ENABLED` | `bool` | `false` | Enable OpenTelemetry tracing exporter |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `string` | `""` | OTLP gRPC collector endpoint (`localhost:4317`) |
| `OTEL_SERVICE_NAME` | `string` | `team-identity` | Service name stamped in distributed traces |

---

## 9. Running & Testing

### Local Development Commands
```bash
# 1. Start postgres-identity database container
docker compose -p platform-core up -d postgres-identity

# 2. Configure environment
cp .env.example .env

# 3. Apply database migrations
make migrate

# 4. Run environment drift check and unit tests
make check

# 5. Start the gRPC + JWKS service
make run
```

### Verification via grpcurl & curl

**Register a New Seller:**
```bash
grpcurl -plaintext -d '{"username":"seller_dan","password":"SecurePassword123","role":"seller"}' \
  localhost:50053 platform.identity.v1.AuthService/Register
```

**Login:**
```bash
grpcurl -plaintext -d '{"username":"seller_dan","password":"SecurePassword123"}' \
  localhost:50053 platform.identity.v1.AuthService/Login
```

**Fetch Public JWKS Keys (HTTP):**
```bash
curl -i http://localhost:50063/.well-known/jwks.json
```

**Health Check:**
```bash
grpcurl -plaintext localhost:50053 grpc.health.v1.Health/Check
```
