## ADDED Requirements

### Requirement: QueryAuditLog filters by target id, action and time range

`AuditService.QueryAuditLog` SHALL accept the optional filters `actor_id`, `target_type`, `target_id`, `action` (all exact match; an empty value means no filter), `created_after` (inclusive lower bound) and `created_before` (exclusive upper bound); an unset timestamp means no bound. All set filters SHALL combine with AND. `page.total` SHALL count the matches before paging. Ordering (`created_at DESC, id DESC`), the default page size 20, the maximum 100 and the offset cursor SHALL be unchanged. The Postgres and in-memory repositories SHALL return identical results for the same filter.

#### Scenario: Target id and target type narrow to one target
- **WHEN** an admin queries with target_type "order" and target_id "o-1" while the log holds events for orders "o-1" and "o-2"
- **THEN** only the events of "o-1" are returned and `page.total` equals their count
- **VERIFIED BY**: team-audit `internal/handler` `TestQueryAuditLogFilterByTarget`
- **VERIFIED BY**: platform-e2e `features/security/audit_query_filters.feature` "An admin filters the audit log by target"

#### Scenario: Action filter is an exact match
- **WHEN** an admin queries with action "kyc.review" while the log also holds "kyc.reviewed" and "review.hide"
- **THEN** only events whose action is exactly "kyc.review" are returned
- **VERIFIED BY**: team-audit `internal/handler` `TestQueryAuditLogFilterByActionExact`
- **VERIFIED BY**: platform-e2e `features/security/audit_query_filters.feature` "An admin filters the audit log by action"

#### Scenario: Filters combine with AND
- **WHEN** an admin queries with actor "adm-1", action "order.force_fail" and target_id "o-1" while other events match only one or two of those three
- **THEN** only the event matching all three filters is returned
- **VERIFIED BY**: team-audit `internal/handler` `TestQueryAuditLogFiltersAreAnded`
- **VERIFIED BY**: platform-e2e `features/security/audit_query_filters.feature` "Filters combine with AND"

#### Scenario: Time range includes the lower bound and excludes the upper bound
- **WHEN** an admin queries a target holding events at 10:00:00, 11:00:00 and 12:00:00 with created_after 10:00:00 and created_before 12:00:00
- **THEN** the events at 10:00:00 and 11:00:00 are returned and the event at 12:00:00 is not
- **VERIFIED BY**: team-audit `internal/service` `TestQueryTimeRangeBoundaries`
- **VERIFIED BY**: platform-e2e `features/security/audit_query_filters.feature` "The time range includes created_after and excludes created_before"

#### Scenario: One-sided time bounds
- **WHEN** an admin queries with only created_after set, then with only created_before set, over events at 10:00:00 and 12:00:00 and bound 11:00:00
- **THEN** the first query returns the 12:00:00 event and the second returns the 10:00:00 event
- **VERIFIED BY**: team-audit `internal/service` `TestQueryOneSidedTimeBounds`

#### Scenario: Equal bounds match nothing
- **WHEN** an admin queries with created_after equal to created_before equal to the timestamp of an existing event
- **THEN** the call succeeds with no events and total 0
- **VERIFIED BY**: team-audit `internal/service` `TestQueryEqualTimeBoundsIsEmpty`

#### Scenario: No match returns an empty page
- **WHEN** an admin queries with a target_id that no event has
- **THEN** the call succeeds with an empty events list, total 0 and an empty next_cursor
- **VERIFIED BY**: team-audit `internal/handler` `TestQueryAuditLogNoMatchIsEmpty`
- **VERIFIED BY**: platform-e2e `features/security/audit_query_filters.feature` "A filter with no match returns an empty list"

#### Scenario: Paging walks a filtered result
- **WHEN** an admin queries a target holding 25 events with page size 20 and then follows next_cursor
- **THEN** page 1 holds 20 events with total 25 and a next_cursor, page 2 holds the 5 oldest and no next_cursor, newest first throughout
- **VERIFIED BY**: team-audit `internal/handler` `TestQueryAuditLogFilteredPaging`
- **VERIFIED BY**: platform-e2e `features/security/audit_query_filters.feature` "A filtered result is paged newest first"

#### Scenario: Requests without the new fields behave as before
- **WHEN** an admin queries with only actor_id and target_type
- **THEN** the result equals the pre-change behaviour (same events, order, total)
- **VERIFIED BY**: team-audit `internal/handler` `TestQueryAuditLogCursorPaging`

#### Scenario: Filter values are bound parameters
- **WHEN** an admin queries with action `x' OR '1'='1`
- **THEN** the call succeeds with no events
- **VERIFIED BY**: platform-e2e `features/security/audit_query_filters.feature` "A quote in a filter value is matched literally"

#### Scenario: Audit query: In-memory repository matches Postgres
- **WHEN** the same table of filters runs against both repositories over the same seeded events
- **THEN** both return the same event ids in the same order and the same total
- **VERIFIED BY**: team-audit `internal/repository` `TestRepositoryFilterParity` (Postgres half runs when `TEST_DATABASE_URL` is set)

### Requirement: An inverted time range is refused

When both `created_after` and `created_before` are set and `created_before` is earlier than `created_after`, `QueryAuditLog` SHALL return `InvalidArgument` and run no query. Authorization SHALL be checked before this validation.

#### Scenario: created_before earlier than created_after
- **WHEN** an admin queries with created_after 12:00:00 and created_before 10:00:00
- **THEN** the call fails with InvalidArgument and the repository is not called
- **VERIFIED BY**: team-audit `internal/handler` `TestQueryAuditLogInvertedRangeInvalidArgument`
- **VERIFIED BY**: platform-e2e `features/security/audit_query_filters.feature` "An inverted time range is refused"

#### Scenario: Authorization precedes validation
- **WHEN** a non-admin user queries with an inverted time range
- **THEN** the call fails with PermissionDenied, not InvalidArgument
- **VERIFIED BY**: team-audit `internal/handler` `TestQueryAuditLogAuthBeforeValidation`

### Requirement: Filtered queries stay admin only

`QueryAuditLog` SHALL return `Unauthenticated` for a missing or anonymous principal and `PermissionDenied` for any principal without the `admin` scope, whatever filters are set. The gateway SHALL keep `QueryAuditLog` admin-gated and `WriteAuditEvent` unrouted, and SHALL forward the new fields unchanged.

#### Scenario: Non-admin cannot query with filters
- **WHEN** a seller (no admin scope) queries through the gateway with target_type "order" and target_id "o-1"
- **THEN** the gateway answers HTTP 403 and no event is returned
- **VERIFIED BY**: platform-e2e `features/security/audit_query_filters.feature` "Only an admin can query with filters"

#### Scenario: Anonymous caller cannot query with filters
- **WHEN** an anonymous client queries through the gateway with an action filter
- **THEN** the gateway answers HTTP 401
- **VERIFIED BY**: platform-e2e `features/security/audit_query_filters.feature` "Only an admin can query with filters"

#### Scenario: Handler refuses a missing principal with any filter
- **WHEN** a caller with no principal calls the team-audit handler with any filter
- **THEN** the call fails with Unauthenticated and the repository is not called
- **VERIFIED BY**: team-audit `internal/handler` `TestQueryAuditLogUnauthenticated`

#### Scenario: Handler denies a non-admin before reading filters
- **WHEN** a user principal without `admin` calls the team-audit handler with any filter
- **THEN** the call fails with PermissionDenied and the repository is not called
- **VERIFIED BY**: team-audit `internal/handler` `TestQueryAuditLogAdminOnly`

#### Scenario: Gateway forwards the new fields unchanged
- **WHEN** an admin request carrying target_id, action, created_after and created_before reaches the gateway forwarder
- **THEN** the upstream client receives a request with the same four values
- **VERIFIED BY**: team-gateway `internal/edge` `TestQueryAuditLogForwardsFilters`

#### Scenario: The admin procedure set is unchanged
- **WHEN** the gateway policy tests run after regeneration
- **THEN** `QueryAuditLog` is in `adminProcedures` and `WriteAuditEvent` is unrouted
- **VERIFIED BY**: team-gateway `internal/edge` `TestAdminProcedureSetIsPinned`

### Requirement: Action index serves action-only queries

The `audit_events` table SHALL have the index `idx_audit_events_action_created` on `(action, created_at DESC)`, created by migration `002` and removable by its down migration.

#### Scenario: Migration creates and drops the index
- **WHEN** migration 002 is applied up and then down on a database holding migration 001
- **THEN** the index exists after up and is absent after down, and existing rows are untouched
- **VERIFIED BY**: team-audit `internal/repository` `TestMigration002ActionIndex` (runs when `TEST_DATABASE_URL` is set)

#### Scenario: Migration up is re-runnable without failure
- **WHEN** migration 002 up is applied twice in a row
- **THEN** the second run does not fail and exactly one `idx_audit_events_action_created` index exists
- **VERIFIED BY**: team-audit `internal/repository` `TestMigration002ActionIndex` (runs when `TEST_DATABASE_URL` is set)

### Requirement: Shared e2e audit helper

`platform-e2e/tests/e2e/support/audit_support.py` SHALL let any e2e step query audit events through the gateway as the seeded admin by target type plus target id and optionally action, seed events with chosen timestamps, assert that a target has exactly one event of an action with given actor and metadata, assert that it has none, and stop and start team-audit with guaranteed restart.

#### Scenario: Helper finds seeded events through the gateway
- **WHEN** the helper seeds two events for a fresh target and queries them as admin
- **THEN** it returns exactly those two events, newest first
- **VERIFIED BY**: platform-e2e `features/security/audit_query_filters.feature` "An admin filters the audit log by target"

#### Scenario: Helper restarts team-audit after a stop
- **WHEN** a scenario stops team-audit through the helper and finishes, passing or failing
- **THEN** a later admin QueryAuditLog through the gateway returns HTTP 200
- **VERIFIED BY**: platform-e2e `features/security/audit_query_filters.feature` "The audit helper restarts team-audit after a stop"

#### Scenario: Query fails clearly while team-audit is down
- **WHEN** the helper queries while team-audit is stopped
- **THEN** it raises `AuditUnavailable` (HTTP 5xx from the gateway) instead of returning an empty list
- **VERIFIED BY**: platform-e2e `features/security/audit_query_filters.feature` "The audit helper restarts team-audit after a stop"

#### Scenario: Helper query with a non-admin token is refused
- **WHEN** the helper queries with a seller token
- **THEN** the gateway answers HTTP 403 and the helper returns that response without events
- **VERIFIED BY**: platform-e2e `features/security/audit_query_filters.feature` "Only an admin can query with filters"
