## ADDED Requirements

### Requirement: Report RPCs are routed and admin ones are gated at the edge

The gateway SHALL forward `EngagementService.CreateReport`, `ListReports` and `ResolveReport` to team-engagement, and SHALL list `EngagementServiceListReportsProcedure` and `EngagementServiceResolveReportProcedure` in `adminProcedures`, so that an anonymous caller is refused with `unauthenticated` and a verified caller without the `admin` scope with `permission_denied` before any upstream call. `CreateReport` and `ResolveReport` SHALL be single-attempt forwards (no retry); `ListReports` SHALL use the read path.

#### Scenario: Listing reports: Anonymous caller cannot list reports at the edge
- **WHEN** an anonymous client calls ListReports through the gateway
- **THEN** the gateway returns `unauthenticated` and team-engagement receives 0 calls
- **VERIFIED BY**: team-gateway `internal/edge` `TestListReportsAnonymousUnauthenticated`

#### Scenario: Listing reports: Non-admin is refused at the edge
- **WHEN** a buyer without the `admin` scope calls ListReports and then ResolveReport through the gateway
- **THEN** both return `permission_denied` and team-engagement receives 0 calls
- **VERIFIED BY**: team-gateway `internal/edge` `TestReportAdminProceduresNonAdminPermissionDenied`

#### Scenario: Listing reports: Admin close is forwarded once
- **WHEN** an admin calls ResolveReport through the gateway and team-engagement returns `Unavailable`
- **THEN** team-engagement receives exactly 1 call and the client gets `unavailable`
- **VERIFIED BY**: team-gateway `internal/edge` `TestResolveReportForwardedOnceNoRetry`

#### Scenario: Listing reports: CreateReport is forwarded once with the caller identity
- **WHEN** a signed-in buyer calls CreateReport through the gateway and team-engagement returns `Unavailable`
- **THEN** team-engagement receives exactly 1 call carrying the buyer's principal metadata and the client gets `unavailable`
- **VERIFIED BY**: team-gateway `internal/edge` `TestCreateReportForwardedOnceWithPrincipal`

#### Scenario: Listing reports: Anonymous CreateReport is not rejected by the edge policy
- **WHEN** an anonymous client calls CreateReport through the gateway
- **THEN** the call reaches team-engagement (which answers `Unauthenticated`) and the client gets `unauthenticated`
- **VERIFIED BY**: team-gateway `internal/edge` `TestCreateReportAnonymousAnsweredByService`

#### Scenario: Listing reports: The admin procedure set is pinned
- **WHEN** the pinned-set test compares `adminProcedures` with the expected list
- **THEN** the list is the entries present before this slice plus `EngagementServiceListReportsProcedure` and `EngagementServiceResolveReportProcedure` and nothing else
- **VERIFIED BY**: team-gateway `internal/edge` `TestAdminProceduresPinned`

#### Scenario: Listing reports: Report RPCs are not left unrouted
- **WHEN** the unrouted guard enumerates EngagementService procedures
- **THEN** `CreateReport`, `ListReports` and `ResolveReport` are all routed by the forwarder
- **VERIFIED BY**: team-gateway `internal/edge` `TestUnroutedGuardCoversReportRPCs`
