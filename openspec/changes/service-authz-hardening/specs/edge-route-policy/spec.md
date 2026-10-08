## Purpose

Defines which RPCs `team-gateway` refuses to route and which it gates on the admin scope at the edge, as defence in depth in front of the services' own authorization.

## ADDED Requirements

### Requirement: Admin-only RPCs are gated at the edge

`team-gateway` SHALL require the scope `admin` on `VerificationService/ReviewKyc`, `EngagementService/ResolveDispute` and
`AuditService/QueryAuditLog` before forwarding. An anonymous or unverifiable caller SHALL receive `unauthenticated` (HTTP
401); a verified caller without `admin` SHALL receive `permission_denied` (HTTP 403); in both cases the upstream service
SHALL NOT be called. An `admin` caller SHALL be forwarded unchanged. The services remain the authoritative check.

#### Scenario: Anonymous call to an admin-only RPC

- **WHEN** an anonymous client calls `QueryAuditLog`, `ReviewKyc` or `ResolveDispute` through the gateway
- **THEN** the gateway answers HTTP 401 with code `unauthenticated`

#### Scenario: Buyer call to an admin-only RPC

- **WHEN** a logged-in buyer calls `QueryAuditLog`, `ReviewKyc` or `ResolveDispute` through the gateway
- **THEN** the gateway answers HTTP 403 with code `permission_denied`

#### Scenario: Admin call passes through

- **WHEN** the seeded admin calls `QueryAuditLog` through the gateway
- **THEN** the request is forwarded and the audit events list is returned

### Requirement: Internal-only RPCs are not routed

`team-gateway` SHALL NOT route `VoucherService/CommitReservation`, `VoucherService/ReleaseReservation` or
`AuditService/WriteAuditEvent`; for every caller, anonymous or authenticated, they SHALL answer `unimplemented` (HTTP 501)
without contacting the upstream service. `VoucherService/ValidateAndReserve` SHALL stay routed because the checkout
preview calls it.

#### Scenario: Anonymous call to an unrouted RPC

- **WHEN** an anonymous client calls `CommitReservation`, `ReleaseReservation` or `WriteAuditEvent` through the gateway
- **THEN** the gateway answers HTTP 501 with code `unimplemented`

#### Scenario: Authenticated call to an unrouted RPC

- **WHEN** a logged-in buyer, then the admin, call the same three RPCs through the gateway
- **THEN** each answers HTTP 501 with code `unimplemented`

#### Scenario: The preview RPC stays routed

- **WHEN** a logged-in buyer calls `ValidateAndReserve` with their own preview reservation id through the gateway
- **THEN** the request reaches `team-promotion` and is answered (valid or not), not `unimplemented`

### Requirement: The forwarded principal is rebuilt from the verified token

`team-gateway` SHALL continue to build the `x-principal-*` metadata it sends upstream only from the principal it verified,
and SHALL NOT copy any client-supplied principal header into it.

#### Scenario: A client-supplied scope header is ignored

- **WHEN** a logged-in buyer sends `x-principal-scopes: promotion.reserve,audit.write,admin` (and `x-principal-type: service`) as HTTP headers on a `CommitReservation` or `ResolveDispute` call
- **THEN** the call is still rejected (`unimplemented` / `permission_denied`) and nothing changes
