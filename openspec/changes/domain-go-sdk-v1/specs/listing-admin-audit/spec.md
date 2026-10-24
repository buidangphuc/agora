## MODIFIED Requirements

### Requirement: Admin-scope update of a listing is audited first

`ListingService.UpdateListing` called by a caller holding the `admin` scope (whether or not that caller owns the listing) SHALL, after the listing is loaded and before it is changed, write one team-audit event with `actor_id` equal to the caller's verified principal id, `action` `listing.admin_update`, `target_type` `listing`, `target_id` the listing id and metadata `seller_id` (the stored owner) and `status` (the lowercase stored-form status string the update applies (`draft`, `published`, `rejected`, as team-domain stores it, see `statusPublished` in handler/listing.go); when the request carries `LISTING_STATUS_UNSPECIFIED` it is the listing's current stored status). The event SHALL be written by team-domain as service principal `service-team-domain` holding `audit.write`.

#### Scenario: Listing audit: Admin update writes one audit event
- **WHEN** an admin calls UpdateListing on an existing listing and changes its status to `rejected`
- **THEN** the RPC succeeds, the listing has status `rejected`, and team-audit holds exactly 1 event with the admin's user id as `actor_id`, `action` `listing.admin_update`, `target_type` `listing`, the listing id as `target_id`, and metadata `seller_id` = the owner id and `status` = `rejected`
- **VERIFIED BY**: team-domain `internal/rpc` `TestUpdateListingAdminWritesAuditBeforeChange`

#### Scenario: Listing audit: Update without a status records the stored status
- **WHEN** an admin calls UpdateListing with `listing.status` unspecified on a listing whose stored status is `published`
- **THEN** the audit event metadata `status` is `published`
- **VERIFIED BY**: team-domain `internal/rpc` `TestUpdateListingAdminAuditStatusFallsBackToStored`

#### Scenario: Listing audit: Audit is written before the change
- **WHEN** an admin calls UpdateListing and the audit write is recorded by a fake writer
- **THEN** the fake receives the audit call before the repository update is invoked and the event carries the stored owner id, not a request-supplied one
- **VERIFIED BY**: team-domain `internal/listing` `TestUpdateWithEventAuditPrecedesUpdate`

#### Scenario: Listing audit: Audit down refuses the admin update
- **WHEN** team-audit returns an error, and in a second case a deadline-exceeded, while an admin calls UpdateListing
- **THEN** each case returns `Unavailable`, the listing row is unchanged, and no `ListingChanged` outbox row is written
- **VERIFIED BY**: team-domain `internal/rpc` `TestUpdateListingAdminAuditDownUnavailableNoChange`

#### Scenario: Listing audit: Update failure after audit keeps the audit row
- **WHEN** the audit write succeeds and the repository update then fails with an internal error
- **THEN** the RPC returns `Internal` and the audit event already written is not retracted
- **VERIFIED BY**: team-domain `internal/listing` `TestUpdateWithEventRepoFailureAfterAudit`

#### Scenario: Listing audit: No audit upstream configured
- **WHEN** settings load with `ENV=local` and `UPSTREAM_AUDIT_ADDR` unset (everything else valid)
- **THEN** `Load` returns an error naming `UPSTREAM_AUDIT_ADDR`, so the service never starts without an audit upstream and no admin action can run unaudited
- **VERIFIED BY**: team-domain `internal/config` `TestUpstreamAddressesRequiredInEveryEnv`

#### Scenario: Listing audit: Unknown listing writes no audit event
- **WHEN** an admin calls UpdateListing with an id that does not exist
- **THEN** the RPC returns `NotFound` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/listing` `TestUpdateWithEventNotFoundNoAudit`

#### Scenario: Listing audit: Empty listing id writes no audit event
- **WHEN** an admin calls UpdateListing with an empty `listing.id`
- **THEN** the RPC returns `InvalidArgument` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/rpc` `TestUpdateListingEmptyIDNoAudit`

#### Scenario: Listing audit: Seller without admin scope cannot update another seller's listing
- **WHEN** a seller who does not own the listing and lacks the `admin` scope calls UpdateListing
- **THEN** the RPC returns `PermissionDenied`, the listing is unchanged, and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/rpc` `TestUpdateListingNonOwnerNonAdminDeniedNoAudit`

#### Scenario: Listing audit: Caller without listing.write is denied before audit
- **WHEN** a caller lacking the `listing.write` scope calls UpdateListing
- **THEN** the RPC returns `PermissionDenied` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/rpc` `TestUpdateListingMissingScopeNoAudit`

### Requirement: team-domain presents a service principal to team-audit

The team-domain audit writer SHALL call `WriteAuditEvent` through `auditx.Dial` with outgoing metadata `x-principal-id=service-team-domain`, `x-principal-type=service`, `x-principal-scopes=audit.write`, replacing any principal on the incoming context, SHALL bound each write by the SDK audit bound of 2 s (the earlier of that and the caller's deadline), and SHALL NOT retry. `UPSTREAM_CALL_TIMEOUT_SECONDS` SHALL NOT be read. `UPSTREAM_AUDIT_ADDR` SHALL be required in every `ENV` (an empty value refuses start-up naming the variable), so no nil writer and no unset-audit mode exists. Any write failure or timeout SHALL make the audited action fail closed with `Unavailable` and the message `audit trail unavailable`.

#### Scenario: Listing audit: Audit is called as the service principal
- **WHEN** the writer writes an event from a context carrying an admin user principal
- **THEN** the fake team-audit server receives `x-principal-id` `service-team-domain`, `x-principal-type` `service`, scopes `audit.write`, and the admin's id only in `actor_id`
- **VERIFIED BY**: team-domain `internal/audit` `TestAuditWriterUsesServicePrincipal`

#### Scenario: Listing audit: Slow audit is cut off at the timeout
- **WHEN** team-audit does not answer within 2 s (table cases: error response, deadline exceeded)
- **THEN** an error response returns `Unavailable` "audit trail unavailable" immediately, a deadline exceeded returns it within 2 s + 500 ms, and both make exactly 1 attempt
- **VERIFIED BY**: team-domain `internal/audit` `TestAuditGateTimeoutSingleAttempt`

#### Scenario: Listing audit: Caller principal is never forwarded
- **WHEN** a non-admin seller's context principal reaches the audit writer through any code path
- **THEN** the outgoing metadata never contains that seller's id or scopes
- **VERIFIED BY**: team-domain `internal/audit` `TestAuditWriterNeverForwardsCallerPrincipal`

#### Scenario: Listing audit: WriteAuditEvent is not reachable from the edge
- **WHEN** an anonymous or authenticated client calls `AuditService/WriteAuditEvent` through the gateway
- **THEN** the gateway returns 501 unimplemented, so a caller cannot forge an event
- **VERIFIED BY**: team-gateway `internal/edge` `TestInternalRPCsAreNotRoutedAtTheEdge` (existing, unchanged)

#### Scenario: Listing audit: Config exposes the audit address
- **WHEN** settings load with `UPSTREAM_AUDIT_ADDR` set, and again with it unset under `ENV=local`
- **THEN** the first has the address and the second returns an error naming `UPSTREAM_AUDIT_ADDR`; `.env.example` lists `UPSTREAM_AUDIT_ADDR` and no longer lists `UPSTREAM_CALL_TIMEOUT_SECONDS` (env drift gate passes)
- **VERIFIED BY**: team-domain `internal/config` `TestUpstreamAuditConfig`

#### Scenario: Listing audit: A request id reaches the audit call and the produced record
- **WHEN** an admin calls UpdateListing with inbound metadata `x-request-id` `req-42` and a `traceparent`, and the audit fake and the Kafka fake record what they receive
- **THEN** the audit call metadata carries `x-request-id` `req-42` and the same trace id, and the `ListingChanged` record relayed from the outbox row carries `x-request-id` `req-42` and a `traceparent` with the same trace id
- **VERIFIED BY**: team-domain `internal/app` `TestRequestIDAndTraceparentContinuity`

### Requirement: Non-admin owner path is unaudited and independent of team-audit

An update or delete by the caller whose id equals the listing's stored `seller_id` and who lacks the `admin` scope SHALL NOT call team-audit, SHALL behave exactly as before this change, and SHALL NOT be affected by team-audit availability.

#### Scenario: Listing audit: Owner update writes no audit event
- **WHEN** a seller without the `admin` scope updates their own listing while team-audit is reachable
- **THEN** the RPC succeeds and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/rpc` `TestUpdateListingOwnerNoAudit`

#### Scenario: Listing audit: Owner update succeeds while audit is down
- **WHEN** a seller without the `admin` scope updates or deletes their own listing and team-audit is stopped
- **THEN** each RPC succeeds and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/rpc` `TestOwnerUpdateIndependentOfAudit`

#### Scenario: Listing audit: Admin who owns the listing is audited
- **WHEN** a caller with the `admin` scope updates a listing whose stored `seller_id` equals the caller's id
- **THEN** the fake audit writer receives exactly 1 `listing.admin_update` call with `seller_id` equal to the caller's id
- **VERIFIED BY**: team-domain `internal/listing` `TestUpdateWithEventAdminOwnerIsAudited`

#### Scenario: Listing audit: Admin who owns the listing is refused while audit is down
- **WHEN** a caller with the `admin` scope deletes their own listing while the audit writer returns an error
- **THEN** the RPC returns `Unavailable` and the listing still exists
- **VERIFIED BY**: team-domain `internal/listing` `TestDeleteWithEventAdminOwnerAuditDownUnavailable`

#### Scenario: Listing audit: Non-owner without admin is refused and unaudited while audit is down
- **WHEN** a seller who does not own the listing and lacks the `admin` scope calls UpdateListing while team-audit is down
- **THEN** the RPC returns `PermissionDenied` (not `Unavailable`) and the listing is unchanged
- **VERIFIED BY**: team-domain `internal/rpc` `TestNonOwnerNonAdminDeniedNotUnavailable`
