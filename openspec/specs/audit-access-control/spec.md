# audit-access-control Specification

## Purpose
Defines who may write audit events and who may read the audit log, so that the audit trail cannot be forged by
end users.

## Requirements

### Requirement: Only services may write audit events

`AuditService/WriteAuditEvent` SHALL require a SERVICE principal holding `audit.write`. A missing or anonymous
principal SHALL receive `unauthenticated`. A user principal, including an admin or a user claiming the scope, and a
service without the scope SHALL receive `permission_denied`. No event SHALL be stored for a refused call.

#### Scenario: A refused edge write leaves no audit event

- **WHEN** a logged-in seller calls `WriteAuditEvent` through the gateway for a fresh target type
- **THEN** the call fails and the seeded admin's `QueryAuditLog` for that target type returns no events
