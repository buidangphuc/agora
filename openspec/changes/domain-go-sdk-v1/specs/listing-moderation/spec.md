## MODIFIED Requirements

### Requirement: Admin moderates a listing

`ListingService.ModerateListing` SHALL require the `admin` scope, validate the request (`INVALID_ARGUMENT`), load the listing (`NOT_FOUND`), check the transition, write one team-audit event `listing.moderate` before changing anything, then update the status and `moderation` fields and enqueue `ListingChanged(UPDATED)` in one transaction that applies only if the listing's status and `updated_at` are unchanged since the load; otherwise it SHALL return `FAILED_PRECONDITION` and change nothing. The `ListingChanged` payload SHALL carry `moderation.reason_code` and `decided_at` but never the note or the screen flags. `APPROVE` SHALL be allowed only from `PENDING_REVIEW` (to `PUBLISHED`). `REJECT` and `BAN` SHALL be allowed from `PENDING_REVIEW` or `PUBLISHED` and SHALL require a `reason_code` from `prohibited_item | counterfeit | misleading | inappropriate | duplicate | seller_enforcement | other`. Every other source status (`DRAFT`, `REJECTED`, `BANNED`, `HELD`) SHALL return `FAILED_PRECONDITION`. A `note` longer than 500 characters SHALL return `INVALID_ARGUMENT`. The audit event SHALL carry `actor_id` = the admin's verified principal id, `target_type` `listing`, `target_id` the listing id and metadata `seller_id`, `decision`, `reason_code`, `note`, `from_status`, `to_status`. An audit failure or timeout SHALL return `UNAVAILABLE` and change nothing. `UPSTREAM_AUDIT_ADDR` is required in every `ENV`, so there is no unset-upstream mode.

#### Scenario: Moderation: Approve publishes a pending listing
- **WHEN** an admin calls ModerateListing with `APPROVE` on a `pending_review` listing
- **THEN** the response listing has status `published`, `moderation.decided_at` is set, and one `ListingChanged(UPDATED)` outbox row carries status `published`
- **VERIFIED BY**: team-domain `internal/listing` `TestModerateApprovePublishes`

#### Scenario: Moderation: Reject records reason and note for the seller
- **WHEN** an admin calls ModerateListing with `REJECT`, reason code `counterfeit` and note `Fake logo` on a `pending_review` listing
- **THEN** the stored status is `rejected`, `moderation.reason_code` is `counterfeit`, `moderation.note` is `Fake logo`, and one `ListingChanged(UPDATED)` outbox row carries status `rejected`
- **VERIFIED BY**: team-domain `internal/listing` `TestModerateRejectRecordsReason`

#### Scenario: Moderation: Ban takes down a published listing
- **WHEN** an admin calls ModerateListing with `BAN` and reason code `prohibited_item` on a `published` listing
- **THEN** the stored status is `banned` and one `ListingChanged(UPDATED)` outbox row carries status `banned`
- **VERIFIED BY**: team-domain `internal/listing` `TestModerateBanPublished`

#### Scenario: Moderation: Reject of a published listing from a report
- **WHEN** an admin calls ModerateListing with `REJECT` and reason code `misleading` on a `published` listing
- **THEN** the stored status is `rejected` and an anonymous GetListing returns `NotFound`
- **VERIFIED BY**: team-domain `internal/listing` `TestModerateRejectPublished`

#### Scenario: Moderation: Audit event is written first
- **WHEN** an admin calls ModerateListing and the audit write is recorded by a fake writer
- **THEN** the fake receives one `listing.moderate` call before the transaction starts, with the stored owner id as `seller_id` and `from_status` `pending_review`, `to_status` `rejected`
- **VERIFIED BY**: team-domain `internal/listing` `TestModerateAuditPrecedesChange`

#### Scenario: Moderation: Audit down refuses the decision
- **WHEN** team-audit returns an error, and in a second case a deadline-exceeded, while an admin calls ModerateListing
- **THEN** each case returns `Unavailable`, the stored status is unchanged and no outbox row is written
- **VERIFIED BY**: team-domain `internal/rpc` `TestModerateListingAuditDownUnavailableNoChange`

#### Scenario: Moderation: No audit upstream configured
- **WHEN** settings load with `ENV=local` and `UPSTREAM_AUDIT_ADDR` unset (everything else valid)
- **THEN** `Load` returns an error naming `UPSTREAM_AUDIT_ADDR`, so the service never starts without an audit upstream and no moderation can run unaudited
- **VERIFIED BY**: team-domain `internal/config` `TestUpstreamAddressesRequiredInEveryEnv`

#### Scenario: Moderation: Approve of a held listing is refused
- **WHEN** an admin calls ModerateListing with `APPROVE` on a `held` listing
- **THEN** the RPC returns `FailedPrecondition`, the stored status stays `held` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/listing` `TestModerateHeldFailedPreconditionNoAudit`

#### Scenario: Moderation: Approve of a rejected listing is refused
- **WHEN** an admin calls ModerateListing with `APPROVE` on a `rejected` listing
- **THEN** the RPC returns `FailedPrecondition` and the stored status stays `rejected`
- **VERIFIED BY**: team-domain `internal/listing` `TestModerateApproveRejectedFailedPrecondition`

#### Scenario: Moderation: Approve of a banned listing is refused
- **WHEN** an admin calls ModerateListing with `APPROVE` on a `banned` listing
- **THEN** the RPC returns `FailedPrecondition` and the stored status stays `banned`
- **VERIFIED BY**: team-domain `internal/listing` `TestModerateApproveBannedFailedPrecondition`

#### Scenario: Moderation: Concurrent decision applies once
- **WHEN** two admins call ModerateListing on the same `pending_review` listing, one `APPROVE` and one `REJECT`, and both pass the transition check before either transaction runs
- **THEN** exactly one call succeeds, the other returns `FailedPrecondition`, the stored status is the winner's `to_status` and exactly 1 `ListingChanged(UPDATED)` outbox row is written
- **VERIFIED BY**: team-domain `internal/store` `TestModerateTxConditionalOnLoadedStatus`

#### Scenario: Moderation: Seller edit between check and decision is refused
- **WHEN** the seller's UpdateListing commits after an admin's ModerateListing loaded the listing and before its `ModerateTx` runs
- **THEN** the admin call returns `FailedPrecondition`, the stored status is `pending_review` and no moderation outbox row is written
- **VERIFIED BY**: team-domain `internal/listing` `TestModerateStaleLoadFailedPrecondition`

#### Scenario: Moderation: ListingChanged carries no note or flags
- **WHEN** an admin rejects a `pending_review` listing with flag `keyword:replica` using reason code `counterfeit` and note `Fake logo`
- **THEN** the `ListingChanged(UPDATED)` outbox payload has `moderation.reason_code` `counterfeit`, an empty `moderation.note` and an empty `moderation.screen_flags`
- **VERIFIED BY**: team-domain `internal/rpc` `TestModerateListingEventStripsNoteAndFlags`

#### Scenario: Moderation: Second decision on a decided listing is refused
- **WHEN** an admin calls ModerateListing with `REJECT` twice on the same `pending_review` listing
- **THEN** the second call returns `FailedPrecondition` and the fake audit writer holds exactly 1 event
- **VERIFIED BY**: team-domain `internal/listing` `TestModerateTwiceSecondRefused`

#### Scenario: Moderation: Reject without a reason code is invalid
- **WHEN** an admin calls ModerateListing with `REJECT` and an empty `reason_code`
- **THEN** the RPC returns `InvalidArgument` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/rpc` `TestModerateListingRejectNeedsReason`

#### Scenario: Moderation: Unknown reason code is invalid
- **WHEN** an admin calls ModerateListing with `BAN` and reason code `because`
- **THEN** the RPC returns `InvalidArgument` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/rpc` `TestModerateListingUnknownReasonCode`

#### Scenario: Moderation: Note limit is 500 characters
- **WHEN** an admin calls ModerateListing with a 500-character note, and in a second call a 501-character note
- **THEN** the first call succeeds and the second returns `InvalidArgument` with 0 audit events written for it
- **VERIFIED BY**: team-domain `internal/rpc` `TestModerateListingNoteBoundary500`

#### Scenario: Moderation: Unspecified decision is invalid
- **WHEN** an admin calls ModerateListing with `MODERATION_DECISION_UNSPECIFIED`
- **THEN** the RPC returns `InvalidArgument`
- **VERIFIED BY**: team-domain `internal/rpc` `TestModerateListingUnspecifiedDecision`

#### Scenario: Moderation: Unknown listing writes no audit event
- **WHEN** an admin calls ModerateListing with an id that does not exist
- **THEN** the RPC returns `NotFound` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/listing` `TestModerateNotFoundNoAudit`

#### Scenario: Moderation: Invalid request on an unknown listing is invalid
- **WHEN** an admin calls ModerateListing with `BAN`, reason code `because` and an id that does not exist
- **THEN** the RPC returns `InvalidArgument` (validation runs before the load) and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/rpc` `TestModerateListingValidatesBeforeLoad`

#### Scenario: Moderation: Seller cannot moderate
- **WHEN** a seller holding `listing.write` without the `admin` scope calls ModerateListing on their own listing
- **THEN** the RPC returns `PermissionDenied`, the stored status is unchanged and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-domain `internal/rpc` `TestModerateListingNonAdminDenied`

#### Scenario: Moderation: Admin who owns the listing is audited
- **WHEN** an admin calls ModerateListing on a listing they own
- **THEN** the call is applied and the fake audit writer holds exactly 1 `listing.moderate` event with that admin as `actor_id`
- **VERIFIED BY**: team-domain `internal/listing` `TestModerateAdminOwnListingAudited`

#### Scenario: Moderation: Decision flows end to end with audit
- **WHEN** an admin rejects a seller's `pending_review` listing with reason `counterfeit` and note `Fake logo` through the gateway
- **THEN** team-audit holds exactly 1 event with `action` `listing.moderate`, the admin id as `actor_id`, the listing id as `target_id` and metadata `decision` `reject`, and the seller's ListMyListings shows status `rejected` with reason `counterfeit`
- **VERIFIED BY**: platform-e2e `tests/e2e/features/seller/listing_moderation.feature` "Admin rejects a flagged listing"

#### Scenario: Moderation: Moderation is refused while audit is down end to end
- **WHEN** team-audit is stopped and an admin calls ModerateListing through the gateway
- **THEN** the call returns `unavailable`, and after team-audit restarts the listing is still `pending_review` and team-audit holds 0 `listing.moderate` events for it
- **VERIFIED BY**: platform-e2e `tests/e2e/features/seller/listing_moderation.feature` "Moderation is refused while audit is down" (tagged `@destructive`)

#### Scenario: Moderation: Rejected listing leaves search end to end
- **WHEN** an admin approves a flagged listing and later bans it
- **THEN** a team-search query returns the listing after the approval and returns no hit for it within 60 s of the ban
- **VERIFIED BY**: platform-e2e `tests/e2e/features/seller/listing_moderation.feature` "Banned listing leaves search"
