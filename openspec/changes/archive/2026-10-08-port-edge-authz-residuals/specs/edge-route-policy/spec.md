## MODIFIED Requirements

### Requirement: Admin-only RPCs are gated at the edge

`team-gateway` SHALL require the scope `admin` on `VerificationService/ReviewKyc`, `EngagementService/ResolveDispute`,
`AuditService/QueryAuditLog` and `OrderService/ForceFailSaga` before forwarding. An anonymous caller SHALL receive
`unauthenticated` (HTTP 401). A verified caller without `admin` SHALL receive `permission_denied` (HTTP 403).

#### Scenario: Anonymous call to an admin-only RPC

- **WHEN** an anonymous client calls `QueryAuditLog`, `ReviewKyc` or `ResolveDispute` through the gateway
- **THEN** the gateway answers HTTP 401

#### Scenario: Buyer call to an admin-only RPC

- **WHEN** a logged-in buyer calls `QueryAuditLog`, `ReviewKyc` or `ResolveDispute` through the gateway
- **THEN** the gateway answers HTTP 403

#### Scenario: Admin may query the audit log

- **WHEN** the seeded admin calls `QueryAuditLog` through the gateway
- **THEN** the request succeeds with an events list

#### Scenario: A buyer cannot force-fail their own order

- **WHEN** a buyer calls `ForceFailSaga` through the gateway on their own `Pending` order
- **THEN** the gateway answers HTTP 403 and the order is still `Pending` with its stock still held
