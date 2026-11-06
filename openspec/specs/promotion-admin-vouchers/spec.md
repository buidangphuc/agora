# promotion-admin-vouchers Specification

## Purpose
TBD - created by archiving change promotion-admin-vouchers. Update Purpose after archive.

## Requirements

### Requirement: Admin can stop a platform voucher
The system SHALL let an admin deactivate a voucher so that no new reservation is placed on it, while existing holds still commit and release. Validation (admin scope, existing voucher) SHALL run before the audit append, so a refused call writes no audit entry.

#### Scenario: V1 stop blocks new checkouts
- **WHEN** an admin calls `DeactivateVoucher` for voucher SALE10
- **THEN** the system SHALL return the voucher with `active=false` and a non-empty `deactivated_at`

#### Scenario: V1a next reservation is refused
- **WHEN** `ValidateAndReserve` is called for SALE10 after the stop
- **THEN** the system SHALL return `valid=false, reason=inactive`

#### Scenario: V1b no hold is created
- **WHEN** `ValidateAndReserve` is refused with `inactive`
- **THEN** the system SHALL create no reservation row for that reservation id

#### Scenario: V1c inactive wins over expired
- **WHEN** `ValidateAndReserve` is called for an inactive voucher whose `ends_at` is also in the past
- **THEN** the system SHALL return `reason=inactive`, not the expired reason

#### Scenario: V2 existing hold still commits
- **WHEN** an order already holding SALE10 reserved before the stop calls `CommitReservation`
- **THEN** the system SHALL return `committed=true` and increase `used` by 1

#### Scenario: V3 existing hold still releases
- **WHEN** an order already holding SALE10 reserved before the stop calls `ReleaseReservation`
- **THEN** the system SHALL return `released=true` with `used` unchanged

#### Scenario: V4 stopping twice is a no-op
- **WHEN** an admin calls `DeactivateVoucher` on an already inactive voucher
- **THEN** the system SHALL return the voucher with the original `deactivated_at`

#### Scenario: V4a stopping twice is audited per call
- **WHEN** an admin calls `DeactivateVoucher` on an already inactive voucher
- **THEN** the system SHALL write exactly one `voucher.deactivate` audit entry for that call

#### Scenario: V5 unknown voucher is NOT_FOUND
- **WHEN** an admin calls `DeactivateVoucher` with an id that does not exist
- **THEN** the system SHALL return `NOT_FOUND`

#### Scenario: V5a unknown voucher writes no audit
- **WHEN** an admin calls `DeactivateVoucher` with an id that does not exist
- **THEN** the system SHALL write no audit entry because validation precedes the audit append

#### Scenario: V6 non-admin cannot stop
- **WHEN** a seller or buyer token calls `DeactivateVoucher`
- **THEN** the system SHALL return `PERMISSION_DENIED`

#### Scenario: V6a non-admin stop leaves no trace
- **WHEN** a seller or buyer token calls `DeactivateVoucher`
- **THEN** the system SHALL write no audit entry and leave the voucher row unchanged

#### Scenario: V7 anonymous cannot stop or update
- **WHEN** a request without a token calls `DeactivateVoucher` or `UpdateVoucher` through the gateway
- **THEN** the gateway SHALL return `UNAUTHENTICATED`

#### Scenario: V7a anonymous request does not reach promotion
- **WHEN** a request without a token calls `DeactivateVoucher` or `UpdateVoucher` through the gateway
- **THEN** team-promotion SHALL receive no call
- **VERIFIED BY**: team-gateway `internal/edge` `TestV7_AnonymousCannotStopOrUpdate_NoUpstreamCall`

#### Scenario: V7b buyer token cannot stop or update through the gateway
- **WHEN** a buyer token calls `DeactivateVoucher` or `UpdateVoucher` through the gateway
- **THEN** the gateway SHALL return `PERMISSION_DENIED`

### Requirement: Admin can move the end date or raise the quota
The system SHALL let an admin change only `ends_at` and raise `quota` of an active voucher. A new quota SHALL be non-zero, at least `used` and at least the current quota; a voucher whose current quota is 0 (unlimited) SHALL NOT receive a finite quota. All validation SHALL run before the audit append, so refused calls write no audit entry. The quota check and the update SHALL be one conditional UPDATE, atomic against a concurrent `ValidateAndReserve`.

#### Scenario: V8 raise quota
- **WHEN** an admin calls `UpdateVoucher` with `quota=150` on SALE10 whose quota is 100 and used is 40
- **THEN** the system SHALL return the voucher with `quota=150`

#### Scenario: V8a raise quota leaves other fields
- **WHEN** an admin calls `UpdateVoucher` with `quota=150`
- **THEN** discount, min spend, max discount, scope and `ends_at` SHALL be unchanged

#### Scenario: V8b raise quota is audited once
- **WHEN** an admin calls `UpdateVoucher` with `quota=150`
- **THEN** the system SHALL write exactly one `voucher.update` audit entry

#### Scenario: V9 move end date
- **WHEN** an admin calls `UpdateVoucher` with an `ends_at` later than `starts_at` and later than now
- **THEN** the system SHALL return the voucher with the new `ends_at`

#### Scenario: V9a new end date reopens the window
- **WHEN** `ValidateAndReserve` is called after the old end date and before the new one
- **THEN** the system SHALL return `valid=true`

#### Scenario: V9b ends_at at or before now is refused
- **WHEN** an admin calls `UpdateVoucher` with `ends_at` before the current time or exactly equal to it
- **THEN** the system SHALL return `INVALID_ARGUMENT`

#### Scenario: V9c ends_at one second after now is accepted
- **WHEN** an admin calls `UpdateVoucher` with `ends_at` = now + 1 second
- **THEN** the system SHALL return the voucher with that `ends_at`

#### Scenario: V10 quota below used is refused
- **WHEN** an admin calls `UpdateVoucher` with `quota=30` on a voucher with `used=40`
- **THEN** the system SHALL return `INVALID_ARGUMENT` and leave the voucher unchanged

#### Scenario: V10a refused quota writes no audit
- **WHEN** an admin calls `UpdateVoucher` with `quota=30` on a voucher with `used=40`
- **THEN** the system SHALL write no audit entry

#### Scenario: V11 quota lowered is refused
- **WHEN** an admin calls `UpdateVoucher` with a quota above `used` but lower than the current quota
- **THEN** the system SHALL return `INVALID_ARGUMENT`, leave the voucher unchanged and write no audit entry

#### Scenario: V12 fully used voucher with the same quota and a new end date is accepted
- **WHEN** an admin calls `UpdateVoucher` with `quota=100` and a valid `ends_at` on a fully used voucher with quota 100 and used 100
- **THEN** the system SHALL accept the call because the new quota is at least `used` and at least the current quota

#### Scenario: V12a same quota and no end date is nothing to change
- **WHEN** an admin calls `UpdateVoucher` with `quota` equal to the current quota and no `ends_at`
- **THEN** the system SHALL return `INVALID_ARGUMENT` with message "nothing to change" and write no audit entry

#### Scenario: V12b quota equal to current with ends_at
- **WHEN** an admin calls `UpdateVoucher` with `quota` equal to the current quota and a valid `ends_at`
- **THEN** the system SHALL return the voucher with the new `ends_at` and `quota` unchanged

#### Scenario: V12c quota equal to current with ends_at is audited once
- **WHEN** an admin calls `UpdateVoucher` with `quota` equal to the current quota and a valid `ends_at`
- **THEN** the system SHALL write exactly one `voucher.update` audit entry

#### Scenario: V12d quota greater than used and greater than current
- **WHEN** an admin calls `UpdateVoucher` with `quota=101` on a voucher with quota 100 and used 100
- **THEN** the system SHALL return the voucher with `quota=101`

#### Scenario: V13 end date not after start is refused
- **WHEN** an admin calls `UpdateVoucher` with `ends_at` equal to or before `starts_at`
- **THEN** the system SHALL return `INVALID_ARGUMENT` with no audit entry and the voucher unchanged

#### Scenario: V14 nothing to change
- **WHEN** an admin calls `UpdateVoucher` with neither `ends_at` nor `quota`
- **THEN** the system SHALL return `INVALID_ARGUMENT` and write no audit entry

#### Scenario: V15 inactive voucher cannot be edited
- **WHEN** an admin calls `UpdateVoucher` on an inactive voucher
- **THEN** the system SHALL return `FAILED_PRECONDITION` with no audit entry and the voucher unchanged

#### Scenario: V15a unknown voucher on update
- **WHEN** an admin calls `UpdateVoucher` with an id that does not exist
- **THEN** the system SHALL return `NOT_FOUND` and write no audit entry

#### Scenario: V16 money fields are never altered by an update
- **WHEN** any successful `UpdateVoucher` is applied
- **THEN** the stored discount type, discount value, min spend, max discount and scope SHALL be identical before and after

#### Scenario: V16a quota zero is refused
- **WHEN** an admin calls `UpdateVoucher` with `quota=0`
- **THEN** the system SHALL return `INVALID_ARGUMENT` and write no audit entry

#### Scenario: V16b unlimited voucher cannot become finite
- **WHEN** an admin calls `UpdateVoucher` with `quota=500` on a voucher whose current quota is 0
- **THEN** the system SHALL return `INVALID_ARGUMENT` and leave the quota at 0

#### Scenario: V16c unlimited voucher end date can move
- **WHEN** an admin calls `UpdateVoucher` with only a valid `ends_at` on a voucher whose quota is 0
- **THEN** the system SHALL return the voucher with the new `ends_at` and quota 0

#### Scenario: V16d race with a concurrent reservation
- **WHEN** `UpdateVoucher` with `quota=Q` runs concurrently with commits that make `used` exceed `Q` before the conditional UPDATE
- **THEN** the system SHALL return `INVALID_ARGUMENT` and the stored `quota` SHALL never be below `used`
- **VERIFIED BY**: team-promotion `internal/store` `TestPostgresVoucherUpdateAgainstConcurrentReservationsNeverBreaksQuotaInvariant_V16d`
- **VERIFIED BY**: team-promotion `internal/store` `TestPostgresVoucherConcurrentUpdatesNeverLowerQuotaBelowUsed_V16d`

#### Scenario: V16e lost race keeps the audit entry
- **WHEN** an `UpdateVoucher` loses the race of V16d after its audit append
- **THEN** the `voucher.update` audit entry SHALL remain and record the attempt (assumption, flagged for the human in LOCK.md)
- **VERIFIED BY**: team-promotion `internal/voucher` `TestUpdateVoucher_V16d_V16e_lostRaceIsInvalidKeepsAuditAndNeverBelowUsed`

#### Scenario: V16f concurrent deactivate during update
- **WHEN** a `DeactivateVoucher` commits between the validation and the conditional UPDATE of an `UpdateVoucher`
- **THEN** the conditional UPDATE, which also requires `active=true`, SHALL match no row and the call SHALL return `FAILED_PRECONDITION`
- **VERIFIED BY**: team-promotion `internal/store` `TestPostgresVoucherConcurrentDeactivateAndUpdateStayConsistent_V16f`
- **VERIFIED BY**: team-promotion `internal/voucher` `TestUpdateVoucher_V16f_deactivateBetweenValidationAndUpdateIsFailedPrecondition`

#### Scenario: V16g error precedence
- **WHEN** an admin write fails several checks at once
- **THEN** the system SHALL return the first applicable code in the order `UNAUTHENTICATED`, `PERMISSION_DENIED`, `NOT_FOUND`, `FAILED_PRECONDITION`, `INVALID_ARGUMENT`
- **VERIFIED BY**: team-promotion `internal/voucher` `TestUpdateVoucher_V16g_errorPrecedence`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestUpdateVoucherFailedPreconditionBeatsInvalidArgument_V16g`

#### Scenario: V17 non-admin cannot edit
- **WHEN** a seller token calls `UpdateVoucher`
- **THEN** the system SHALL return `PERMISSION_DENIED` and write no audit entry

### Requirement: Duplicate voucher code is reported as ALREADY_EXISTS
The system SHALL return `ALREADY_EXISTS` when `CreateVoucher` is called with a code already in use. The duplicate-code pre-check SHALL run before the audit append, so a refused create writes no audit entry.

#### Scenario: V18 taken code is refused with an error
- **WHEN** an admin calls `CreateVoucher` with code SALE10 that already exists
- **THEN** the system SHALL return `ALREADY_EXISTS` with a message naming the code field, never `INTERNAL`

#### Scenario: V18a duplicate writes no audit
- **WHEN** an admin calls `CreateVoucher` with code SALE10 that already exists
- **THEN** the system SHALL write no audit entry and leave the existing voucher unchanged

#### Scenario: V18b duplicate race has one winner
- **WHEN** two concurrent `CreateVoucher` calls use the same new code
- **THEN** exactly one SHALL succeed

#### Scenario: V18c duplicate race loser gets ALREADY_EXISTS
- **WHEN** two concurrent `CreateVoucher` calls use the same new code and one fails on the unique constraint
- **THEN** the failing call SHALL return `ALREADY_EXISTS`, never `INTERNAL`

#### Scenario: V19 gateway keeps the code
- **WHEN** the same duplicate call goes through the gateway
- **THEN** the gateway SHALL return Connect code `already_exists`, not `internal`

#### Scenario: V20 admin UI shows the duplicate message
- **WHEN** an admin submits the create form with a taken code in the admin app
- **THEN** the form SHALL show "code already exists" on the code field

#### Scenario: V20a no voucher added on duplicate
- **WHEN** an admin submits the create form with a taken code in the admin app
- **THEN** the voucher list SHALL contain no additional row

### Requirement: Audit-down and promotion-down are distinguishable
The system SHALL fail closed on every admin voucher write and SHALL tell the browser which dependency is down. Invalid input SHALL be refused with its own code even while audit is down.

#### Scenario: V21 audit down on any admin voucher write
- **WHEN** team-audit is unavailable and an admin calls `CreateVoucher`, `DeactivateVoucher` or `UpdateVoucher` with valid input
- **THEN** team-promotion SHALL return `UNAVAILABLE` with `ErrorInfo{domain:"promotion.platform", reason:"AUDIT_UNAVAILABLE"}`

#### Scenario: V21a audit down changes nothing
- **WHEN** team-audit is unavailable and an admin calls any admin voucher write with valid input
- **THEN** the system SHALL create or change no row

#### Scenario: V21b audit down with invalid input
- **WHEN** team-audit is unavailable and an admin calls `UpdateVoucher` with `quota=0`, or `CreateVoucher` with a taken code, or `DeactivateVoucher` with an unknown id
- **THEN** the system SHALL return `INVALID_ARGUMENT`, `ALREADY_EXISTS` or `NOT_FOUND` respectively and write nothing

#### Scenario: V22 audit written before the change
- **WHEN** an admin write passes validation
- **THEN** the audit entry (`voucher.platform_create`, `voucher.deactivate` or `voucher.update`) SHALL be appended before the voucher row is written, with the admin id as actor and the voucher code as target
- **VERIFIED BY**: team-promotion `internal/voucher` `TestDeactivateVoucher_V22_auditBeforeWriteWithActorAndCode`
- **VERIFIED BY**: team-promotion `internal/voucher` `TestUpdateVoucher_V22_auditBeforeWriteWithActorAndCode`

#### Scenario: V23 gateway keeps the audit reason
- **WHEN** the gateway receives an upstream `UNAVAILABLE` carrying `ErrorInfo` reason `AUDIT_UNAVAILABLE`
- **THEN** it SHALL return Connect `unavailable` with `ErrorInfo` reason `AUDIT_UNAVAILABLE` and the message of the `msgServiceUnavailable` constant in `team-gateway/internal/edge/forward.go` ("service unavailable")

#### Scenario: V23a gateway never forwards the raw backend message
- **WHEN** the gateway sanitizes an upstream `UNAVAILABLE` carrying `ErrorInfo`
- **THEN** the response message SHALL NOT contain the upstream message text

#### Scenario: V24 gateway reports promotion down
- **WHEN** team-promotion cannot be reached from the gateway on an admin voucher write
- **THEN** the gateway SHALL return Connect `unavailable` with `ErrorInfo{domain:"promotion.platform", reason:"PROMOTION_UNAVAILABLE"}`

#### Scenario: V25 upstream unavailable without ErrorInfo stays generic
- **WHEN** an upstream `UNAVAILABLE` carries no `ErrorInfo` and the upstream is not promotion
- **THEN** the gateway SHALL return `unavailable` with no `ErrorInfo` reason

#### Scenario: V26 admin UI audit-down message
- **WHEN** an admin submits create while team-audit is down
- **THEN** the admin app SHALL show "audit is down, nothing created"

#### Scenario: V26a admin UI audit-down message on stop and edit
- **WHEN** an admin submits stop or edit while team-audit is down
- **THEN** the admin app SHALL show "audit is down, nothing changed"

#### Scenario: V27 admin UI promotion-down message
- **WHEN** an admin submits create while team-promotion is down
- **THEN** the admin app SHALL show "promotion is down, nothing created"

#### Scenario: V27b admin UI promotion-down on stop and edit
- **WHEN** an admin submits stop or edit while team-promotion is down
- **THEN** the admin app SHALL show "promotion is down, nothing changed"

#### Scenario: V27c the two messages differ
- **WHEN** the admin app shows either outage message
- **THEN** it SHALL NOT show the other message

#### Scenario: V27a non-admin write is denied before audit
- **WHEN** a token without scope admin calls any admin voucher write
- **THEN** the system SHALL return `PERMISSION_DENIED` without writing an audit entry

### Requirement: Admin app screens for stop and edit
The admin app SHALL let an admin stop a voucher and edit its end date and quota from `/admin/vouchers`, and the buyer app SHALL tell the buyer a stopped voucher no longer applies. Only sessions holding both `admin` and `promotion.admin` SHALL reach the voucher screens and run their server actions.

#### Scenario: V28 stop from the list
- **WHEN** an admin confirms "Stop" on an active voucher in the admin app
- **THEN** the voucher row SHALL show status "stopped" after refresh

#### Scenario: V28a buyer sees a stopped voucher as not applicable
- **WHEN** a buyer applies the stopped code in checkout in the buyer app
- **THEN** both VoucherModal components (`team-frontend/src/features/order` and `src/features/voucher`) SHALL show "Mã giảm giá đã ngừng áp dụng." and no raw reason string

#### Scenario: V28b other reasons keep their text
- **WHEN** the buyer applies a code refused with a reason other than `inactive`
- **THEN** the modal SHALL show the same text as before this change

#### Scenario: V29 edit end date and quota
- **WHEN** an admin saves a new end date and a higher quota on the voucher detail screen
- **THEN** the detail screen SHALL show the new end date and quota

#### Scenario: V29a money fields read-only
- **WHEN** an admin opens the edit screen
- **THEN** discount, min spend, max discount and scope SHALL be read-only

#### Scenario: V30 edit validation shown on the field
- **WHEN** an admin enters a quota below the used count or below the current quota, or an end date not after the start date
- **THEN** the form SHALL show an error on that field and send no request

#### Scenario: V30a quota field hidden for unlimited vouchers
- **WHEN** an admin opens the edit form of a voucher whose quota is 0 (unlimited)
- **THEN** the form SHALL not render the quota field

#### Scenario: V31 stopped voucher has no edit
- **WHEN** an admin opens a stopped voucher
- **THEN** the Stop and Edit controls SHALL be absent

#### Scenario: V32 non-admin sees no controls
- **WHEN** a signed-in user without scope admin opens `/admin/vouchers`
- **THEN** the app SHALL show the forbidden screen with no Stop or Edit control

#### Scenario: V32a admin token without promotion.admin is refused
- **WHEN** a session holding `admin` but not `promotion.admin` opens `/admin/vouchers`, `/admin/vouchers/new` or a voucher detail, or runs the create, stop or edit action
- **THEN** the app SHALL show the forbidden screen or return the forbidden result, and SHALL send no request to the gateway
- **VERIFIED BY**: team-frontend-admin `src/app/admin/vouchers/**/page.test.tsx` and `src/features/vouchers/actions.test.ts` "(admin token without promotion.admin)"

#### Scenario: V33 create dialog no longer claims immutability
- **WHEN** an admin opens the create confirmation dialog
- **THEN** the dialog text SHALL NOT state that a voucher cannot be changed after creation

### Requirement: Gap rows closed
The documentation SHALL no longer list gaps G9, G10 and G11.

#### Scenario: V34 gaps removed
- **WHEN** a reader opens `fe-platform/docs/gaps.md` after this change
- **THEN** the file SHALL contain no row whose id is G9, G10 or G11

#### Scenario: V35 other gaps untouched
- **WHEN** a reader compares `fe-platform/docs/gaps.md` on the branch with the base this change is applied to
- **THEN** rows G9, G10 and G11 are removed, no row is added, and every remaining row is byte-identical to the base, without any other change
