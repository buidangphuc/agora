## ADDED Requirements

### Requirement: Dispute list RPCs are routed and gated at the edge

The gateway SHALL forward `EngagementService.ListOrderDisputes` to team-engagement for signed-in users, SHALL forward `EngagementService.ListDisputes` and list `EngagementServiceListDisputesProcedure` in `adminProcedures`, and SHALL pass `return_id` on `CreateDispute` and `ruling` on `ResolveDispute` through unchanged. An anonymous caller SHALL be refused with `unauthenticated` and a verified caller without the `admin` scope with `permission_denied` before any upstream call to an admin procedure.

#### Scenario: Dispute edge: Anonymous ListDisputes is refused
- **WHEN** an anonymous client calls ListDisputes through the gateway
- **THEN** the gateway returns `unauthenticated` and team-engagement receives 0 calls
- **VERIFIED BY**: team-gateway `internal/edge` `TestListDisputesAnonymousUnauthenticated`

#### Scenario: Dispute edge: Non-admin ListDisputes is refused
- **WHEN** a buyer without the `admin` scope calls ListDisputes through the gateway
- **THEN** the gateway returns `permission_denied` and team-engagement receives 0 calls
- **VERIFIED BY**: team-gateway `internal/edge` `TestListDisputesNonAdminPermissionDenied`

#### Scenario: Dispute edge: Admin ListDisputes is forwarded once
- **WHEN** an admin calls ListDisputes through the gateway and team-engagement returns `Unavailable`
- **THEN** team-engagement receives exactly 1 call and the client gets `unavailable`
- **VERIFIED BY**: team-gateway `internal/edge` `TestListDisputesForwardedOnceNoRetry`

#### Scenario: Dispute edge: ListOrderDisputes carries the caller's principal
- **WHEN** a signed-in buyer calls ListOrderDisputes through the gateway
- **THEN** team-engagement receives 1 call with the buyer's principal and the request's order_id, and the response is returned unchanged
- **VERIFIED BY**: team-gateway `internal/edge` `TestListOrderDisputesForwardsPrincipal`

#### Scenario: Dispute edge: Anonymous ListOrderDisputes is refused
- **WHEN** an anonymous client calls ListOrderDisputes through the gateway
- **THEN** the client gets `unauthenticated`
- **VERIFIED BY**: team-gateway `internal/edge` `TestListOrderDisputesAnonymousUnauthenticated`

#### Scenario: Dispute edge: return_id and ruling pass through
- **WHEN** a buyer calls CreateDispute with return_id "r-1" and an admin calls ResolveDispute with ruling BUYER_FAVOUR through the gateway
- **THEN** team-engagement receives return_id "r-1" in the first call and ruling BUYER_FAVOUR in the second
- **VERIFIED BY**: team-gateway `internal/edge` `TestDisputeFieldsPassThrough`

#### Scenario: Dispute edge: The admin procedure set is pinned
- **WHEN** the pinned-set test compares `adminProcedures` with the expected list
- **THEN** the list equals the set merged before this slice plus `EngagementServiceListDisputesProcedure` and nothing else
- **VERIFIED BY**: team-gateway `internal/edge` `TestAdminProceduresPinnedSet`

#### Scenario: Dispute edge: The new RPCs are not left unrouted
- **WHEN** the unrouted guard enumerates EngagementService procedures
- **THEN** `ListOrderDisputes` and `ListDisputes` are routed by the forwarder
- **VERIFIED BY**: team-gateway `internal/edge` `TestUnroutedGuardCoversDisputeLists`
