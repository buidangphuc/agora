## ADDED Requirements

### Requirement: Dispute resolution is audited before it is applied

`EngagementService.ResolveDispute` SHALL, after authorization, validation, loading the dispute and the closed check have passed, write exactly one team-audit event and only then update the dispute. The event SHALL have `action=dispute.resolve`, `target_type=dispute`, `target_id` equal to the dispute id, `actor_id` equal to the admin's user id from the verified Principal, and `metadata` holding `resolution` (trimmed request note), `order_id` (the dispute's order) and `status` (the requested status). The audit write SHALL use the team-engagement service principal with scope `audit.write`. A call refused before the audit step SHALL write no event.

#### Scenario: Dispute audit: Admin resolves a dispute and one event is written
- **WHEN** an admin with user id "adm-1" calls ResolveDispute for open dispute "d-1" of order "o-1" with status resolved and resolution "refund buyer"
- **THEN** the dispute status becomes resolved and exactly one WriteAuditEvent call is made with action `dispute.resolve`, target_type `dispute`, target_id "d-1", actor_id "adm-1", and metadata `resolution`="refund buyer", `order_id`="o-1", `status`="resolved"
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeWritesAuditEvent`

#### Scenario: Dispute audit: Investigating dispute can be updated again with one event each
- **WHEN** an admin calls ResolveDispute with status investigating twice on the same dispute (an investigating dispute is not closed, so the current service allows it)
- **THEN** each call returns OK and writes exactly one `dispute.resolve` event, 2 in total
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeInvestigatingRepeatAudited`

#### Scenario: Dispute audit: Any permitted target status is audited
- **WHEN** an admin moves an open dispute to rejected, and another open dispute to investigating
- **THEN** each call writes exactly one `dispute.resolve` event whose metadata `status` equals the requested status
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeAuditsEveryStatus`

#### Scenario: Dispute audit: Audit is written before the update
- **WHEN** ResolveDispute runs for an open dispute
- **THEN** the recorded call order is the audit write, then the repository update
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveDisputeAuditBeforeUpdate`

#### Scenario: Dispute audit: written as the service principal
- **WHEN** the audit client sends the `dispute.resolve` event through the generic write method
- **THEN** the outgoing principal is `service-team-engagement`, type `service`, scopes `audit.write`, and the admin's own principal metadata is absent
- **VERIFIED BY**: team-engagement `internal/upstream` `TestAuditClientWriteEventUsesServicePrincipal`

#### Scenario: Dispute audit: Hide-review audit behaviour is unchanged
- **WHEN** an admin hides a review after the generic write method is introduced
- **THEN** the event still has action `review.hide`, target_type `review` and metadata `reason` and `listing_id`
- **VERIFIED BY**: team-engagement `internal/upstream` `TestWriteHideEventStillBuildsReviewHideEvent`

#### Scenario: Dispute audit: Resolution note is trimmed in the event
- **WHEN** an admin resolves a dispute with resolution "  refund  "
- **THEN** the audit metadata `resolution` equals "refund" and the stored dispute resolution equals "refund"
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeAuditResolutionTrimmed`

#### Scenario: Dispute audit: Empty resolution is audited as empty
- **WHEN** an admin moves a dispute to investigating with an empty resolution
- **THEN** the event is written with metadata `resolution`="" and the call returns OK
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeEmptyResolutionAudited`

### Requirement: Dispute resolution fails closed when team-audit is unavailable

`EngagementService.ResolveDispute` SHALL return `Unavailable` and SHALL leave the dispute unchanged when the team-audit write errors, times out, or no audit client is configured (`UPSTREAM_AUDIT_ADDR` unset). Other dispute RPCs SHALL NOT depend on team-audit.

#### Scenario: Dispute audit: Audit error leaves the dispute open
- **WHEN** team-audit returns an error during ResolveDispute for an open dispute
- **THEN** the call returns `Unavailable` and GetDispute still returns the previous status and resolution
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveDisputeAuditFailureFailsClosed`

#### Scenario: Dispute audit: timeout is Unavailable
- **WHEN** team-audit does not answer within `UPSTREAM_CALL_TIMEOUT_SECONDS` during ResolveDispute
- **THEN** the audit client returns an error within the timeout plus 1 second, and ResolveDispute maps it to `Unavailable` with the dispute unchanged
- **VERIFIED BY**: team-engagement `internal/upstream` `TestAuditClientWriteEventTimeout`; team-engagement `internal/service` `TestResolveDisputeAuditTimeoutUnavailable`

#### Scenario: Dispute audit: No audit upstream configured
- **WHEN** ResolveDispute runs with `UPSTREAM_AUDIT_ADDR` unset
- **THEN** the call returns `Unavailable` and the dispute is unchanged
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveDisputeNilAuditUnavailable`

#### Scenario: Dispute audit: Update failure after audit is Internal and retryable
- **WHEN** the audit write succeeds, the repository update fails, and the call is retried with the repository healthy
- **THEN** the first call returns `Internal`, the retry returns OK, the dispute is resolved, and 2 `dispute.resolve` events exist for it
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveDisputeUpdateFailureInternalThenRetry`

#### Scenario: Dispute audit: Other dispute RPCs ignore team-audit availability
- **WHEN** team-audit is down and a buyer calls CreateDispute and GetDispute
- **THEN** both calls succeed
- **VERIFIED BY**: team-engagement `internal/handler` `TestDisputeReadWriteIndependentOfAudit`

### Requirement: No audit event for refused dispute resolutions

`EngagementService.ResolveDispute` SHALL write no audit event when the caller is unauthenticated or not an admin, when `dispute_id` is empty, when the status is invalid, when the dispute does not exist, or when the dispute is already closed. Authorization SHALL be checked before validation and before any audit call, and team-audit availability SHALL NOT change the result of these refusals.

#### Scenario: Dispute audit: Anonymous caller is unauthenticated
- **WHEN** an anonymous caller calls ResolveDispute
- **THEN** the call returns `Unauthenticated` and no audit event is written
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeAnonymousNoAudit`

#### Scenario: Dispute audit: Non-admin is denied
- **WHEN** a buyer, a seller and a service principal without the `admin` scope each call ResolveDispute
- **THEN** each call returns `PermissionDenied`, no audit event is written and the dispute is unchanged
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeNonAdminDeniedNoAudit`

#### Scenario: Dispute audit: Authorization precedes validation
- **WHEN** a non-admin calls ResolveDispute with an empty dispute id and team-audit is down
- **THEN** the call returns `PermissionDenied`, not `InvalidArgument` or `Unavailable`
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeAuthBeforeValidationAndAudit`

#### Scenario: Dispute audit: Empty dispute id is rejected
- **WHEN** an admin calls ResolveDispute with an empty dispute id
- **THEN** the call returns `InvalidArgument` and no audit event is written
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeEmptyIDNoAudit`

#### Scenario: Dispute audit: Invalid status is rejected
- **WHEN** an admin calls ResolveDispute with an unspecified or open status
- **THEN** the call returns `InvalidArgument` and no audit event is written
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeInvalidStatusNoAudit`

#### Scenario: Dispute audit: Unknown dispute
- **WHEN** an admin calls ResolveDispute with an id that does not exist
- **THEN** the call returns `NotFound` and no audit event is written, also when team-audit is down
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeNotFoundNoAudit`

#### Scenario: Dispute audit: Already closed dispute is not re-audited
- **WHEN** an admin calls ResolveDispute on a dispute already resolved or rejected
- **THEN** the call returns `FailedPrecondition` and no audit event is written
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeClosedNoAudit`

### Requirement: Audited dispute resolution works end to end

With the e2e stack running team-audit and `UPSTREAM_AUDIT_ADDR` set for team-engagement, an admin's resolution through the gateway SHALL be visible in team-audit, and a resolution while team-audit is stopped SHALL be refused with the dispute unchanged.

#### Scenario: Dispute audit: Resolved dispute leaves one audit event end to end
- **WHEN** a buyer opens a dispute and an admin resolves it through the gateway with resolution "refund buyer"
- **THEN** QueryAuditLog as admin for target_type `dispute` and the dispute id returns exactly one event with action `dispute.resolve`, the admin's user id as actor, and metadata `resolution`="refund buyer", `order_id` and `status`="resolved"
- **VERIFIED BY**: platform-e2e `engagement/dispute_audit.feature` "Resolving a dispute leaves an audit event" (uses `assert_one_event`)

#### Scenario: Dispute audit: Resolution is refused while team-audit is down end to end
- **WHEN** team-audit is stopped and an admin resolves an open dispute through the gateway
- **THEN** the call returns `Unavailable`, GetDispute still shows status open with an empty resolution, and after team-audit is started again QueryAuditLog for the dispute returns no `dispute.resolve` event
- **VERIFIED BY**: platform-e2e `engagement/dispute_audit.feature` "Resolution is refused while audit is down" (tagged @destructive; uses `stop_audit(world)` / `start_audit(world)`, asserts with `assert_no_event`)

#### Scenario: Dispute audit: A buyer cannot resolve a dispute end to end
- **WHEN** the buyer who opened the dispute calls ResolveDispute through the gateway
- **THEN** the call is refused with `PermissionDenied`, the dispute stays open and QueryAuditLog for the dispute returns no event
- **VERIFIED BY**: platform-e2e `engagement/dispute_audit.feature` "A buyer cannot resolve a dispute"

#### Scenario: Dispute audit: Resolving a closed dispute writes no second event end to end
- **WHEN** an admin resolves a dispute and then resolves the same dispute again
- **THEN** the second call returns `FailedPrecondition` and QueryAuditLog for the dispute still returns exactly one event
- **VERIFIED BY**: platform-e2e `engagement/dispute_audit.feature` "A closed dispute is not audited twice"
