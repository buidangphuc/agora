# 00-spec — Trust enforcement: listing moderation, user enforcement, listing reports, return disputes

initiative: q6-trust-enforcement

Source: backlog Q6, Q8, Q23 (`plans/pm-review-2026-10/BACKLOG.md`), evidence `plans/pm-review-2026-10/trust-ops.md` #1, #2, #3, #7; the return-dispute half of Q10 (moved here from q10-order-lifecycle, decision X2).
Base: `origin/main` (Q1 session refresh, Q2 review integrity, Q3 audit log, Q4 loop events, Q5 advertised price merged). The two dispute slices also need q10-order-lifecycle merged (see Architecture decisions, cross-initiative dependency).

## Goal
A seller can no longer publish a listing by sending `status=PUBLISHED`, or undo an admin's rejection: listing status is server-owned, every content write goes through an auto-screen (keyword list plus a team-ai classifier, mock by default), and anything flagged waits in an admin moderation queue at `/admin/listings`. An admin can restrict, suspend or ban a user from `/admin/users`: the user's sessions end within a minute, a suspended or banned user cannot log in, and their listings disappear from the store while the suspension or ban lasts. A buyer can report a listing, and a buyer can escalate a rejected or ignored return to an Agora dispute. Admins work listing reports, the moderation queue and disputes from one `/admin/cases` inbox; a ruling in the buyer's favour refunds the return through the existing refund saga. Every admin action here writes a team-audit event first (Q3 rule).

## User stories
1. **Status is server-owned.**
   - Given a seller creating or editing a listing.
   - When the request carries `status=PUBLISHED` (a publish request) and the content passes the auto-screen.
   - Then the stored status is `PUBLISHED`. If the screen flags it, the stored status is `PENDING_REVIEW` and the listing stays hidden from non-owners (NOT_FOUND, as for drafts today).
   - Edge cases:
     - `status=DRAFT` stores a draft; `REJECTED`, `BANNED`, `PENDING_REVIEW` or `HELD` sent by a seller are ignored (the stored status is kept or derived as above).
     - A seller edit of a `REJECTED` listing moves it to `PENDING_REVIEW` (manual review, no auto-publish); the seller cannot move it anywhere else.
     - A `BANNED` or `HELD` listing refuses seller writes with `FAILED_PRECONDITION`.
     - Editing only price, stock or variants of a `PUBLISHED` listing does not re-screen; editing title, description, category or images does.
2. **Admin moderates the queue.**
   - Given an admin on `/admin/listings`.
   - When they open the queue, they see `PENDING_REVIEW` listings oldest first, with the screen flags.
   - When they approve, reject (reason code + optional note) or ban a listing, its status changes, a `listing.moderate` audit event is written first, and a `ListingChanged(UPDATED)` event de-indexes or re-indexes it in team-search and team-ai.
   - Edge cases: team-audit down → `UNAVAILABLE`, no change. A non-admin gets 403 at the edge. The seller sees the reason code and note in the seller center.
3. **Admin enforces a user.**
   - Given an admin on `/admin/users` who finds a user by username or id.
   - When they set the status to `restricted`, `suspended` (with an end time) or `banned` with a reason code.
   - Then a `user.set_status` audit event is written first, every session of that user is revoked (refresh tokens and the gateway denylist via `SessionRevoked`), open SSE streams end, and `UserStatusChanged` is emitted.
   - Edge cases:
     - Login (and RefreshSession) of a suspended or banned user fails with `PERMISSION_DENIED` after a correct password; a suspension past its end time is lifted at the next login (status `active`, `UserStatusChanged(ACTIVE)` emitted).
     - A restricted user can log in, buy and chat, but their token lacks `listing.write` (restriction covers listing writes only).
     - An admin cannot change their own status or another admin's (`FAILED_PRECONDITION`).
4. **A suspended or banned seller's listings leave the store.**
   - Given team-domain receives `UserStatusChanged(SUSPENDED)` or `UserStatusChanged(BANNED)`.
   - Then each `PUBLISHED` or `PENDING_REVIEW` listing of that seller becomes `HELD` (storing its prior status) and is de-indexed.
   - Edge cases:
     - When the user returns to `active` or `restricted` (admin reinstatement, or a suspension lifted at login), each held listing goes back to its stored prior status automatically (no moderation queue); a prior `PUBLISHED` listing is re-indexed.
     - `restricted` holds nothing. Redelivered events change nothing twice; `SUSPENDED → BANNED` keeps the listings held with their original prior status.
5. **A buyer reports a listing; the admin works the case.**
   - Given a signed-in user on a PDP.
   - When they report it with a reason code and an optional note (≤500 chars).
   - Then a report is stored `OPEN`; a second open report by the same user on the same listing returns the existing one.
   - When an admin on `/admin/cases` (tab "Báo cáo") opens a report, they can take action on the listing (ModerateListing) and close the report as `ACTIONED` or `DISMISSED` with a note; closing writes a `report.resolve` audit event first.
   - Edge cases: anonymous gets 401; a seller reporting their own listing is refused; the reporter's identity is never shown to the seller.
6. **A buyer escalates a return to Agora.**
   - Given a return that q10 marks `OrderReturn.can_escalate` (q10 rule: the return is REJECTED and the order is SHIPPED or still inside its return window; a PENDING return is auto-approved by q10 after 3 days, so it never escalates).
   - When the buyer taps "Khiếu nại lên Agora" on the order detail, Then team-engagement creates a dispute linked to `order_id` and `return_id` (claimant buyer, defendant seller) after reading the return from team-order (`GetReturnRequest` as a service principal with `order.read`); one open dispute per return. The order detail lists the order's disputes (`ListOrderDisputes`, the order's buyer or seller, or admin — A18).
   - Edge cases: `can_escalate` false → `FAILED_PRECONDITION`; a second dispute on the same return while one is open returns `ALREADY_EXISTS`; a non-party gets `PERMISSION_DENIED` on `ListOrderDisputes`.
7. **Admin rules on a dispute; a buyer-favour ruling refunds the return.**
   - Given an admin on `/admin/cases` tab "Khiếu nại", which lists disputes (`ListDisputes`, filter by status, oldest first).
   - When they move a dispute to INVESTIGATING and then resolve it with ruling `DISPUTE_RULING_BUYER_FAVOUR` (or reject it), Then the existing dispute audit is written first, one `DisputeResolved` event is emitted; team-order writes the audit event `return.dispute_refund` (actor = the resolving admin), moves the return to APPROVED and, for an order paid online, to REFUNDED, emitting the existing `ReturnRefunded` that team-payment refunds once.
   - Edge cases:
     - Dispute state machine: OPEN → INVESTIGATING → {RESOLVED, REJECTED}; INVESTIGATING → INVESTIGATING allowed (note update); OPEN → RESOLVED refused (`FAILED_PRECONDITION`); RESOLVED on a return-linked dispute requires a ruling (`INVALID_ARGUMENT`).
     - Ruling `SELLER_FAVOUR` or a REJECTED dispute moves no return and no money.
     - A COD order's return is APPROVED only (no online refund, as today); a return already REFUNDED is left unchanged and the event is acknowledged.
     - Re-approving a REJECTED return re-checks the order's returnable remainder; when it no longer fits, the return stays as it is and the failure is logged and dead-lettered for CS.
     - team-audit down in team-order: the event is redelivered, no change.

## Non-goals
- Reports on shops, reviews, Q&A answers or chat messages; chat block-user and chat restriction for `restricted` users; off-platform contact filtering.
- KYC queue in the inbox (`ListPendingKyc`); case assignment, SLA timers, macros (later Q23 items).
- Image scanning; a penalty-points ledger; appeal flow for sellers; email/push notification of a moderation decision or ruling.
- Cancelling or refunding a suspended or banned seller's open orders.
- Splitting the `admin` role or admin MFA (trust-ops #10).
- Return window, `can_escalate`, `ReturnRequested`, seller return notifications and the escalation threshold: owned by q10-order-lifecycle, consumed here.

## Architecture decisions
- team-domain owns listing status. The first task of domain-listing-moderation is the quick fix (X3): `CreateWithEvent`/`UpdateWithEvent` in `team-domain/internal/service/listing.go` ignore the client `status` beyond DRAFT / publish request, and `REJECTED` stays sticky. The service then derives the stored status from the state machine `DRAFT → (screen) → PUBLISHED | PENDING_REVIEW`, `PENDING_REVIEW → PUBLISHED | REJECTED | BANNED` (admin), `REJECTED → PENDING_REVIEW` (seller edit), `PUBLISHED | PENDING_REVIEW → HELD → stored prior status` (enforcement). An admin's `UpdateListing` keeps its Q3 behaviour but can no longer set status; status moves only through `ModerateListing`. Why: closes the un-reject hole in one place and keeps existing clients and e2e seeds working, since a clean listing sent as `PUBLISHED` still ends up published.
- Auto-screen runs on the write path in team-domain: a local prohibited-term list first (a hit → `PENDING_REVIEW`, flag `keyword:<term>`), then `AIService.ScreenListing` on team-ai as a service principal with a 2 s timeout. `REVIEW` → `PENDING_REVIEW`; error or timeout → `PENDING_REVIEW` (fail safe: an outage slows publishing but never lets an unscreened listing through). Why: a sync call mirrors team-search → `ClassifyTags`; an async screen pipeline costs two events and a consumer for one bit.
- The prohibited-term list is a config file in team-domain (VI and EN terms, case- and diacritic-insensitive), seeded from the prohibited-goods categories of Decree 52/2013 and Decree 85/2021; legal/T&S sign-off on the list is required before launch (NEEDS_HUMAN). Why: changes are policy edits, not deploys of code.
- team-ai `ScreenListing` follows the project LLM rule: backend `SCREEN_BACKEND: Literal["mock", "llm_router"] = "mock"`; the team-ai classifier is kept, mock is the default and deterministic (`REVIEW` iff the text contains a fixture marker, else `ALLOW`) so e2e is hermetic. Scope `ai.classify` (service-only, already held by service principals); not routed by the gateway. Why: no new scope in team-identity, no LLM in tests.
- team-ai's listing indexer indexes only `PUBLISHED` (today it deletes on `DRAFT`/`REJECTED`, so a new status would be indexed). team-search needs no logic change: its published-only default already hides unknown statuses. Why: de-index follows from `ListingChanged`, which every status change already emits.
- No `ListingModerated` event: `ListingChanged(UPDATED)` with the new status plus the `moderation` field on `Listing` carries everything search, AI and the seller center need. Why: one event per write already exists; a second would be a dual source of truth.
- team-identity owns `users.status` (+ `status_reason`, `status_until`). `SetUserStatus` = audit first (fail closed) → update status → revoke all sessions and refresh tokens, emitting one `SessionRevoked` per session (existing gateway denylist path) and one `UserStatusChanged` through the identity outbox, all in one transaction. Why: reuses Q1's revocation path; lockout needs no gateway change.
- `restricted` is enforced at token issue: `ScopesForRoles` minus `listing.write` for a restricted user; it blocks listing writes only, chat restriction is a later item (team-chat has no scope gate to remove). Why: no new checks in team-domain; the session revoke forces a new token.
- team-domain gains its first Kafka consumer (`identity.events`, own consumer group), idempotent on envelope event id, mirroring `team-search/internal/consumer/listing.go`. It holds listings on SUSPENDED and BANNED and restores the stored prior status on ACTIVE or RESTRICTED. Why: AGENTS.md rule 5; identity must not call domain synchronously.
- Reports live in team-engagement (where disputes live), target type `LISTING` only; `target_id` is checked for UUID shape, not existence (no new listing upstream in engagement); the admin view shows a missing listing as removed. Why: smallest cut that gives buyers recourse.
- Dispute escalation (from q10): `CreateDispute` gains `return_id`; team-engagement checks the return synchronously with `GetReturnRequest` (service principal, `order.read`) and requires `can_escalate`; team-order does not consume dispute creation. The dispute state machine is tightened in team-engagement. Why: the dispute owns reason and evidence; the return keeps its status until a ruling.
- A ruling crosses back as an event: team-engagement emits `DisputeResolved` on `engagement.events` through its outbox; team-order consumes it with a new consumer (deduped by event id) and refunds via the existing `ReturnRefunded` → team-payment saga. The refund is a system money action: team-order writes `return.dispute_refund` to team-audit before the transition, fail closed by redelivery. Why: AGENTS.md rule 5 (state change = Kafka event); Q3 rule "money actions audit first".
- Cross-initiative dependency: engagement-dispute-escalation and order-dispute-refund build only after q10-order-lifecycle has merged to main (they need its `order-receipt-completion` slice: return window, `OrderReturn.can_escalate`, `ReturnRequested`). Every returns-related field stays in q10's contract and is only consumed here. Letting `GetReturnRequest` accept a service principal with `order.read` is a team-order task inside engagement-dispute-escalation (q10 does not provide it). All other slices of this initiative build in parallel with q10 and do not wait.
- Admin RPCs (`ModerateListing`, `SetUserStatus`, `ResolveReport`, existing `ResolveDispute`) write team-audit first, fail closed (`UNAVAILABLE`); list RPCs (`ListModerationQueue`, `ListUsers`, `ListReports`, `ListDisputes`) are reads and are not audited. New audit actions: `listing.moderate`, `user.set_status`, `report.resolve`, `return.dispute_refund`. Why: AGENTS.md rule 6.
- No new dependencies.

## Contracts
```proto
// platform/listing/v1/listing.proto — additive
enum ListingStatus { /* existing 0-3 */ LISTING_STATUS_PENDING_REVIEW = 4; LISTING_STATUS_BANNED = 5; LISTING_STATUS_HELD = 6; }
message Listing { /* existing 1-11 */ ModerationInfo moderation = 12; } // server-set; ignored on writes
message ModerationInfo {
  string reason_code = 1;   // prohibited_item | counterfeit | misleading | inappropriate | duplicate | seller_enforcement | other
  string note = 2;          // admin note shown to the seller, ≤500 chars
  repeated string screen_flags = 3; // e.g. "keyword:<term>", "ai:<label>"; admin/owner only
  google.protobuf.Timestamp decided_at = 4;
}
// service ListingService — admin only (scope admin); edge-routed.
rpc ModerateListing(ModerateListingRequest) returns (ModerateListingResponse);
rpc ListModerationQueue(ListModerationQueueRequest) returns (ListModerationQueueResponse);
enum ModerationDecision { MODERATION_DECISION_UNSPECIFIED = 0; MODERATION_DECISION_APPROVE = 1; MODERATION_DECISION_REJECT = 2; MODERATION_DECISION_BAN = 3; }
message ModerateListingRequest { string listing_id = 1; ModerationDecision decision = 2; string reason_code = 3; string note = 4; }
// FAILED_PRECONDITION when the transition is not allowed (e.g. APPROVE on HELD); reason_code required for REJECT/BAN.
message ModerateListingResponse { Listing listing = 1; }
message ListModerationQueueRequest { platform.common.v1.PageRequest page = 1; } // PENDING_REVIEW, oldest submission first
message ModerationQueueItem { Listing listing = 1; google.protobuf.Timestamp submitted_at = 2; }
message ListModerationQueueResponse { repeated ModerationQueueItem items = 1; platform.common.v1.PageResponse page = 2; }

// platform/ai/v1/ai.proto — add to AIService. Internal (service principal, scope ai.classify); not routed.
rpc ScreenListing(ScreenListingRequest) returns (ScreenListingResponse);
message ScreenListingRequest { string listing_id = 1; string title = 2; string description = 3; string category_id = 4; }
enum ScreenVerdict { SCREEN_VERDICT_UNSPECIFIED = 0; SCREEN_VERDICT_ALLOW = 1; SCREEN_VERDICT_REVIEW = 2; }
message ScreenListingResponse { ScreenVerdict verdict = 1; repeated string labels = 2; string backend = 3; } // backend: mock | llm_router

// platform/identity/v1/identity.proto — new service, admin only (scope admin); edge-routed.
service AccountAdminService {
  rpc ListUsers(ListUsersRequest) returns (ListUsersResponse);
  // Audit first (user.set_status), fail closed. FAILED_PRECONDITION for self or an admin target;
  // INVALID_ARGUMENT for empty reason_code, or until missing/past for SUSPENDED / set for others.
  rpc SetUserStatus(SetUserStatusRequest) returns (SetUserStatusResponse);
}
enum UserStatus { USER_STATUS_UNSPECIFIED = 0; USER_STATUS_ACTIVE = 1; USER_STATUS_RESTRICTED = 2; USER_STATUS_SUSPENDED = 3; USER_STATUS_BANNED = 4; }
message AdminUser {
  string id = 1; string username = 2; repeated string roles = 3; UserStatus status = 4;
  string status_reason = 5; google.protobuf.Timestamp status_until = 6; google.protobuf.Timestamp created_at = 7;
}
message ListUsersRequest { string query = 1; UserStatus status = 2; platform.common.v1.PageRequest page = 3; } // query: username prefix or exact id
message ListUsersResponse { repeated AdminUser users = 1; platform.common.v1.PageResponse page = 2; }
message SetUserStatusRequest { string user_id = 1; UserStatus status = 2; string reason_code = 3; string note = 4; google.protobuf.Timestamp until = 5; }
message SetUserStatusResponse { AdminUser user = 1; int32 sessions_revoked = 2; }
// Kafka identity.events, key = user id, EventEnvelope type "platform.identity.v1.UserStatusChanged".
message UserStatusChanged {
  string user_id = 1; UserStatus status = 2; UserStatus previous_status = 3;
  string reason_code = 4; google.protobuf.Timestamp until = 5; google.protobuf.Timestamp changed_at = 6;
}

// platform/engagement/v1/engagement.proto — add to EngagementService.
rpc CreateReport(CreateReportRequest) returns (CreateReportResponse);   // signed-in user (engagement:write)
rpc ListReports(ListReportsRequest) returns (ListReportsResponse);      // admin
rpc ResolveReport(ResolveReportRequest) returns (ResolveReportResponse); // admin; audit first (report.resolve)
enum ReportTargetType { REPORT_TARGET_TYPE_UNSPECIFIED = 0; REPORT_TARGET_TYPE_LISTING = 1; }
enum ReportStatus { REPORT_STATUS_UNSPECIFIED = 0; REPORT_STATUS_OPEN = 1; REPORT_STATUS_ACTIONED = 2; REPORT_STATUS_DISMISSED = 3; }
message Report {
  string id = 1; ReportTargetType target_type = 2; string target_id = 3;
  string reason_code = 4; // prohibited_item | counterfeit | scam | misleading | inappropriate | other
  string note = 5; string reporter_id = 6; ReportStatus status = 7;
  google.protobuf.Timestamp created_at = 8; google.protobuf.Timestamp resolved_at = 9; string resolution_note = 10;
}
message CreateReportRequest { ReportTargetType target_type = 1; string target_id = 2; string reason_code = 3; string note = 4; }
message CreateReportResponse { Report report = 1; } // idempotent per (reporter, target) while OPEN
message ListReportsRequest { ReportStatus status = 1; string target_id = 2; platform.common.v1.PageRequest page = 3; } // oldest first
message ListReportsResponse { repeated Report reports = 1; platform.common.v1.PageResponse page = 2; }
message ResolveReportRequest { string report_id = 1; ReportStatus status = 2; string resolution_note = 3; } // ACTIONED | DISMISSED
message ResolveReportResponse { Report report = 1; }

// Disputes (moved from q10-order-lifecycle).
enum DisputeRuling { DISPUTE_RULING_UNSPECIFIED = 0; DISPUTE_RULING_BUYER_FAVOUR = 1; DISPUTE_RULING_SELLER_FAVOUR = 2; }
// CreateDisputeRequest += string return_id = 5;   // optional; requires OrderReturn.can_escalate (q10)
// Dispute += string return_id = 11; DisputeRuling ruling = 12;
// ResolveDisputeRequest += DisputeRuling ruling = 4; // required for RESOLVED on a return-linked dispute
rpc ListOrderDisputes(ListOrderDisputesRequest) returns (ListOrderDisputesResponse); // order buyer or seller, or admin (A18); newest first
message ListOrderDisputesRequest { string order_id = 1; }
message ListOrderDisputesResponse { repeated Dispute disputes = 1; }
rpc ListDisputes(ListDisputesRequest) returns (ListDisputesResponse); // admin; oldest first; not audited
message ListDisputesRequest { DisputeStatus status = 1; platform.common.v1.PageRequest page = 2; }
message ListDisputesResponse { repeated Dispute disputes = 1; platform.common.v1.PageResponse page = 2; }

// platform/engagement/v1/events.proto — engagement.events (outbox, key = dispute id)
message DisputeResolved {        // once per dispute reaching RESOLVED or REJECTED
  string dispute_id = 1; string order_id = 2; string return_id = 3;
  DisputeStatus status = 4; DisputeRuling ruling = 5;
  string resolver_id = 6;        // admin id; carried into the team-order audit event
  google.protobuf.Timestamp resolved_at = 7;
}

// platform/audit/v1/audit.proto — catalogue comment only: + listing.moderate, user.set_status, report.resolve, return.dispute_refund.
```
Exact field names, error codes and reason-code lists are fixed by the contract slice's spec. Engagement field numbers are coordinated with the q10 contract slice (which keeps `OrderReturn.can_escalate` and the return-window fields) before either merges.
Existing contracts consumed unchanged: `ListingService.CreateListing/UpdateListing/GetListing/ListMyListings`, `listing.events/ListingChanged`, `identity.events/SessionRevoked` (gateway denylist), `AuthService.Login/RefreshSession`, `AuditService.WriteAuditEvent`, `AuditService.QueryAuditLog` (user history in `/admin/audit`), `EngagementService.CreateDispute/GetDispute/ResolveDispute`, `OrderService.GetOrder/GetReturnRequest/UpdateReturnStatus` path, `order.events/ReturnRefunded`; from q10: `OrderReturn.can_escalate`, return window.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| Moderation, enforcement, report and dispute contracts | q6-trust-contract | platform-core | – | provides: all items in Contracts | platform-core (+ proto copies) |
| Listing screen verdict, mock by default; only published listings in RAG | ai-listing-screen | team-ai | q6-trust-contract | provides: AIService.ScreenListing | team-ai, platform-e2e |
| Server-owned status (task 1: X3 quick fix), auto-screen, admin moderation queue | domain-listing-moderation | team-domain | ai-listing-screen | uses: AIService.ScreenListing, AuditService.WriteAuditEvent; provides: ModerateListing, ListModerationQueue | team-domain, team-gateway, platform-e2e |
| Admin restricts/suspends/bans a user; sessions end; login refused | identity-user-enforcement | team-identity | q6-trust-contract | uses: AuditService.WriteAuditEvent; provides: AccountAdminService, UserStatusChanged | team-identity, team-gateway, platform-e2e |
| Suspended or banned seller's listings leave the store and return on reinstatement | domain-seller-enforcement-hold | team-domain | domain-listing-moderation, identity-user-enforcement | uses: identity.events/UserStatusChanged | team-domain, platform-e2e |
| Buyer reports a listing; admin resolves the report | engagement-listing-reports | team-engagement | q6-trust-contract | uses: AuditService.WriteAuditEvent; provides: CreateReport, ListReports, ResolveReport | team-engagement, team-order, team-gateway, platform-e2e |
| Return escalates to a dispute; ruling; admin dispute list | engagement-dispute-escalation | team-engagement | engagement-listing-reports; **after q10 merges** | uses: GetReturnRequest, GetOrder; provides: CreateDispute.return_id, ResolveDispute.ruling, ListOrderDisputes, ListDisputes, DisputeResolved | team-engagement, team-gateway, platform-e2e |
| Buyer-favour ruling refunds the return | order-dispute-refund | team-order | engagement-dispute-escalation; **after q10 merges** | uses: engagement.events/DisputeResolved, WriteAuditEvent, UpdateReturnStatus path, ReturnRefunded | team-order, platform-e2e |
| /admin/listings, /admin/cases (reports + queue), report button, seller sees status + reason | frontend-listing-moderation | team-frontend | domain-listing-moderation, engagement-listing-reports | uses: ModerateListing, ListModerationQueue, CreateReport, ListReports, ResolveReport | team-frontend, platform-e2e |
| /admin/users with status actions; suspended/banned login message | frontend-user-enforcement | team-frontend | identity-user-enforcement | uses: AccountAdminService.ListUsers, SetUserStatus, AuthService.Login | team-frontend, platform-e2e |
| "Khiếu nại lên Agora" on order detail; /admin/cases tab "Khiếu nại" with resolve action | frontend-dispute-cases | team-frontend | frontend-listing-moderation, order-dispute-refund | uses: CreateDispute, ListOrderDisputes, ListDisputes, GetDispute, ResolveDispute | team-frontend, platform-e2e |

Hot files edited by more than one slice (initiative-build serialises those slices):
- `team-gateway/internal/edge/policy.go` (admin scope map) — domain-listing-moderation, identity-user-enforcement, engagement-listing-reports, engagement-dispute-escalation.
- `team-gateway/internal/edge/engagement.go` forwarder — engagement-listing-reports then engagement-dispute-escalation; `server.go` (register `AccountAdminService`) — identity-user-enforcement only; `listing.go` — domain-listing-moderation only.
- `team-domain/internal/service/listing.go`, `internal/handler/listing.go`, `internal/bootstrap/` — domain-listing-moderation then domain-seller-enforcement-hold (sequential by depends_on).
- `team-engagement/internal/handler/`, `internal/service/`, `migrations/`, `internal/events/relayer.go` — engagement-listing-reports then engagement-dispute-escalation (sequential by depends_on; migration numbers follow it).
- `team-frontend/src/app/(shop)/admin/cases/` — created by frontend-listing-moderation, extended by frontend-dispute-cases (sequential by depends_on); `src/components/shell/ConsumerShell.tsx` (admin links) and `src/features/admin/` — all three frontend slices.
- Cross-initiative (resolved by building the dispute slices after q10 merges): `team-order/cmd/server/main.go`, `internal/consumer/`, `migrations/` (order-dispute-refund numbers follow q10's); `team-gateway/internal/edge/engagement.go`, `policy.go` (q10 order-receipt-completion); `team-frontend/src/features/order/OrderDetailView.tsx` (q10 frontend-order-lifecycle, q11-checkout-coherence).
- `platform-e2e/tests/e2e/support/` (admin login, audit_support.py reuse, listing seed helpers, order/return helpers) and `team-domain/FEATURES.yaml` — any slice adding a helper; domain slices sequential.

## Out of initiative
- team-search: regenerated stubs only; its published-only default already hides `PENDING_REVIEW`/`BANNED`/`HELD`. `statusString` mapping the new values to strings is a one-line task of domain-listing-moderation if an admin search filter needs it.
- `platform-core/docs/ROADMAP.md:63` "Admin: approve/lock listings" row flipped by the contract slice (doc).
- `MASTER_CAPABILITY_MATRIX.md` admin drift (trust-ops "doc drift"): not touched.
- `team-engagement/README.md` Known gaps line on the loose dispute state machine is removed by engagement-dispute-escalation (doc).
- q10-order-lifecycle drops its two dispute slices, the engagement/dispute part of its contract and the escalate button / `ListOrderDisputes` from frontend-order-lifecycle (edited by the q10 planner, not here).

## NFR and data
- NFR: CreateListing/UpdateListing add ≤ 2 s (screen timeout) only on content writes; keyword check < 5 ms. Enforcement lockout < 1 min end to end (Kafka → gateway denylist; SSE ends per `edge.sse-ends-on-session-revoke`). Moderation queue page ≤ 200 ms at 10k pending. Listing hold/restore after `UserStatusChanged` < 1 min. A buyer-favour ruling reaches REFUNDED within one consumer retry cycle; refund once per return id.
- Money: int64 VND; no new money path — dispute refunds reuse `ReturnRefunded` in team-payment, once per return.
- Personal data:
  - `users.status`, `status_reason`, `status_until` and the admin note (in the audit event) — enforcement record about a person; basis: platform duty to act against violations (Decree 85/2021); retained while the account exists, audit copy per Q3.
  - `reports.reporter_id` and free-text `note` (may contain PII) — complaint handling; never shown to the reported seller; closed reports are kept 2 years after `resolved_at`, then `reporter_id` is pseudonymised and `note` dropped (a sweep task of engagement-listing-reports).
  - Disputes: no new personal fields (reason/evidence already stored in team-engagement); `DisputeResolved` carries ids, status and ruling only.
  - Listing title/description sent to team-ai for screening — not personal by design; mock backend sends nothing outside the cluster.
- Risk routing expected: auth (new admin RPCs, restricted scope set, login refusal, `ListOrderDisputes` party check, service-principal `order.read`), pii (enforcement record, report notes), payment (dispute refund), proto (contract slice).

## NEEDS_HUMAN
- Legal/T&S sign-off on the seeded prohibited-term list (Decree 52/85 categories) before launch.
- DPO sign-off on the enforcement-record basis and the 2-year report retention with pseudonymisation.
- Coordinate with the q10 planner: q10 removes the dispute slices and engagement proto items; engagement field numbers agreed between the two contract slices; q10 keeps `OrderReturn.can_escalate` in order-receipt-completion; service-principal `GetReturnRequest` is done here (engagement-dispute-escalation, team-order task).
- Start engagement-dispute-escalation and order-dispute-refund only after q10-order-lifecycle is merged to main.

## Conventions
- Build order across initiatives: q6-trust-enforcement builds in parallel with q10-order-lifecycle, except engagement-dispute-escalation, order-dispute-refund and frontend-dispute-cases, which wait for q10 to merge; q11-checkout-coherence builds after q10.
- Audit-first pattern: mirror `team-engagement/internal/handler` `HideReview` + `team-domain/internal/upstream/audit.go` (Q3 `domain-admin-listing-audit`); FEATURES ids in the style `listing.admin-audit-*`.
- Session revoke: mirror `team-identity/internal/service/refresh.go` `revokedEvent` and the outbox in `internal/repository/outbox.go`; Kafka consumer: mirror `team-search/internal/consumer/listing.go`.
- team-ai backend switch: mirror `ChatBackend` in `team-ai/app/core/config/ai.py` (exhaustive `match`); indexer change in `team-ai/app/modules/messaging/listing_indexer/handler.py` and `decode.py`.
- Disputes: team-engagement outbox + relayer `internal/events/relayer.go` (as `ReviewCreated`), dispute audit tests `handler/disputes_audit_test.go`, upstream `internal/upstream/order.go`; team-order consumer mirrors `consumer/payment.go` (dedupe ledger, DLQ, `ErrPermanent`) and `handler.auditAdmin` with its service principal for `return.dispute_refund`.
- Gateway: admin RPCs in `policy.go` `adminScope`; extend `policy_edge_test.go`, `unrouted_test.go` (ScreenListing not routed).
- Frontend: admin pages under `team-frontend/src/app/(shop)/admin/` like `audit/page.tsx` (guest → login, non-admin → 403 view); VN UI copy.
- E2E: listings flagged through the mock marker term and a seeded keyword; team-audit stop/start via `audit_support.py` for fail-closed scenarios; return timers shortened via q10's compose overlay, never by sleeping.

## Locked
2026-10-16 — Confirmed by Bùi Đăng Phúc (recommended defaults accepted). Suspended and banned sellers' listings are HELD and return to their stored prior status on reinstatement; Q23 cut = reports + moderation queue + Dispute tab in `/admin/cases` (chat reports, KYC later); prohibited-term seed from Decree 52/85 with legal sign-off before launch; restricted blocks listing writes only; closed reports kept 2 years then pseudonymised; team-ai classifier kept, mock default; X3 quick fix is task 1 of domain-listing-moderation; the dispute half moves here from q10 and its slices build after q10 merges (q10 provides `can_escalate` and the return window; service-principal `GetReturnRequest` is a team-order task here).

## Clarifications — spec-clarify round 1 (2026-10-16, Bùi Đăng Phúc: "approve hết", all recommended answers accepted)
Money / risk
- M1 ProcessMockPayment re-reads the order (team-order GetOrder) before settling; a CANCELLED order refuses settle (closes the CreatePayment-vs-cancel race). Owner: payment-unpaid-cancel.
- M2 An APPROVED return on a COD order (no PaidAt) does not block auto-complete; COD refunds are a follow-up. Owner: order-receipt-completion.
- M3 No auto-refund of an APPROVED return the seller never refunds; follow-up.
- M4 ReserveStock / checkout on a HELD listing is refused FAILED_PRECONDITION. Owner: domain-seller-enforcement-hold (team-domain reservation path).
- M5 Legacy CreateOrder.voucher_code is treated as the platform-voucher slot only; no SHOP auto-routing probe. Owner: order-multishop-checkout.
Trust (q6)
- A8 Every seller content edit of a PUBLISHED listing is re-screened; a flagged edit moves it to PENDING_REVIEW. Status omitted must be distinguishable from DRAFT (fix domainStatus UNSPECIFIED handling).
- A9 Admin REJECT/BAN allowed from PENDING_REVIEW and PUBLISHED; no reversal of REJECTED/BANNED (APPROVE refused).
- A10 Seller DeleteListing on BANNED/HELD → FAILED_PRECONDITION.
- A11 screen_flags are admin-only; the owner sees reason_code + note only.
- A12 An admin's own listing follows the seller path (screen runs), still audited.
- A13 ListingChanged events carry no moderation note and no screen flags.
- A14 SetUserStatus RESTRICTED/SUSPENDED/BANNED (including re-setting the same status) revokes all sessions; ACTIVE revokes none. Contract and identity slice must say the same.
- A15 Expired suspension is lifted only at the user's next login (listings stay HELD until then); a lift sweeper is a follow-up.
- A16 Login refusal is one generic message ("tài khoản bị khóa"); no suspended/banned distinction or end time.
- A17 Dispute state machine allows OPEN→INVESTIGATING and OPEN→REJECTED (spam); INVESTIGATING→{RESOLVED,REJECTED}. A ruling with any status other than RESOLVED is INVALID_ARGUMENT (contract comment changes to match).
- A18 Re-escalation of a return after its dispute closed is allowed while can_escalate is true. ListOrderDisputes is allowed to the order's buyer and seller (and admin).
- A19 Max 20 OPEN reports per reporter (RESOURCE_EXHAUSTED); ResolveReport note required, 1–500 chars.
- A20 A restricted seller visiting seller pages sees a "Tài khoản bị hạn chế" notice instead of a /login redirect loop. Owner: frontend-user-enforcement.
Order lifecycle (q10)
- B7 Pre-deploy orders: unpaid orders get no payment_due_at (never auto-expired); backfill SHIPPED auto_complete_at = migration+7d, COMPLETED return_window_ends_at = updated_at+15d, PENDING returns auto_approve_at = migration+3d (fixed values, stated in the proposal).
- B8 ConfirmReceipt is allowed while the buyer's return is PENDING or APPROVED.
- B9 A buyer sending a system-only cancel reason gets PERMISSION_DENIED.
- B10 Buyer notifications: ReturnRefunded yes; OrderCancelled only when reason is PAYMENT_EXPIRED; OrderCompleted always.
Checkout (q11)
- C8 VoucherQuote.scope uses a local enum in order.proto (no import of promotion.proto, no extra proto copies).
- C9 A buyer with addresses but no default and an empty address_id → FAILED_PRECONDITION "address not found" (no first-address fallback).
- C10 Cart default selection = all ticked; on a partial selection /cart hides the voucher row and says "Giảm giá được tính ở bước thanh toán".
Cross-slice fixes (orchestrator)
- q6 engagement proto: q10 no longer touches engagement.proto — the "coordinate field numbers with q10" note is moot; q6-trust-contract owns them.
- q10 contract test must NOT assert "no Order field >= 25" (q11 uses 25–27).
- q6 ownership: duplicate scenario titles between engagement-listing-reports / frontend-listing-moderation / frontend-user-enforcement must be made unique (prefix each with its slice's capability, e.g. "Moderation UI: …", "Users UI: …").
- Every frontend-called RPC must have a gateway forwarder in a named slice (PreviewCheckout → order-multishop-checkout; ConfirmReceipt → order-receipt-completion; AccountAdminService → identity-user-enforcement; ListOrderDisputes/ListDisputes → engagement-dispute-escalation).
- The mock screen default must ALLOW clean text so existing e2e seeds that publish listings still auto-publish.
NEEDS_HUMAN before release: DPO (enforcement record basis, 2-year report retention, Langfuse traces when a real LLM is on, dispute audit metadata); legal sign-off on the prohibited-term list; Kafka ACL on identity.events.
