# trust-contract Specification

## Purpose
Proto contract for Q6 trust and enforcement: listing moderation, ScreenListing, account admin and UserStatusChanged, listing reports, return disputes and the audit action catalogue.

## Requirements

### Requirement: Listing moderation statuses and ModerationInfo

`ListingStatus` in `platform/listing/v1/listing.proto` SHALL gain `LISTING_STATUS_PENDING_REVIEW = 4`, `LISTING_STATUS_BANNED = 5` and `LISTING_STATUS_HELD = 6`; values 0-3 SHALL NOT change. `Listing` SHALL gain `ModerationInfo moderation = 12` with `ModerationInfo { reason_code = 1, note = 2, screen_flags = 3 (repeated string), decided_at = 4 (google.protobuf.Timestamp) }`. The comments SHALL state that `moderation` and `status` are server-owned (`moderation` is ignored on writes; `status` on `CreateListing`/`UpdateListing` is a request, DRAFT or publish, and every other value is ignored; an unset `status` on `UpdateListing` keeps the stored status and is not a DRAFT request), that `screen_flags` is visible to admin only (the owner sees `reason_code` and `note` only), that `ListingChanged` events carry neither `note` nor `screen_flags`, that `reason_code` is one of `prohibited_item`, `counterfeit`, `misleading`, `inappropriate`, `duplicate`, `seller_enforcement`, `other`, and that `note` is at most 500 characters.

#### Scenario: Trust contract: New listing statuses have fixed numbers
- **WHEN** the compiled `ListingStatus` enum is inspected
- **THEN** values 0 to 3 keep their names and numbers and `PENDING_REVIEW=4`, `BANNED=5`, `HELD=6` exist
- **VERIFIED BY**: Go descriptor test `platform-core/packages/proto/platform/listing/v1/moderation_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: ModerationInfo fields are fixed
- **WHEN** the descriptor test reads `ModerationInfo` and `Listing`
- **THEN** `ModerationInfo` has exactly `reason_code=1`, `note=2`, `screen_flags=3` (repeated string), `decided_at=4` (Timestamp), and `Listing.moderation` is field 12 of type `ModerationInfo` with fields 1 to 11 unchanged
- **VERIFIED BY**: Go descriptor test `moderation_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Listing with moderation round-trips
- **WHEN** a `Listing` with status `LISTING_STATUS_HELD`, `moderation.reason_code=seller_enforcement`, two `screen_flags` and a `decided_at` is serialised and parsed
- **THEN** every field parses back equal, and a `Listing` serialised without field 12 parses with `moderation` unset
- **VERIFIED BY**: Go round-trip test `moderation_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Old readers skip the moderation field and tolerate new statuses
- **WHEN** bytes of a `Listing` with status 6 and field 12 set are parsed with a schema that knows only fields 1 to 11 and statuses 0 to 3
- **THEN** parsing succeeds, field 12 is skipped, and status 6 is kept as an unknown enum number without error
- **VERIFIED BY**: Go test `moderation_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Status is documented as server-owned
- **WHEN** a reader opens the comments of `Listing.status`, `Listing.moderation` and `ModerationInfo`
- **THEN** they state that `moderation` is ignored on writes, that a client `status` is only a request (DRAFT or publish) and an unset `status` on update keeps the stored status, that `screen_flags` is admin only and the owner sees `reason_code` and `note` only, that `ListingChanged` carries no note and no screen flags, the seven reason codes, and the 500-character note limit
- **VERIFIED BY**: comment-text assertion in `moderation_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Missing server-owned note is caught
- **WHEN** the test's comment helper runs against a fixture `Listing.status` comment that lacks "server-owned"
- **THEN** the helper reports failure (the test expects it), proving a missing note is caught
- **VERIFIED BY**: comment-text assertion in `moderation_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Seller cannot set server-owned fields
- **WHEN** a reader opens the `CreateListing`, `UpdateListing` and `DeleteListing` comments
- **THEN** they state that a seller cannot set server-owned fields: a `status` other than DRAFT or publish and any `moderation` value are dropped (no error), so a seller cannot publish past the screen or undo a rejection; that a seller write or delete on a BANNED or HELD listing is FAILED_PRECONDITION; that a seller edit of a REJECTED listing moves it to PENDING_REVIEW, never to PUBLISHED; that a content edit of a PUBLISHED listing is re-screened and a flagged edit moves it to PENDING_REVIEW; that an admin writing their own listing follows the seller path (screen runs, still audited); and that a caller who is not the owner or an admin is denied the write (PERMISSION_DENIED)
- **VERIFIED BY**: comment-text assertion in `moderation_compat_test.go` (tasks 2.1)

### Requirement: ListingService moderation RPCs

`ListingService` SHALL gain `ModerateListing` and `ListModerationQueue` with `enum ModerationDecision { UNSPECIFIED=0, APPROVE=1, REJECT=2, BAN=3 }`, `ModerateListingRequest { listing_id=1, decision=2, reason_code=3, note=4 }`, `ModerateListingResponse { listing=1 }`, `ListModerationQueueRequest { page=1 }`, `ModerationQueueItem { listing=1, submitted_at=2 }`, `ListModerationQueueResponse { items=1, page=2 }`. The RPC comments SHALL document: both are admin-only (scope `admin`, UNAUTHENTICATED without a principal, PERMISSION_DENIED otherwise) and edge-routed; `ModerateListing` writes the audit event `listing.moderate` first and returns `UNAVAILABLE` with no change when the audit sink is down; `INVALID_ARGUMENT` for an empty `listing_id`, `decision` UNSPECIFIED, a missing `reason_code` on REJECT or BAN, a `reason_code` outside the closed set, or a `note` over 500 characters; `NOT_FOUND` for an unknown listing; `FAILED_PRECONDITION` when the transition is not allowed (APPROVE only from PENDING_REVIEW; REJECT and BAN from PENDING_REVIEW or PUBLISHED; any decision on DRAFT, HELD, REJECTED or BANNED, so a rejection or ban is never reversed by APPROVE); each status change emits `ListingChanged(UPDATED)` without the note or screen flags; `ListModerationQueue` returns PENDING_REVIEW listings oldest submission first, is a read and is not audited.

#### Scenario: Trust contract: Moderation RPCs exist on ListingService
- **WHEN** the compiled `ListingService` descriptor is inspected
- **THEN** every pre-existing RPC is unchanged and `ModerateListing` and `ListModerationQueue` exist with the request and response types above
- **VERIFIED BY**: Go descriptor test `moderation_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Moderation messages have fixed numbers
- **WHEN** the descriptor test reads the new messages and `ModerationDecision`
- **THEN** every field name, number and type equals the list above, the enum numbers are 0 to 3, and no other field exists
- **VERIFIED BY**: Go descriptor test `moderation_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Moderation request round-trips
- **WHEN** a `ModerateListingRequest` with decision `MODERATION_DECISION_BAN`, `reason_code=counterfeit` and a 500-character note is serialised and parsed
- **THEN** every field parses back equal
- **VERIFIED BY**: Go round-trip test `moderation_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Moderation authz and audit are documented
- **WHEN** a reader opens the comments of `ModerateListing` and `ListModerationQueue`
- **THEN** both state admin-only with UNAUTHENTICATED and PERMISSION_DENIED, `ModerateListing` states audit action `listing.moderate` first and `UNAVAILABLE` with no change when the sink is down, and `ListModerationQueue` states oldest first and not audited
- **VERIFIED BY**: comment-text assertion in `moderation_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Moderation errors and transitions are documented
- **WHEN** a reader opens the `ModerateListing` comment
- **THEN** it names INVALID_ARGUMENT (empty listing_id, UNSPECIFIED decision, missing or unknown reason_code, note over 500), NOT_FOUND, and FAILED_PRECONDITION with the allowed transitions (APPROVE from PENDING_REVIEW only, REJECT and BAN from PENDING_REVIEW or PUBLISHED, nothing from DRAFT, HELD, REJECTED or BANNED) and the `ListingChanged(UPDATED)` emission without note or screen flags
- **VERIFIED BY**: comment-text assertion in `moderation_compat_test.go` (tasks 2.1)

### Requirement: AIService.ScreenListing

`AIService` in `platform/ai/v1/ai.proto` SHALL gain `ScreenListing(ScreenListingRequest) returns (ScreenListingResponse)` with `ScreenListingRequest { listing_id=1, title=2, description=3, category_id=4 }`, `enum ScreenVerdict { UNSPECIFIED=0, ALLOW=1, REVIEW=2 }` and `ScreenListingResponse { verdict=1, labels=2 (repeated string), backend=3 }`. The comment SHALL state that the RPC is internal (service principal with scope `ai.classify`, not routed by the gateway; a caller without that principal gets PERMISSION_DENIED and a call with no principal UNAUTHENTICATED), that `listing_id` is empty on create, that a successful response never has verdict UNSPECIFIED, that `backend` is `mock` or `llm_router`, that the `mock` backend returns ALLOW for text with no prohibited term, and that the caller treats an error or a timeout as REVIEW (fail safe). Existing RPCs and messages SHALL NOT change.

#### Scenario: Trust contract: ScreenListing exists on AIService
- **WHEN** the compiled `AIService` descriptor is inspected
- **THEN** the existing five RPCs are unchanged and `ScreenListing` exists with the request and response types above
- **VERIFIED BY**: Go descriptor test `platform-core/packages/proto/platform/ai/v1/screen_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Screen messages have fixed numbers
- **WHEN** the descriptor test reads `ScreenListingRequest`, `ScreenListingResponse` and `ScreenVerdict`
- **THEN** field names, numbers and types and the enum numbers 0 to 2 equal the list above and no other field exists
- **VERIFIED BY**: Go descriptor test `screen_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Screen response round-trips
- **WHEN** a `ScreenListingResponse` with `SCREEN_VERDICT_REVIEW`, labels `["weapon","keyword:x"]` and `backend=mock` is serialised and parsed, and a request with an empty `listing_id` and a 5,000-character description likewise
- **THEN** every field parses back equal, including the empty `listing_id`
- **VERIFIED BY**: Go round-trip test `screen_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: ScreenListing is documented as internal
- **WHEN** a reader opens the `ScreenListing` comment
- **THEN** it contains "scope ai.classify", states the gateway does not route it, PERMISSION_DENIED for other callers, UNAUTHENTICATED without a principal, the backend values `mock` and `llm_router`, that `mock` returns ALLOW for clean text, and that a caller treats an error or timeout as REVIEW
- **VERIFIED BY**: comment-text assertion in `screen_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Missing internal note is caught
- **WHEN** the helper runs against a fixture `ScreenListing` comment lacking "ai.classify"
- **THEN** the helper reports failure (the test expects it)
- **VERIFIED BY**: comment-text assertion in `screen_compat_test.go` (tasks 2.1)

### Requirement: AccountAdminService and UserStatusChanged

`platform/identity/v1/identity.proto` SHALL gain `service AccountAdminService { ListUsers; SetUserStatus }`, `enum UserStatus { UNSPECIFIED=0, ACTIVE=1, RESTRICTED=2, SUSPENDED=3, BANNED=4 }`, `AdminUser { id=1, username=2, roles=3, status=4, status_reason=5, status_until=6, created_at=7 }`, `ListUsersRequest { query=1, status=2, page=3 }`, `ListUsersResponse { users=1, page=2 }`, `SetUserStatusRequest { user_id=1, status=2, reason_code=3, note=4, until=5 }`, `SetUserStatusResponse { user=1, sessions_revoked=2 }` and `UserStatusChanged { user_id=1, status=2, previous_status=3, reason_code=4, until=5, changed_at=6 }`. Comments SHALL document: both RPCs admin-only (scope `admin`; UNAUTHENTICATED, PERMISSION_DENIED) and edge-routed; `ListUsers` `query` is a username prefix or an exact id, UNSPECIFIED status means all, and it is a read, not audited; `SetUserStatus` writes audit action `user.set_status` first and returns `UNAVAILABLE` with no change when the sink is down; `FAILED_PRECONDITION` for the caller's own id or an admin target; `INVALID_ARGUMENT` for status UNSPECIFIED, an empty or non-lower_snake_case or over-64-char `reason_code`, a `note` over 500 characters, `until` missing or not in the future for SUSPENDED, or `until` set for any other status; `NOT_FOUND` for an unknown user; every successful call emits one `UserStatusChanged`; a call setting RESTRICTED, SUSPENDED or BANNED (including re-setting the current status) revokes every session of the user and emits one `SessionRevoked` per session, while a call setting ACTIVE revokes none. `UserStatusChanged` SHALL be documented as published on `identity.events`, key = user id, envelope type `platform.identity.v1.UserStatusChanged`. The comments on `AuthService.Login` and `RefreshSession` SHALL state that a SUSPENDED or BANNED user gets `PERMISSION_DENIED` with one generic message that does not tell suspended from banned and does not reveal an end time; the `Login` comment SHALL state that a SUSPENDED user whose `until` has passed is lifted to ACTIVE at that login (`RefreshSession` never lifts); existing messages SHALL NOT change.

#### Scenario: Trust contract: AccountAdminService exists
- **WHEN** the compiled identity descriptor is inspected
- **THEN** `AccountAdminService` has exactly `ListUsers` and `SetUserStatus` with the types above, and the existing services keep their RPCs
- **VERIFIED BY**: Go descriptor test `platform-core/packages/proto/platform/identity/v1/userstatus_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: User status messages have fixed numbers
- **WHEN** the descriptor test reads `AdminUser`, the four request and response messages, `UserStatusChanged` and `UserStatus`
- **THEN** every field name, number and type and the enum numbers 0 to 4 equal the lists above and no other field exists
- **VERIFIED BY**: Go descriptor test `userstatus_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: UserStatusChanged round-trips
- **WHEN** a `UserStatusChanged` with status SUSPENDED, previous ACTIVE, `reason_code=counterfeit_seller`, `until` and `changed_at` set is serialised and parsed, and a `SetUserStatusRequest` for BANNED with no `until` likewise
- **THEN** every field parses back equal and the unset `until` stays unset
- **VERIFIED BY**: Go round-trip test `userstatus_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Old readers tolerate unknown user status
- **WHEN** bytes of an `AdminUser` with status 4 are parsed with a schema that knows only statuses 0 to 2
- **THEN** parsing succeeds and status 4 is kept as an unknown enum number
- **VERIFIED BY**: Go test `userstatus_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: SetUserStatus authz, audit and errors are documented
- **WHEN** a reader opens the `SetUserStatus` comment
- **THEN** it states admin-only, audit `user.set_status` first with `UNAVAILABLE` and no change when the sink is down, FAILED_PRECONDITION for self or an admin target, INVALID_ARGUMENT for each case listed above, NOT_FOUND, one `UserStatusChanged` per call, and session revocation for RESTRICTED, SUSPENDED or BANNED (also when re-set) but none for ACTIVE
- **VERIFIED BY**: comment-text assertion in `userstatus_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: ListUsers is documented as a read
- **WHEN** a reader opens the `ListUsers` comment
- **THEN** it states admin-only, `query` as username prefix or exact id, and that it is not audited
- **VERIFIED BY**: comment-text assertion in `userstatus_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Event and login refusal are documented
- **WHEN** a reader opens the `UserStatusChanged` comment and the `Login` and `RefreshSession` comments
- **THEN** the event comment names topic `identity.events`, key = user id and the envelope type, and both auth comments state PERMISSION_DENIED with one generic message for a SUSPENDED or BANNED user, and the `Login` comment states that an expired suspension is lifted at that login
- **VERIFIED BY**: comment-text assertion in `userstatus_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Non-admin callers are denied user administration
- **WHEN** a reader opens the `AccountAdminService` comment
- **THEN** it states that a caller without scope `admin` gets PERMISSION_DENIED and a caller without a principal gets UNAUTHENTICATED on both RPCs
- **VERIFIED BY**: comment-text assertion in `userstatus_compat_test.go` (tasks 2.1)

### Requirement: Listing reports

`platform/engagement/v1/engagement.proto` SHALL gain `EngagementService.CreateReport`, `ListReports` and `ResolveReport`, `enum ReportTargetType { UNSPECIFIED=0, LISTING=1 }`, `enum ReportStatus { UNSPECIFIED=0, OPEN=1, ACTIONED=2, DISMISSED=3 }`, `Report { id=1, target_type=2, target_id=3, reason_code=4, note=5, reporter_id=6, status=7, created_at=8, resolved_at=9, resolution_note=10 }`, `CreateReportRequest { target_type=1, target_id=2, reason_code=3, note=4 }`, `CreateReportResponse { report=1 }`, `ListReportsRequest { status=1, target_id=2, page=3 }`, `ListReportsResponse { reports=1, page=2 }`, `ResolveReportRequest { report_id=1, status=2, resolution_note=3 }`, `ResolveReportResponse { report=1 }`. Comments SHALL document: `CreateReport` needs a signed-in user (UNAUTHENTICATED for anonymous); `INVALID_ARGUMENT` for target type UNSPECIFIED, a `target_id` that is not a UUID, a `reason_code` outside `prohibited_item | counterfeit | scam | misleading | inappropriate | other`, or a `note` over 500 characters; `FAILED_PRECONDITION` when the caller owns the listing; `RESOURCE_EXHAUSTED` when the caller already has 20 OPEN reports; idempotent per (reporter, target) while OPEN (returns the existing report); the target is not checked for existence; the reporter's identity is returned to the reporter and to admin only, never to the reported seller; `ListReports` and `ResolveReport` are admin-only, `ListReports` is oldest first and not audited, UNSPECIFIED status means all; `ResolveReport` writes audit action `report.resolve` first (`UNAVAILABLE`, no change when the sink is down), accepts only ACTIONED or DISMISSED (else INVALID_ARGUMENT), requires a `resolution_note` of 1 to 500 characters after trim, returns NOT_FOUND for an unknown report and FAILED_PRECONDITION for an already closed report.

#### Scenario: Trust contract: Report RPCs exist on EngagementService
- **WHEN** the compiled `EngagementService` descriptor is inspected
- **THEN** every pre-existing RPC is unchanged and `CreateReport`, `ListReports`, `ResolveReport` exist with the types above
- **VERIFIED BY**: Go descriptor test `platform-core/packages/proto/platform/engagement/v1/trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Report messages have fixed numbers
- **WHEN** the descriptor test reads `Report`, the six request and response messages and the two enums
- **THEN** every field name, number and type and the enum numbers equal the lists above and no other field exists
- **VERIFIED BY**: Go descriptor test `trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Report round-trips
- **WHEN** a `Report` with status ACTIONED, a 500-character note, `created_at` and `resolved_at` set is serialised and parsed, and a `CreateReportRequest` with an empty note likewise
- **THEN** every field parses back equal, including the empty note
- **VERIFIED BY**: Go round-trip test `trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: CreateReport rules are documented
- **WHEN** a reader opens the `CreateReport` comment
- **THEN** it names UNAUTHENTICATED for anonymous, INVALID_ARGUMENT for each case above, FAILED_PRECONDITION for an own listing, RESOURCE_EXHAUSTED at 20 OPEN reports per reporter, idempotency per (reporter, target) while OPEN, no existence check, and the closed reason set of six codes
- **VERIFIED BY**: comment-text assertion in `trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Report admin RPCs are documented
- **WHEN** a reader opens the `ListReports` and `ResolveReport` comments
- **THEN** both state admin-only, `ListReports` states oldest first and not audited, `ResolveReport` states audit `report.resolve` first with `UNAVAILABLE`, the ACTIONED or DISMISSED rule, the note length, NOT_FOUND and FAILED_PRECONDITION for a closed report
- **VERIFIED BY**: comment-text assertion in `trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Reporter identity is documented as admin-only
- **WHEN** a reader opens the `Report.reporter_id` comment
- **THEN** it states that the id is returned to the reporter and to admin only, never to the reported seller
- **VERIFIED BY**: comment-text assertion in `trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Missing audit-first note is caught
- **WHEN** the helper runs against a fixture `ResolveReport` comment lacking "report.resolve"
- **THEN** the helper reports failure (the test expects it)
- **VERIFIED BY**: comment-text assertion in `trust_compat_test.go` (tasks 2.1)

### Requirement: Return disputes, rulings and DisputeResolved

`engagement.proto` SHALL gain `enum DisputeRuling { UNSPECIFIED=0, BUYER_FAVOUR=1, SELLER_FAVOUR=2 }`; `CreateDisputeRequest.return_id = 5`; `Dispute.return_id = 11` and `Dispute.ruling = 12`; `ResolveDisputeRequest.ruling = 4`; `EngagementService.ListOrderDisputes` with `ListOrderDisputesRequest { order_id=1 }` and `ListOrderDisputesResponse { disputes=1 }`; `EngagementService.ListDisputes` with `ListDisputesRequest { status=1, page=2 }` and `ListDisputesResponse { disputes=1, page=2 }`. `events.proto` SHALL gain `DisputeResolved { dispute_id=1, order_id=2, return_id=3, status=4 (DisputeStatus), ruling=5 (DisputeRuling), resolver_id=6, resolved_at=7 (Timestamp) }`. Existing fields (`Dispute` 1-10, `CreateDisputeRequest` 1-4, `ResolveDisputeRequest` 1-3) SHALL NOT change. Comments SHALL document: with `return_id`, the claimant must be the order's buyer and the defendant is the order's seller (a different supplied `defendant_id` is INVALID_ARGUMENT); `FAILED_PRECONDITION` when the return is not escalatable (`OrderReturn.can_escalate` false); `ALREADY_EXISTS` while another dispute on the same return is OPEN or INVESTIGATING (a new dispute after the previous one closed is allowed while `can_escalate` is true); `NOT_FOUND` for an unknown return; `UNAVAILABLE` when team-order cannot be reached; the `ResolveDispute` state machine (OPEN to INVESTIGATING or REJECTED; INVESTIGATING to INVESTIGATING (note update), RESOLVED or REJECTED; every other transition FAILED_PRECONDITION, in particular OPEN to RESOLVED and any move out of RESOLVED or REJECTED), that `ruling` is required for RESOLVED on a return-linked dispute and that a `ruling` sent with any status other than RESOLVED is INVALID_ARGUMENT, the existing audit action `dispute.resolve` written first, and that exactly one `DisputeResolved` is emitted per dispute reaching RESOLVED or REJECTED; `ListOrderDisputes` readable by the order's buyer, the order's seller or an admin (PERMISSION_DENIED otherwise, INVALID_ARGUMENT for an empty `order_id`), newest first; `ListDisputes` admin-only, oldest first, UNSPECIFIED status means all, not audited. The `DisputeResolved` comment SHALL state topic `engagement.events`, key = dispute id, envelope type `platform.engagement.v1.DisputeResolved`, and that it carries ids, status and ruling only (no free text).

#### Scenario: Trust contract: Dispute RPCs exist
- **WHEN** the compiled `EngagementService` descriptor is inspected
- **THEN** `ListOrderDisputes` and `ListDisputes` exist with the types above and `CreateDispute`, `GetDispute`, `ResolveDispute` are unchanged
- **VERIFIED BY**: Go descriptor test `trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Dispute fields have fixed numbers
- **WHEN** the descriptor test reads `Dispute`, `CreateDisputeRequest`, `ResolveDisputeRequest`, `DisputeRuling` and the list messages
- **THEN** `Dispute` has fields 1 to 10 unchanged plus `return_id=11` and `ruling=12`; `CreateDisputeRequest` has 1 to 4 plus `return_id=5`; `ResolveDisputeRequest` has 1 to 3 plus `ruling=4`; the enum numbers are 0 to 2; the list messages match the list above
- **VERIFIED BY**: Go descriptor test `trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Dispute with ruling round-trips
- **WHEN** a `Dispute` with `return_id`, status RESOLVED and ruling BUYER_FAVOUR is serialised and parsed, and a `ResolveDisputeRequest` with ruling SELLER_FAVOUR likewise
- **THEN** every field parses back equal, and a `Dispute` serialised without fields 11 and 12 parses with empty `return_id` and ruling UNSPECIFIED
- **VERIFIED BY**: Go round-trip test `trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Old readers skip the dispute additions
- **WHEN** bytes of a `Dispute` with fields 11 and 12 set are parsed with a schema that knows only fields 1 to 10
- **THEN** parsing succeeds and fields 11 and 12 are skipped
- **VERIFIED BY**: Go test `trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: DisputeResolved message is fixed
- **WHEN** the descriptor test reads `DisputeResolved` in `events.proto` and round-trips one with status RESOLVED, ruling BUYER_FAVOUR and a `resolved_at`
- **THEN** it has exactly the seven fields above and the round-trip is lossless, and the existing event messages in the file are unchanged
- **VERIFIED BY**: Go descriptor and round-trip test `trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Dispute escalation errors are documented
- **WHEN** a reader opens the `CreateDispute` comment
- **THEN** it names FAILED_PRECONDITION for `can_escalate` false, ALREADY_EXISTS while another dispute on the return is OPEN or INVESTIGATING (re-escalation after close allowed while `can_escalate` is true), NOT_FOUND for an unknown return, UNAVAILABLE when team-order is unreachable, the claimant-is-buyer rule and the defendant rule
- **VERIFIED BY**: comment-text assertion in `trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Dispute state machine is documented
- **WHEN** a reader opens the `ResolveDispute` comment
- **THEN** it lists the allowed transitions (including OPEN to REJECTED), states that OPEN to RESOLVED is FAILED_PRECONDITION, that `ruling` is required for RESOLVED on a return-linked dispute (INVALID_ARGUMENT) and that a `ruling` with any other status is INVALID_ARGUMENT, audit `dispute.resolve` first, and one `DisputeResolved` per dispute reaching RESOLVED or REJECTED
- **VERIFIED BY**: comment-text assertion in `trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Dispute listing authz is documented
- **WHEN** a reader opens the `ListOrderDisputes` and `ListDisputes` comments
- **THEN** `ListOrderDisputes` states the order's buyer, the order's seller or an admin with PERMISSION_DENIED for others and newest first, and `ListDisputes` states admin-only, oldest first and not audited
- **VERIFIED BY**: comment-text assertion in `trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: DisputeResolved delivery is documented
- **WHEN** a reader opens the `DisputeResolved` comment
- **THEN** it names topic `engagement.events`, key = dispute id, the envelope type, and that it carries no free text
- **VERIFIED BY**: comment-text assertion in `trust_compat_test.go` (tasks 2.1)

#### Scenario: Trust contract: Non-party callers are denied dispute reads
- **WHEN** a reader opens the `ListOrderDisputes` and `ListDisputes` comments
- **THEN** they state that a caller who is neither the order's buyer, the order's seller nor an admin gets PERMISSION_DENIED on `ListOrderDisputes` and that any non-admin gets PERMISSION_DENIED on `ListDisputes`
- **VERIFIED BY**: comment-text assertion in `trust_compat_test.go` (tasks 2.1)

### Requirement: Audit action catalogue lists the Q6 actions

The `WriteAuditEventRequest` comment in `platform/audit/v1/audit.proto` SHALL list 17 actions: the existing 13 plus `listing.moderate`, `user.set_status`, `report.resolve` and `return.dispute_refund`. No field, RPC or message changes.

#### Scenario: Trust contract: Catalogue lists the new actions
- **WHEN** the extended catalogue test reads the `WriteAuditEventRequest` comment
- **THEN** it contains all 17 action names and the heading says 17
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/audit/v1/auditquery_roundtrip_test.go` (tasks 2.2)

#### Scenario: Trust contract: Audit wire is unchanged
- **WHEN** the existing wire-unchanged test runs
- **THEN** `WriteAuditEventRequest`, `QueryAuditLogRequest` and `AuditEvent` keep their field names, numbers and types
- **VERIFIED BY**: existing Go test `auditquery_roundtrip_test.go` (tasks 2.2)

#### Scenario: Trust contract: Catalogue test rejects a missing action
- **WHEN** the catalogue helper runs against a fixture comment that lists only the original 13 actions
- **THEN** the helper reports failure naming `listing.moderate` (the test expects it)
- **VERIFIED BY**: Go test `auditquery_roundtrip_test.go` (tasks 2.2)

### Requirement: Change is backward compatible and copies match

All edits SHALL be additive: `buf lint` and `buf breaking` (FILE) against the base pass, and `make -C platform-core proto` generates the new RPCs, messages and enums in Go, Python, JVM and TypeScript. The source protos SHALL be copied byte-identical to the consumer trees listed in the proposal, and each consumer copy SHALL lint.

#### Scenario: Trust contract: Change is backward compatible
- **WHEN** `make -C platform-core lint-proto breaking` runs against the committed base
- **THEN** both pass with no finding
- **VERIFIED BY**: command run recorded in tasks 1.6 (`make -C platform-core lint-proto breaking`)

#### Scenario: Trust contract: Wire-breaking edit is rejected
- **WHEN** a field of a touched message is renumbered in a scratch copy and `buf breaking` runs against the base
- **THEN** it fails and names the field
- **VERIFIED BY**: scratch-copy run of `buf breaking`, recorded in tasks 1.6

#### Scenario: Trust contract: New symbols in every generated language
- **WHEN** `make -C platform-core proto` completes
- **THEN** the Go, Python, JVM and TypeScript outputs each expose `ModerateListing`, `ListModerationQueue`, `ScreenListing`, `AccountAdminService`, `UserStatusChanged`, `CreateReport`, `ListReports`, `ResolveReport`, `ListOrderDisputes`, `ListDisputes` and `DisputeResolved`
- **VERIFIED BY**: grep over `platform-core/gen` outputs, recorded in tasks 2.3

#### Scenario: Trust contract: Copies match the source
- **WHEN** `cmp` compares each listed consumer copy with its source proto
- **THEN** every pair is identical
- **VERIFIED BY**: one-off `cmp` loop recorded in tasks 3.1

#### Scenario: Trust contract: Drifted or missing copy is rejected
- **WHEN** the same loop runs against a consumer tree with one byte changed or a copy deleted
- **THEN** it reports a mismatch or a missing file and exits non-zero (a missing file is a failure, not a skip)
- **VERIFIED BY**: scratch-tree run of the `cmp` loop, recorded in tasks 3.1

#### Scenario: Trust contract: Consumer copies lint
- **WHEN** `buf lint` runs in the proto directory of each consumer that has a `buf.yaml`
- **THEN** it passes (the added `events.proto` in team-order compiles with its `engagement.proto`)
- **VERIFIED BY**: `buf lint` run per consumer, recorded in tasks 3.2
