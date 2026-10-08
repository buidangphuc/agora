## Purpose

Defines who may append to and read the audit log in `team-audit`, so the log cannot be forged by external callers and is not readable by the general public.

## ADDED Requirements

### Requirement: Audit writes require service authority

`team-audit` SHALL require a principal of type `service` holding the scope `audit.write` on `WriteAuditEvent`. A caller
without a principal, or with the anonymous principal, SHALL receive `UNAUTHENTICATED`; any other caller (buyer, seller,
admin, or a service principal without the scope) SHALL receive `PERMISSION_DENIED`; in both cases nothing SHALL be appended. The scope SHALL be
carried only by service principals and SHALL NOT be grantable to a user role. A permitted write SHALL keep accepting the
request's `actor_id` (a service records the actor it acts for).

#### Scenario: Anonymous caller cannot forge an event

- **WHEN** a caller with no principal, then the anonymous principal, calls `WriteAuditEvent`
- **THEN** both receive `UNAUTHENTICATED` and the log contains no new row

#### Scenario: A signed-in user cannot write

- **WHEN** a buyer, a seller and an admin each call `WriteAuditEvent`
- **THEN** every call is `PERMISSION_DENIED` and the log is unchanged

#### Scenario: A user principal carrying the scope string is rejected

- **WHEN** a principal of type `user` whose scopes include `audit.write` calls `WriteAuditEvent`
- **THEN** the call is `PERMISSION_DENIED`

#### Scenario: A service with the scope appends an event

- **WHEN** a service principal holding `audit.write` calls `WriteAuditEvent` with an actor, action and target
- **THEN** the event is appended and appears in `QueryAuditLog`

### Requirement: Audit reads are admin-only

`team-audit` SHALL require the scope `admin` on `QueryAuditLog`. A caller without a principal or with the anonymous
principal SHALL receive `UNAUTHENTICATED`; a signed-in caller without `admin` SHALL receive `PERMISSION_DENIED` and no event
data SHALL be returned.

#### Scenario: Anonymous caller cannot read the log

- **WHEN** an anonymous caller calls `QueryAuditLog`
- **THEN** the call is `UNAUTHENTICATED` and the response carries no events

#### Scenario: Non-admin users cannot read the log

- **WHEN** a buyer, then a seller, call `QueryAuditLog`
- **THEN** each call is `PERMISSION_DENIED`

#### Scenario: Admin reads the log

- **WHEN** an admin calls `QueryAuditLog` filtered by a target type
- **THEN** the matching events are returned newest first with a `next_cursor` while more pages remain
