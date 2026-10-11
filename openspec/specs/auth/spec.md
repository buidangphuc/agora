# auth Specification

## Purpose
Defines how users are authenticated: team-identity issues RS256 JWTs and publishes its JWKS, the gateway verifies tokens by key id across key rotation, role tokens carry the scopes services check, and login attempts are recorded.

## Requirements

### Requirement: Identity issues RS256 JWTs signed with a private key and a key id

`team-identity` SHALL sign every issued JWT with the `RS256` algorithm using an RSA **private**
key it alone holds, and SHALL set a `kid` (key id) in the JWT header identifying the signing key.
It SHALL NOT sign with a shared HMAC secret; the `HS256`/`JWT_SECRET` signing path SHALL be
removed. The `Principal{id,type,scopes}` claims and token TTL are unchanged.

#### Scenario: Login issues an RS256 token carrying a kid

- **WHEN** a user logs in successfully through `team-identity`
- **THEN** the returned JWT header has `alg = RS256` and a non-empty `kid`, and its claims still
  carry `sub`, `name`, `typ`, and `scopes`

#### Scenario: The HS256 shared-secret signing path is gone

- **WHEN** `team-identity` starts
- **THEN** it requires an RSA private key + `kid` from configuration and does not read or require a
  shared `JWT_SECRET`, and it cannot mint an HS256 token

### Requirement: Identity publishes its public keys at a JWKS endpoint

`team-identity` SHALL expose an unauthenticated HTTP endpoint `GET /.well-known/jwks.json` that
returns a JWKS document — a `keys` array of RSA **public** JWKs (`kty=RSA`, `use=sig`,
`alg=RS256`, each with its `kid`, `n`, `e`). The document SHALL include the key currently used to
sign, and MAY include additional keys to support rotation. It SHALL expose only public key
material (never the private key).

#### Scenario: JWKS serves the active public key

- **WHEN** a client requests `GET /.well-known/jwks.json` from `team-identity`
- **THEN** the response is a JWKS whose `keys` array contains an RSA public JWK whose `kid` matches
  the `kid` in a freshly issued token, and no private key material is present

### Requirement: Gateway verifies RS256 tokens against the cached JWKS by kid

`team-gateway` SHALL verify an incoming bearer JWT by (a) requiring the `RS256` algorithm, (b)
reading the token's `kid`, and (c) matching it to a public key in a JWKS keyset it fetches from
`team-identity` (`JWKS_URL`) and caches. The gateway SHALL hold no signing secret; the shared
`JWT_SECRET` verification path SHALL be removed. A token that verifies resolves to its
`Principal`; a request with no token resolves to the anonymous principal with `PUBLIC_SCOPES`; a
token that is present but does not verify SHALL be rejected with `Unauthenticated` (HTTP 401) on
every route, public ones included (RFC 6750 §3.1), and never downgraded to anonymous. The forwarded `x-principal-{id,type,scopes}` metadata is unchanged.

#### Scenario: A valid RS256 token from identity is accepted

- **WHEN** a request reaches the gateway with a bearer token freshly issued by `team-identity`
- **THEN** the gateway verifies it against the JWKS public key matching the token's `kid` and
  forwards the resolved `x-principal-*` metadata to the upstream service

#### Scenario: Gateway rejects a token not signed by a current JWKS key

- **WHEN** a request reaches the gateway with a bearer token whose signature does not match any
  public key currently in the JWKS (e.g. a token whose `kid` is absent from the JWKS, or one signed
  by a key the JWKS does not publish)
- **THEN** the gateway answers `Unauthenticated` (401, with a `WWW-Authenticate: Bearer
  error="invalid_token"` challenge) on public and protected routes alike, and the request is not
  forwarded upstream; the same holds for a malformed, expired or garbage token

#### Scenario: A request without a token is anonymous

- **WHEN** a request reaches the gateway with no `Authorization` bearer
- **THEN** it resolves to the anonymous principal with `PUBLIC_SCOPES`

### Requirement: Gateway honors key rotation via multiple JWKS keys

The JWKS MAY publish more than one key so that keys can be rotated without downtime.
`team-gateway` SHALL accept a token signed by **any** key currently published in the JWKS, and
SHALL refresh its cached keyset (on a TTL and/or on encountering an unknown `kid`) so that a key
newly rotated into the JWKS becomes usable without redeploying the gateway.

#### Scenario: A token signed by a rotated-in key verifies

- **GIVEN** `team-identity` has added a new signing keypair, published its public key (a new `kid`)
  in the JWKS alongside the previous key, and begun signing with the new `kid`
- **WHEN** a request reaches the gateway with a token signed by the new `kid`, after the gateway has
  refreshed its cached JWKS
- **THEN** the gateway finds the matching public key in the refreshed JWKS and accepts the token,
  and tokens signed by the still-published previous `kid` also continue to verify

### Requirement: Role tokens carry the AI and recommendation scopes

Tokens issued to buyer, seller and admin accounts SHALL include `recommendations:read` and `ai:use`. Service-only
scopes (`inventory.write`, `order.read`, `promotion.reserve`, `audit.write`, `features.read`, `features.dataset`)
SHALL NOT be granted to any role.

#### Scenario: A buyer token carries the AI scopes and no service scope

- **WHEN** a buyer logs in through the gateway
- **THEN** the token's scopes include `recommendations:read` and `ai:use` and include none of the service-only scopes

### Requirement: Login attempts are recorded

A successful login SHALL add a success entry to the user's login history. A wrong password for an existing user SHALL
add a failure entry. An unknown username SHALL record nothing and SHALL return the same error as a wrong password.

#### Scenario: Login history shows a failed and a successful attempt

- **WHEN** a user logs in once with a wrong password and then with the right one, and lists their login history through the gateway
- **THEN** the history contains a failure entry followed by a success entry

### Requirement: The admin role is issued the order.admin scope

Tokens issued to an admin account SHALL include `order.admin` in addition to `admin`. Tokens issued to buyer and seller
accounts, and tokens held by service principals, SHALL NOT include `order.admin`.

#### Scenario: An admin token carries order.admin

- **WHEN** the seeded admin logs in through the gateway
- **THEN** the token's scopes include `admin` and `order.admin`

#### Scenario: Buyer and seller tokens carry no order.admin

- **WHEN** a buyer and a seller each log in through the gateway
- **THEN** neither token's scopes include `order.admin` or `admin`

### Requirement: Every user token carries the session it belongs to

Identity SHALL create a session row on every successful login and register and SHALL
sign that session's id into the JWT as the `sid` claim. Service tokens (no user session)
SHALL carry no `sid`. A failure to record the session SHALL fail the login (no token
without a session row).

#### Scenario: Login returns a token bound to a listed session

- **WHEN** a buyer logs in
- **THEN** the token's `sid` equals the id of a session that `ListSessions` returns for
  that buyer

### Requirement: Revoking a session invalidates its token at the gateway

When a user revokes one of their sessions, identity SHALL mark it revoked and, in the
same database transaction, write a `SessionRevoked` event (session id, user id, the
token expiry) to its outbox, relayed to the Kafka topic `identity.events` keyed by user
id. The gateway SHALL consume `identity.events` into an in-memory denylist of revoked
session ids (each entry kept until that token's expiry), replay the topic from the
earliest retained offset on start, and reject a token whose `sid` is denylisted with 401
on every route. The gateway SHALL still verify each token locally (no per-request call
to identity).

#### Scenario: A revoked session's token stops working

- **WHEN** a buyer revokes the session of another device and that device then calls a
  protected RPC with its old token
- **THEN** within 5 s of the revoke the gateway answers 401 for that token, while the
  buyer's current session keeps working

#### Scenario: Revocations survive a gateway restart

- **WHEN** a session is revoked and the gateway is restarted before the token expires
- **THEN** after the restart the revoked token is still rejected with 401

#### Scenario: Expired denylist entries are dropped

- **WHEN** the revoked token's expiry has passed
- **THEN** the gateway no longer holds its denylist entry (the token is rejected as
  expired anyway)

### Requirement: The gateway forwards trusted client context for auditing

The gateway SHALL set `x-client-ip` (from the socket address, or from
`X-Forwarded-For` only when the request comes from a configured trusted proxy) and
`x-client-user-agent` (clipped to 256 bytes) on every outgoing gRPC call, overwriting
any inbound value. Services SHALL use them only for audit fields (such as a session's
device and IP), never for authorization.

#### Scenario: A new session records device and IP

- **WHEN** a buyer logs in from a browser
- **THEN** the session listed on `/account/security` shows that browser's user agent and
  a non-empty IP

#### Scenario: A client cannot spoof the forwarded IP

- **WHEN** an untrusted client sends `X-Forwarded-For: 1.2.3.4` and `x-client-ip: 1.2.3.4`
- **THEN** the recorded session IP is the socket address, not `1.2.3.4`
