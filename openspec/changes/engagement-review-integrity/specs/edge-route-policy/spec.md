## ADDED Requirements

### Requirement: HideReview is admin-gated and forwarded at the edge

The gateway SHALL forward `EngagementService.HideReview` to team-engagement and SHALL list `EngagementServiceHideReviewProcedure` in `adminProcedures`, so that an anonymous caller is refused with `unauthenticated` and a verified caller without the `admin` scope with `permission_denied` before any upstream call. The forward SHALL be a single attempt (no retry).

#### Scenario: Anonymous caller is refused at the edge
- **WHEN** an anonymous client calls HideReview through the gateway
- **THEN** the gateway returns `unauthenticated` and team-engagement receives 0 calls
- **VERIFIED BY**: team-gateway `internal/edge` `TestHideReviewAnonymousUnauthenticated`

#### Scenario: Non-admin is refused at the edge
- **WHEN** a buyer without the `admin` scope calls HideReview through the gateway
- **THEN** the gateway returns `permission_denied` and team-engagement receives 0 calls
- **VERIFIED BY**: team-gateway `internal/edge` `TestHideReviewNonAdminPermissionDenied`

#### Scenario: Admin call is forwarded once
- **WHEN** an admin calls HideReview through the gateway and team-engagement returns `Unavailable`
- **THEN** team-engagement receives exactly 1 call and the client gets `unavailable`
- **VERIFIED BY**: team-gateway `internal/edge` `TestHideReviewForwardedOnceNoRetry`

#### Scenario: The admin procedure set is pinned
- **WHEN** the pinned-set test compares `adminProcedures` with the expected list
- **THEN** the list contains the existing 7 procedures plus `EngagementServiceHideReviewProcedure` and nothing else
- **VERIFIED BY**: team-gateway `internal/edge` `TestAdminProceduresPinnedSet`

#### Scenario: HideReview is not left unrouted
- **WHEN** the unrouted guard enumerates EngagementService procedures
- **THEN** `HideReview` is routed by the forwarder
- **VERIFIED BY**: team-gateway `internal/edge` `TestUnroutedGuardCoversHideReview`
