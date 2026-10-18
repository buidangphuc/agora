## Why
The Q6 initiative moves listing status under server control, lets admins restrict, suspend or ban users, lets buyers report listings and escalate returns to disputes. None of that can be built because the contract has no moderation statuses, no admin RPCs for listings and users, no report messages, no dispute ruling, and no `UserStatusChanged` / `DisputeResolved` events. This slice adds them, additively, plus the proto copies consumers need. Behaviour lives in the later slices (ai-listing-screen, domain-listing-moderation, identity-user-enforcement, engagement-listing-reports, engagement-dispute-escalation, order-dispute-refund and the frontend slices).

initiative: q6-trust-enforcement

## What changes
All edits are additive in `platform-core/packages/proto/platform/`; no field is removed, renumbered or retyped.

**`listing/v1/listing.proto`**
- `ListingStatus` gains `PENDING_REVIEW = 4`, `BANNED = 5`, `HELD = 6`.
- `Listing` gains `ModerationInfo moderation = 12` (server-set, ignored on writes). New `ModerationInfo { reason_code = 1; note = 2; screen_flags = 3 (repeated string); decided_at = 4 (Timestamp) }`.
- `ListingService` gains `ModerateListing` and `ListModerationQueue` with `ModerationDecision` (UNSPECIFIED 0, APPROVE 1, REJECT 2, BAN 3), `ModerateListingRequest { listing_id = 1; decision = 2; reason_code = 3; note = 4 }`, `ModerateListingResponse { listing = 1 }`, `ListModerationQueueRequest { page = 1 }`, `ModerationQueueItem { listing = 1; submitted_at = 2 }`, `ListModerationQueueResponse { items = 1; page = 2 }`.
- Comments on `CreateListing`/`UpdateListing`/`DeleteListing` state that `status` on a write is a request (DRAFT or publish; unset on update keeps the stored status), the stored status is server-derived, a seller write or delete on BANNED/HELD is FAILED_PRECONDITION, a seller edit of REJECTED goes to PENDING_REVIEW, and a content edit of PUBLISHED is re-screened.

**`ai/v1/ai.proto`**
- `AIService.ScreenListing` with `ScreenListingRequest { listing_id = 1; title = 2; description = 3; category_id = 4 }`, `ScreenVerdict` (UNSPECIFIED 0, ALLOW 1, REVIEW 2), `ScreenListingResponse { verdict = 1; labels = 2 (repeated string); backend = 3 }`.

**`identity/v1/identity.proto`**
- New `service AccountAdminService { ListUsers; SetUserStatus }`, `enum UserStatus` (UNSPECIFIED 0, ACTIVE 1, RESTRICTED 2, SUSPENDED 3, BANNED 4), `AdminUser { id = 1; username = 2; roles = 3; status = 4; status_reason = 5; status_until = 6; created_at = 7 }`, `ListUsersRequest { query = 1; status = 2; page = 3 }`, `ListUsersResponse { users = 1; page = 2 }`, `SetUserStatusRequest { user_id = 1; status = 2; reason_code = 3; note = 4; until = 5 }`, `SetUserStatusResponse { user = 1; sessions_revoked = 2 }`.
- Event `UserStatusChanged { user_id = 1; status = 2; previous_status = 3; reason_code = 4; until = 5; changed_at = 6 }` on `identity.events`, next to `SessionRevoked`.
- Comment on `AuthService.Login` and `RefreshSession`: a SUSPENDED or BANNED user gets `PERMISSION_DENIED` (no field change).

**`engagement/v1/engagement.proto`**
- `EngagementService` gains `CreateReport`, `ListReports`, `ResolveReport`, `ListOrderDisputes`, `ListDisputes`.
- Reports: `ReportTargetType` (UNSPECIFIED 0, LISTING 1), `ReportStatus` (UNSPECIFIED 0, OPEN 1, ACTIONED 2, DISMISSED 3), `Report { id = 1; target_type = 2; target_id = 3; reason_code = 4; note = 5; reporter_id = 6; status = 7; created_at = 8; resolved_at = 9; resolution_note = 10 }`, and the request/response messages exactly as the initiative Contracts.
- Disputes: `DisputeRuling` (UNSPECIFIED 0, BUYER_FAVOUR 1, SELLER_FAVOUR 2); `CreateDisputeRequest.return_id = 5`; `Dispute.return_id = 11`, `Dispute.ruling = 12`; `ResolveDisputeRequest.ruling = 4`; `ListOrderDisputesRequest { order_id = 1 }` / `Response { disputes = 1 }`; `ListDisputesRequest { status = 1; page = 2 }` / `Response { disputes = 1; page = 2 }`.
- Comments on the new RPCs fix error codes, authz, ordering, audit-first and the state machine (see the spec). The existing `ResolveDispute` and `CreateDispute` comments gain the tightened state machine and `return_id` semantics.

**`engagement/v1/events.proto`**
- `DisputeResolved { dispute_id = 1; order_id = 2; return_id = 3; status = 4 (DisputeStatus); ruling = 5 (DisputeRuling); resolver_id = 6; resolved_at = 7 (Timestamp) }` on `engagement.events`, key = dispute id. The file gains imports of `google/protobuf/timestamp.proto` (for `resolved_at`; the file has no imports today) and `engagement.proto` (for the two enums).

**`audit/v1/audit.proto`**: the action catalogue comment goes from 13 to 17 actions: `+ listing.moderate, user.set_status, report.resolve, return.dispute_refund`. The existing catalogue test is extended to 17 (wire unchanged).

**Doc**: `platform-core/docs/ROADMAP.md` row "Admin: Approve/lock listings, manage users, handle reports" is annotated with the Q6 slice names (status stays unticked until the initiative is done).

**Copies** (byte-identical to the source, replacing stale subsets; consumers regenerate in their own slices and never edit a copy):
- `listing.proto`: team-domain, team-gateway, team-frontend, team-ai, team-search, team-order.
- `ai.proto`: team-ai, team-domain, team-gateway, team-frontend, team-search, team-analytics.
- `identity.proto`: team-identity, team-gateway, team-frontend, team-domain.
- `engagement.proto`: team-engagement, team-gateway, team-frontend, team-order, team-analytics.
- `events.proto` (engagement): team-engagement, team-analytics, team-order (new copy; needs `engagement.proto` beside it).
- `audit.proto`: the ten repos that carry it (team-analytics, team-audit, team-domain, team-engagement, team-frontend, team-gateway, team-order, team-payment, team-promotion, team-verification).

Committed Go tests pin RPCs, messages, field numbers, enum numbers and comment text, with wire round-trips, in each touched package (new files `listing/v1/moderation_compat_test.go`, `ai/v1/screen_compat_test.go`, `identity/v1/userstatus_compat_test.go`, `engagement/v1/trust_compat_test.go`; `audit/v1/auditquery_roundtrip_test.go` extended).

Out of scope: any behaviour, storage, gateway routes or policy (each owned by a later slice); `OrderReturn.can_escalate`, the return window and Order fields 18-24 (q10-order-lifecycle); the prohibited-term list; chat/shop/review reports.

## NFR
- Additive only: `buf breaking` (FILE) against the base passes; consumers unaware of the additions keep working; unknown enum values (`PENDING_REVIEW`, `BANNED`, `HELD`) must not be removed or renumbered later.
- No new service port, dependency or runtime path; contract-only, no latency impact. The latency budgets of the initiative (screen 2 s timeout, queue page 200 ms, lockout under 1 min) are stated in the comments of the RPCs they bind.
- Compat: old readers skip the new fields; old bytes of every touched message are unchanged.

## Data classification
| field | class | purpose | consent / basis | retention |
|---|---|---|---|---|
| `AdminUser.status`, `status_reason`, `status_until`; `SetUserStatusRequest.reason_code/note/until`; `UserStatusChanged.reason_code/until` | personal (enforcement record about a user) | act on policy violations | platform duty to act against violations (Decree 85/2021); DPO sign-off pending | while the account exists; audit copy per Q3 |
| `AdminUser.username`, `roles`, `id` | personal (existing account data) | admin lookup | contract of service | unchanged (existing) |
| `Report.reporter_id`, `Report.note` (free text, may hold PII) | personal | complaint handling | legitimate interest (platform safety); DPO sign-off pending | closed reports 2 years after `resolved_at`, then reporter pseudonymised and note dropped (sweep in engagement-listing-reports); the reporter is never exposed to the reported seller |
| `Report.resolution_note`, `ModerationInfo.note` | admin text about a listing/report (may name a person) | explain a decision | same as above | with the report / listing; seller sees `ModerationInfo.note` of their own listing only |
| `ModerationInfo.screen_flags` | not personal (matched term / label of listing text) | admin triage | n/a | with the listing; visible to admin only (the owner sees `reason_code` and `note` only); never on `ListingChanged` |
| `Dispute.return_id`, `ruling`; `DisputeResolved` | business identifiers and outcome | route a dispute ruling to team-order | contract of service | as the dispute (existing); the event carries ids, status, ruling and resolver id only, no free text |
| `ScreenListingRequest.title/description` | not personal by design | auto-screen | n/a | not stored by team-ai; the mock backend sends nothing outside the cluster |

## Threat model
- Assets: listing visibility (a seller must not publish or un-reject by themselves); account standing (an admin action must not be spoofable or silently lost); reporter identity; money paid out on a dispute ruling.
- Abuse cases: a seller sending `status=PUBLISHED` or a `moderation` object on a write (documented as ignored; enforced in domain-listing-moderation); a browser or seller calling `ScreenListing` to probe the keyword/AI screen (service-only, `ai.classify`, not routed); a non-admin calling `ModerateListing`, `SetUserStatus`, `ListUsers`, `ResolveReport`, `ListReports`, `ListDisputes` (admin scope; PERMISSION_DENIED); an admin locking themselves or another admin out (FAILED_PRECONDITION); a user reading disputes of an order they are not party to (`ListOrderDisputes` buyer/seller check); a reporter flooding the queue (20 OPEN reports per reporter, RESOURCE_EXHAUSTED); a seller learning who reported them (reporter id appears only in admin-scope RPCs); a forged `DisputeResolved` (event on an internal topic; team-order re-checks the dispute and writes an audit event first); a consumer copy drifting from the source (`cmp` check).
- Authz rules (documented in the proto comments, enforced and tested in the owner slices): scope `admin` for `ModerateListing`, `ListModerationQueue`, `ListUsers`, `SetUserStatus`, `ListReports`, `ResolveReport`, `ListDisputes`; `ResolveDispute` stays admin; service principal with `ai.classify` for `ScreenListing`; signed-in user for `CreateReport` and `CreateDispute`; `ListOrderDisputes` for the order's buyer, the order's seller or an admin. Admin actions write team-audit first and fail closed with `UNAVAILABLE`.
