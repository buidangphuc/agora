## ADDED Requirements

### Requirement: Admin-scope update of a listing is audited first

`ListingService.UpdateListing` called by a caller holding the `admin` scope (whether or not that caller owns the listing) SHALL, after the listing is loaded and before it is changed, write one team-audit event with `actor_id` equal to the caller's verified principal id, `action` `listing.admin_update`, `target_type` `listing`, `target_id` the listing id and metadata `seller_id` (the stored owner) and `status` (the lowercase stored-form status string the update applies (`draft`, `published`, `rejected`, as team-domain stores it, see `statusPublished` in handler/listing.go); when the request carries `LISTING_STATUS_UNSPECIFIED` it is the listing's current stored status). The event SHALL be written by team-domain as service principal `service-team-domain` holding `audit.write`.

#### Scenario: Listing audit: Admin update writes one audit event
- **WHEN** an admin calls UpdateListing on an existing listing and changes its status to `rejected`
- **THEN** the RPC succeeds, the listing has status `rejected`, and team-audit holds exactly 1 event with the admin's user id as `actor_id`, `action` `listing.admin_update`, `target_type` `listing`, the listing id as `target_id`, and metadata `seller_id` = the owner id and `status` = `rejected`
- **VERIFIED BY**: team-domain `internal/handler` `TestUpdateListingAdminWritesAuditBeforeChange`

#### Scenario: Listing audit: Update without a status records the stored status
- **WHEN** an admin calls UpdateListing with `listing.status` unspecified on a listing whose stored status is `published`
- **THEN** the audit event metadata `status` is `published`
- **VERIFIED BY**: team-domain `internal/handler` `TestUpdateListingAdminAuditStatusFallsBackToStored`

#### Scenario: Listing audit: Audit is written before the change
- **WHEN** an admin calls UpdateListing and the audit write is recorded by a fake writer
- **THEN** the fake receives the audit call before the repository update is invoked and the event carries the stored owner id, not a request-supplied one
- **VERIFIED BY**: team-domain `internal/service` `TestUpdateWithEventAuditPrecedesUpdate`

#### Scenario: Listing audit: Audit down refuses the admin update
- **WHEN** team-audit returns an error, and in a second case a deadline-exceeded, while an admin calls UpdateListing
- **THEN** each case returns `Unavailable`, the listing row is unchanged, and no `ListingChanged` outbox row is written
- **VERIFIED BY**: team-domain `internal/handler` `TestUpdateListingAdminAuditDownUnavailableNoChange`

#### Scenario: Listing audit: Update failure after audit keeps the audit row
- **WHEN** the audit write succeeds and the repository update then fails with an internal error
- **THEN** the RPC returns `Internal` and the audit event already written is not retracted
- **VERIFIED BY**: team-domain `internal/service` `TestUpdateWithEventRepoFailureAfterAudit`

#### Scenario: Listing audit: No audit upstream configured
- **WHEN** an admin calls UpdateListing while `UPSTREAM_AUDIT_ADDR` is unset
- **THEN** the RPC returns `Unavailable` and the listing row is unchanged
- **VERIFIED BY**: team-domain `internal/upstream` `TestNewAuditClientEmptyAddrIsNil` and team-domain `internal/service` `TestUpdateWithEventAdminNilAuditUnavailable`

#### Scenario: Listing audit: Unknown listing writes no audit event
- **WHEN** an admin calls UpdateListing with an id that does not exist
- **THEN** the RPC returns `NotFound` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/service` `TestUpdateWithEventNotFoundNoAudit`

#### Scenario: Listing audit: Empty listing id writes no audit event
- **WHEN** an admin calls UpdateListing with an empty `listing.id`
- **THEN** the RPC returns `InvalidArgument` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/handler` `TestUpdateListingEmptyIDNoAudit`

#### Scenario: Listing audit: Seller without admin scope cannot update another seller's listing
- **WHEN** a seller who does not own the listing and lacks the `admin` scope calls UpdateListing
- **THEN** the RPC returns `PermissionDenied`, the listing is unchanged, and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/handler` `TestUpdateListingNonOwnerNonAdminDeniedNoAudit`

#### Scenario: Listing audit: Caller without listing.write is denied before audit
- **WHEN** a caller lacking the `listing.write` scope calls UpdateListing
- **THEN** the RPC returns `PermissionDenied` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/handler` `TestUpdateListingMissingScopeNoAudit`

### Requirement: Admin-scope delete of a listing is audited first

`ListingService.DeleteListing` called by a caller holding the `admin` scope (whether or not that caller owns the listing) SHALL, after the listing is loaded and before it is deleted, write one team-audit event with `actor_id` equal to the caller's verified principal id, `action` `listing.admin_delete`, `target_type` `listing`, `target_id` the listing id and metadata `seller_id` (the stored owner).

#### Scenario: Listing audit: Admin delete writes one audit event
- **WHEN** an admin calls DeleteListing on an existing listing
- **THEN** the RPC succeeds, GetListing returns `NotFound`, and team-audit holds exactly 1 event with `action` `listing.admin_delete`, `target_type` `listing`, the listing id as `target_id`, the admin's user id as `actor_id` and metadata `seller_id` = the owner id
- **VERIFIED BY**: team-domain `internal/handler` `TestDeleteListingAdminWritesAuditBeforeChange`

#### Scenario: Listing audit: Audit down refuses the admin delete
- **WHEN** team-audit returns an error, and in a second case a deadline-exceeded, while an admin calls DeleteListing
- **THEN** each case returns `Unavailable`, the listing still exists, and no `ListingChanged` outbox row is written
- **VERIFIED BY**: team-domain `internal/handler` `TestDeleteListingAdminAuditDownUnavailableNoChange`

#### Scenario: Listing audit: Delete failure after audit keeps the audit row
- **WHEN** the audit write succeeds and the repository delete then fails with an internal error
- **THEN** the RPC returns `Internal` and the audit event already written is not retracted
- **VERIFIED BY**: team-domain `internal/service` `TestDeleteWithEventRepoFailureAfterAudit`

#### Scenario: Listing audit: Unknown listing on delete writes no audit event
- **WHEN** an admin calls DeleteListing with an id that does not exist
- **THEN** the RPC returns `NotFound` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/service` `TestDeleteWithEventNotFoundNoAudit`

#### Scenario: Listing audit: Empty id on delete writes no audit event
- **WHEN** an admin calls DeleteListing with an empty `id`
- **THEN** the RPC returns `InvalidArgument` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/handler` `TestDeleteListingEmptyIDNoAudit`

#### Scenario: Listing audit: Seller without admin scope cannot delete another seller's listing
- **WHEN** a seller who does not own the listing and lacks the `admin` scope calls DeleteListing
- **THEN** the RPC returns `PermissionDenied`, the listing still exists, and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/handler` `TestDeleteListingNonOwnerNonAdminDeniedNoAudit`

### Requirement: Non-admin owner path is unaudited and independent of team-audit

An update or delete by the caller whose id equals the listing's stored `seller_id` and who lacks the `admin` scope SHALL NOT call team-audit, SHALL behave exactly as before this change, and SHALL NOT be affected by team-audit availability or by `UPSTREAM_AUDIT_ADDR` being unset.

#### Scenario: Listing audit: Owner update writes no audit event
- **WHEN** a seller without the `admin` scope updates their own listing while team-audit is reachable
- **THEN** the RPC succeeds and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/handler` `TestUpdateListingOwnerNoAudit`

#### Scenario: Listing audit: Owner update succeeds while audit is down
- **WHEN** a seller without the `admin` scope updates or deletes their own listing and team-audit is stopped or `UPSTREAM_AUDIT_ADDR` is unset
- **THEN** each RPC succeeds and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/handler` `TestOwnerUpdateDeleteIndependentOfAudit`

#### Scenario: Listing audit: Admin who owns the listing is audited
- **WHEN** a caller with the `admin` scope updates a listing whose stored `seller_id` equals the caller's id
- **THEN** the fake audit writer receives exactly 1 `listing.admin_update` call with `seller_id` equal to the caller's id
- **VERIFIED BY**: team-domain `internal/service` `TestUpdateWithEventAdminOwnerIsAudited`

#### Scenario: Listing audit: Admin who owns the listing is refused while audit is down
- **WHEN** a caller with the `admin` scope deletes their own listing while the audit writer returns an error
- **THEN** the RPC returns `Unavailable` and the listing still exists
- **VERIFIED BY**: team-domain `internal/service` `TestDeleteWithEventAdminOwnerAuditDownUnavailable`

#### Scenario: Listing audit: Non-owner without admin is refused and unaudited while audit is down
- **WHEN** a seller who does not own the listing and lacks the `admin` scope calls UpdateListing while team-audit is down
- **THEN** the RPC returns `PermissionDenied` (not `Unavailable`) and the listing is unchanged
- **VERIFIED BY**: team-domain `internal/handler` `TestNonOwnerNonAdminDeniedNotUnavailable`

### Requirement: team-domain presents a service principal to team-audit

The team-domain audit client SHALL call `WriteAuditEvent` with outgoing metadata `x-principal-id=service-team-domain`, `x-principal-type=service`, `x-principal-scopes=audit.write`, replacing any principal on the incoming context, SHALL bound each call by `UPSTREAM_CALL_TIMEOUT_SECONDS`, and SHALL NOT retry. An empty `UPSTREAM_AUDIT_ADDR` SHALL yield a nil client whose use returns an error.

#### Scenario: Listing audit: Audit is called as the service principal
- **WHEN** the client writes an event from a context carrying an admin user principal
- **THEN** the fake team-audit server receives `x-principal-id` `service-team-domain`, `x-principal-type` `service`, scopes `audit.write`, and the admin's id only in `actor_id`
- **VERIFIED BY**: team-domain `internal/upstream` `TestAuditClientUsesServicePrincipal`

#### Scenario: Listing audit: Slow audit is cut off at the timeout
- **WHEN** team-audit does not answer within the configured timeout (2 s default, table cases: error response, deadline exceeded)
- **THEN** an error response returns the error immediately, a deadline exceeded returns an error within 2 s + 500 ms, and both make exactly 1 attempt
- **VERIFIED BY**: team-domain `internal/upstream` `TestAuditClientTimeoutSingleAttempt`

#### Scenario: Listing audit: Caller principal is never forwarded
- **WHEN** a non-admin seller's context principal reaches the audit client through any code path
- **THEN** the outgoing metadata never contains that seller's id or scopes
- **VERIFIED BY**: team-domain `internal/upstream` `TestAuditClientNeverForwardsCallerPrincipal`

#### Scenario: Listing audit: WriteAuditEvent is not reachable from the edge
- **WHEN** an anonymous or authenticated client calls `AuditService/WriteAuditEvent` through the gateway
- **THEN** the gateway returns 501 unimplemented, so a caller cannot forge an event
- **VERIFIED BY**: team-gateway `internal/edge` `TestInternalRPCsAreNotRoutedAtTheEdge` (existing, unchanged)

#### Scenario: Listing audit: Config exposes the audit address
- **WHEN** settings load with `UPSTREAM_AUDIT_ADDR` set, and again with it unset
- **THEN** the first has the address and the second is empty, and `.env.example` lists both new keys (env drift gate passes)
- **VERIFIED BY**: team-domain `internal/config` `TestUpstreamAuditConfig`

### Requirement: Admin listing audit is observable end to end

Through the gateway, an admin update and an admin delete of another seller's listing SHALL each produce one team-audit event queryable by `target_type` `listing` and the listing id, and SHALL be refused with the listing unchanged while team-audit is stopped.

#### Scenario: Listing audit: Admin update and delete are audited end to end
- **WHEN** an admin updates then deletes a seeded seller's listing through the gateway
- **THEN** `assert_one_event` finds, for `target_type` `listing` and that `target_id`, one event of each action, `listing.admin_update` and `listing.admin_delete`, each with the admin's user id as `actor_id` and `seller_id` = the seller's id
- **VERIFIED BY**: platform-e2e `features/seller/listing_admin_audit.feature` "Admin edit and delete of a seller's listing are audited"

#### Scenario: Listing audit: Owner update is not audited end to end
- **WHEN** the seller updates their own listing through the gateway
- **THEN** the update succeeds and `assert_no_event` finds none for that `target_id`
- **VERIFIED BY**: platform-e2e `features/seller/listing_admin_audit.feature` "Owner edit of own listing is not audited"

#### Scenario: Listing audit: Admin edit is refused while audit is down end to end
- **WHEN** team-audit is stopped and an admin updates or deletes another seller's listing through the gateway
- **THEN** each call returns `unavailable` and GetListing still returns the original title, and after team-audit restarts `assert_no_event` finds none for that listing
- **VERIFIED BY**: platform-e2e `features/seller/listing_admin_audit.feature` "Admin edit is refused while audit is down"

#### Scenario: Listing audit: Seller cannot edit another seller's listing end to end
- **WHEN** a seller without the `admin` scope updates another seller's listing through the gateway
- **THEN** the call returns `permission_denied`, the listing is unchanged, and `assert_no_event` finds none for that listing
- **VERIFIED BY**: platform-e2e `features/seller/listing_admin_audit.feature` "Non-admin seller cannot edit another seller's listing"
