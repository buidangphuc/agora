## MODIFIED Requirements

### Requirement: Admins close a report with an audited note

`EngagementService.ResolveReport` SHALL require the `admin` scope, accept a final `status` of `ACTIONED` or `DISMISSED` and a `resolution_note` of 1-500 characters after trimming, write one team-audit event with `action=report.resolve`, `target_type=report`, `target_id` equal to the report id, `actor_id` equal to the admin's user id and `metadata` holding `status`, `listing_id` and `resolution_note` (never `reporter_id` or `note`), and only then set status, `resolved_at`, `resolved_by` and `resolution_note`. If the audit write fails or times out it SHALL return `Unavailable` and leave the report `OPEN`; `UPSTREAM_AUDIT_ADDR` is required in every `ENV`, so there is no unset-upstream mode. A report that is already `ACTIONED` or `DISMISSED` SHALL be refused with `FailedPrecondition` whatever the requested status, with no audit write and no change (contract rule in `q6-trust-contract`). Closing SHALL not change the listing.

#### Scenario: Listing reports: Admin closes a report as actioned
- **WHEN** an admin calls ResolveReport on an `OPEN` report with status `ACTIONED` and note "đã gỡ tin"
- **THEN** the response returns the report with status `ACTIONED`, `resolved_at` set and `resolution_note` "đã gỡ tin"
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveReportActioned`

#### Scenario: Listing reports: Admin dismisses a report
- **WHEN** an admin calls ResolveReport on an `OPEN` report with status `DISMISSED` and note "không vi phạm"
- **THEN** the report has status `DISMISSED` and ListReports with status `OPEN` no longer contains it
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveReportDismissed`

#### Scenario: Listing reports: Audit event carries the required fields
- **WHEN** admin "adm-1" closes report "rp-1" of listing "L-1" as `ACTIONED` with note "đã gỡ tin"
- **THEN** exactly one WriteAuditEvent call is made with action `report.resolve`, target_type `report`, target_id "rp-1", actor_id "adm-1" and metadata `status`="ACTIONED", `listing_id`="L-1", `resolution_note`="đã gỡ tin", and no metadata key carries the reporter id or the report note
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveReportAuditEventFields`

#### Scenario: Listing reports: Audit is called as the service principal
- **WHEN** the audit client sends the `report.resolve` event
- **THEN** the outgoing metadata principal is `service-team-engagement`, type `service`, scope `audit.write`, and the admin's own principal metadata is absent
- **VERIFIED BY**: team-engagement `internal/upstream` `TestAuditClientUsesServicePrincipalForReports`

#### Scenario: Listing reports: Audit is written before the close
- **WHEN** ResolveReport runs
- **THEN** the recorded call order is audit write, then repository close
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveReportAuditBeforeClose`

#### Scenario: Listing reports: Audit failure leaves the report open
- **WHEN** team-audit returns an error or times out during ResolveReport
- **THEN** the call returns `Unavailable`, the report status is still `OPEN`, `resolved_at` is NULL and it still appears in ListReports with status `OPEN`
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveReportAuditFailureFailsClosed`

#### Scenario: Listing reports: No audit upstream configured
- **WHEN** `Load` runs with `UPSTREAM_AUDIT_ADDR` unset in any `ENV` (the audit connection has no fallback, so ResolveReport can never run without one)
- **THEN** it returns an error that names `UPSTREAM_AUDIT_ADDR` and no server is built
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadRefusesEmptyAuditAddr`

#### Scenario: Listing reports: Close failure after audit is Internal and retryable
- **WHEN** the audit write succeeds and the repository close fails, then the call is retried with a healthy repository
- **THEN** the first call returns `Internal`, the retry returns OK, the report is closed, and the audit trail holds 2 `report.resolve` events for the report (at-least-once)
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveReportCloseFailureInternalThenRetry`

#### Scenario: Listing reports: Closing again with the same status is refused
- **WHEN** an admin closes a report as `DISMISSED` and calls ResolveReport again with `DISMISSED`
- **THEN** the second call returns `FailedPrecondition`, `resolved_at`, `resolved_by` and `resolution_note` keep their first values and no second audit event is written
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveReportSameStatusReCloseRefused`

#### Scenario: Listing reports: Closing with a different status is refused
- **WHEN** an admin calls ResolveReport with `ACTIONED` on a report already closed `DISMISSED`
- **THEN** the call returns `FailedPrecondition`, the report stays `DISMISSED` and no audit event is written
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveReportConflictingStatusRefused`

#### Scenario: Listing reports: Two admins close the same report at once
- **WHEN** two admins call ResolveReport concurrently, one with `ACTIONED` and one with `DISMISSED`
- **THEN** exactly one final status is stored, the call that lost returns `FailedPrecondition`, and the stored `resolved_by` is the winner
- **VERIFIED BY**: team-engagement `internal/repository` `TestCloseReportCompareAndSetOneWinner`

#### Scenario: Listing reports: Invalid final status is rejected
- **WHEN** an admin calls ResolveReport with status `OPEN` or unspecified
- **THEN** the call returns `InvalidArgument`, no audit event is written and the report stays `OPEN`
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveReportInvalidStatus`

#### Scenario: Listing reports: Empty or whitespace note is rejected
- **WHEN** an admin calls ResolveReport with note "" and then "   "
- **THEN** both calls return `InvalidArgument`, no audit event is written and the report stays `OPEN`
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveReportEmptyNoteInvalidArgument`

#### Scenario: Listing reports: Resolution note length boundary
- **WHEN** an admin closes one report with a 500-character note and another with a 501-character note
- **THEN** the 500-character call returns OK and the 501-character call returns `InvalidArgument`
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveReportNoteLengthBoundary`

#### Scenario: Listing reports: Unknown report
- **WHEN** an admin calls ResolveReport with an id that does not exist, and with an empty id
- **THEN** the unknown id returns `NotFound` and the empty id `InvalidArgument`, and no audit event is written
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveReportNotFoundAndEmptyID`

#### Scenario: Listing reports: Non-admin and anonymous callers cannot close
- **WHEN** a buyer with `engagement:write`, the reporter, and an anonymous caller each call ResolveReport
- **THEN** the buyer and reporter get `PermissionDenied`, the anonymous caller gets `Unauthenticated`, authorization is checked before validation, the report stays `OPEN` and the audit client receives 0 calls
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveReportAuthzMatrix`

#### Scenario: Listing reports: Closing a report leaves the listing alone
- **WHEN** an admin closes a report as `ACTIONED`
- **THEN** no call is made to any listing, order or other upstream except team-audit
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveReportTouchesOnlyAudit`

#### Scenario: Listing reports: Admin works a report end to end
- **WHEN** a buyer reports a listing, an admin lists open reports through the gateway and closes the report as `ACTIONED` with a note
- **THEN** the list contains the report, the close returns status `ACTIONED`, the report leaves the `OPEN` list, and QueryAuditLog returns one `report.resolve` event with the admin as actor and the report id as target
- **VERIFIED BY**: platform-e2e `features/engagement/listing_reports.feature` "An admin closes a report with an audited note"

#### Scenario: Listing reports: Audit outage blocks closing end to end
- **WHEN** team-audit is stopped and an admin closes a report through the gateway, then team-audit is started and the admin retries
- **THEN** the first call returns `unavailable` and the report is still `OPEN`; the retry returns `ACTIONED`
- **VERIFIED BY**: platform-e2e `features/engagement/listing_reports.feature` "Closing a report fails closed when audit is down"

#### Scenario: Listing reports: Buyer cannot work the queue end to end
- **WHEN** a buyer calls ListReports and ResolveReport through the gateway
- **THEN** both return `permission_denied`
- **VERIFIED BY**: platform-e2e `features/engagement/listing_reports.feature` "A buyer cannot list or close reports"
