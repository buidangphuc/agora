## ADDED Requirements

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
