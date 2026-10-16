# 00-spec — Audit log for privileged actions

initiative: q3-audit-log

Source: backlog Q3 (`plans/pm-review-2026-10/BACKLOG.md`), evidence `plans/pm-review-2026-10/trust-ops.md` #5.
Base: stacked on `asdlc/q2-review-integrity` (Q2 PRs agora#24/#25), which already adds the team-engagement audit client and `HideReview` (`review.hide`).

## Goal
Every state change that succeeds only because the caller holds `admin` (or `order.admin`) writes one team-audit event naming the admin, the action and the target before the change is applied. If team-audit cannot be written, the action is refused and nothing changes. An admin can open `/admin/audit` and answer "who touched this order/listing/user, and when" by filtering on target, actor, action and time.

## User stories
1. **Admin action leaves a trail.**
   - Given an admin (scopes include `admin`).
   - When they perform a privileged action from the catalogue below (e.g. `ReviewKyc` approve).
   - Then the action succeeds and team-audit holds exactly one event with `actor_id` = the admin's user id, the catalogue `action`, `target_type` and `target_id`, and the catalogue metadata keys.
   - Edge cases:
     - The same RPC called by the resource owner on their own resource (a seller updating their own order) writes no audit event.
     - A call refused before the audit write (unauthenticated, permission denied, invalid argument, not found) writes no event.
2. **Audit down blocks the action.**
   - Given team-audit errors, times out, or `UPSTREAM_AUDIT_ADDR` is unset in the producing service.
   - When an admin performs a privileged action.
   - Then the RPC returns `Unavailable`, and the target's state is unchanged (no row change, no event emitted downstream).
   - Calls by non-admin owners are not affected by team-audit availability.
3. **Admin investigates.**
   - Given an admin on `/admin/audit`.
   - When they filter by any combination of target type + target id, actor id, action, and a date range.
   - Then they see matching events newest first with time, actor, action, target and metadata, paginated (20 per page, "next page").
   - Edge cases:
     - No match → an empty state "Không có sự kiện nào".
     - A non-admin opening `/admin/audit` sees the same 403 result as `/admin/cockpit`; an anonymous visitor is redirected to `/login`.
     - `created_before` earlier than `created_after` → `InvalidArgument`.

## Privileged action catalogue
"Admin path" = the call succeeds because of the admin scope, not because the caller owns the resource.

| service | RPC (admin path) | action | target_type / target_id | metadata keys |
|---|---|---|---|---|
| team-verification | ReviewKyc (always admin) | `kyc.review` | `kyc_submission` / submission id | `decision`, `user_id` |
| team-engagement | ResolveDispute (always admin) | `dispute.resolve` | `dispute` / dispute id | `resolution`, `order_id` |
| team-domain | UpdateListing by an admin who is not the owner | `listing.admin_update` | `listing` / listing id | `seller_id`, `status` |
| team-domain | DeleteListing by an admin who is not the owner | `listing.admin_delete` | `listing` / listing id | `seller_id` |
| team-order | ForceFailSaga (always admin + order.admin) | `order.force_fail` | `order` / order id | `fail_step` |
| team-order | UpdateOrderStatus by an admin who is not the seller | `order.admin_status_update` | `order` / order id | `seller_id`, `to_status` |
| team-order | UpdateReturnStatus by an admin who is not the seller | `return.admin_status_update` | `return_request` / return id | `order_id`, `to_status` |
| team-order | CreateShipment by an admin who is not the seller | `shipment.admin_create` | `order` / order id | `carrier` |
| team-payment | RefundPayment by an admin who is not the seller | `payment.admin_refund` | `payment` / payment id | `order_id`, `amount` |
| team-promotion | CreateVoucher with PLATFORM (or unspecified) scope | `voucher.platform_create` | `voucher` / code | `discount_type`, `value` |
| team-promotion | CreateCampaign (flash sale) by an admin for another seller | `flash_sale.admin_create` | `flash_sale` / campaign id or name | `seller_id` |
| team-promotion | CreateAdCampaign by an admin for another seller | `ad_campaign.admin_create` | `ad_campaign` / campaign id or name | `seller_id`, `budget` |

Already shipped in Q2: `review.hide` (team-engagement). Each slice's spec may refine metadata keys to what the RPC actually has, but never adds free text beyond an existing reason/note field.

## Non-goals
- Seller- or system-initiated money flows: seller refunds, payouts, settlement credit, referral rewards, plan subscribe (later; would need an outbox).
- A transactional outbox or async delivery for audit events. An idempotency key on `WriteAuditEvent` (retries may write a second row, as in Q2).
- `ProcessMockPayment` by an admin (mock by design).
- Read-only admin access (GetDispute, wallet reads, analytics) — no audit for reads.
- New admin actions (listing moderation Q6, user enforcement Q8); they will follow the rule this initiative sets.
- Export/CSV, retention or erasure policy for audit rows.
- team-ai admin REST routes (Python).

## Architecture decisions
- **Audit first, fail closed.** In each producer: authenticate → authorize → validate → load the target (NotFound) → write the audit event → apply the change. Audit error, timeout or a nil client → `Unavailable`, no change. If the change fails after a successful audit write, the RPC returns the change's own error (e.g. `FailedPrecondition` for an invalid transition) and the audit row stays as a record of the attempt. Why: no outbox in verification/promotion; admin actions are rare, so blocking them while audit is down is acceptable (human decision 2026-10-15). Same pattern as Q2 `HideReview`.
- **Service principal per producer.** Each producer dials team-audit with its own interceptor that replaces the outgoing principal with `x-principal-id=service-<repo>`, `x-principal-type=service`, `x-principal-scopes=audit.write`, and passes the admin's id as `actor_id`. Never forward the user's principal. Mirror `team-engagement/internal/upstream/audit.go` (a per-repo copy; no shared SDK package, since only team-order and team-promotion import go-sdk).
- **Config.** `UPSTREAM_AUDIT_ADDR` (default unset) and the existing `UPSTREAM_CALL_TIMEOUT_SECONDS` style timeout in every producer. Unset → privileged actions return `Unavailable`; owner paths unaffected.
- **QueryAuditLog filters** (additive, wire-compatible): `target_id`, `action` (exact match), `created_after` (inclusive), `created_before` (exclusive). All filters AND together. Existing paging (offset cursor, default 20, max 100, newest first) unchanged. Uses the existing `(target_type, target_id, created_at DESC)` and `(actor_id, created_at DESC)` indexes; add an index on `(action, created_at DESC)` only if the slice's NFR needs it.
- **Gateway unchanged in policy**: `QueryAuditLog` is already in `adminProcedures`; `WriteAuditEvent` stays unrouted. The gateway's proto copy is regenerated so the new fields pass through.
- **Viewer** is a server-rendered admin page in team-frontend reusing the cockpit's admin gate (`principal.scopes.includes("admin")`), filters in the URL query string, linked from the cockpit page.
- **Rule for future admin RPCs**: AGENTS.md gains one rule ("an RPC whose mutation depends on the admin/order.admin scope writes a team-audit event first, fail closed") and `auth-scope-reviewer` gains the matching check. Done in the contract slice.
- **No new dependencies.**

## Contracts
```proto
// platform/audit/v1/audit.proto — QueryAuditLogRequest (additive)
message QueryAuditLogRequest {
  optional string actor_id = 1;
  optional string target_type = 2;
  platform.common.v1.PageRequest page = 3;
  optional string target_id = 4;                        // exact match
  optional string action = 5;                           // exact match, e.g. "kyc.review"
  google.protobuf.Timestamp created_after = 6;          // inclusive; unset = no lower bound
  google.protobuf.Timestamp created_before = 7;         // exclusive; unset = no upper bound; < created_after → InvalidArgument
}
// WriteAuditEventRequest: unchanged; comment lists the action catalogue naming rule "<entity>.<verb>".
```
The proto is copied into every consumer's `proto/`: team-audit, team-gateway, team-frontend, team-analytics (existing copies) and the producers team-verification, team-domain, team-order, team-payment, team-promotion (new copies; team-engagement already has one from Q2).

Existing contracts consumed unchanged: `AuditService.WriteAuditEvent` (service principal + `audit.write`), `AuditService.QueryAuditLog` (admin, edge-routed).

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner (FEATURES.yaml repo) | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| QueryAuditLog filters + action naming rule | q3-audit-log-contract | platform-core | – | provides: AuditService.QueryAuditLog (filters) | platform-core, proto copies in all consumers, root AGENTS.md, `.claude/agents/auth-scope-reviewer.md`, root `docker-compose.services.yaml` |
| Admin filters the audit log by target, action, time | audit-query-filters | team-audit | q3-audit-log-contract | uses: AuditService.QueryAuditLog | team-audit, team-gateway (regen only), platform-e2e (shared audit support helper) |
| KYC review is audited | verification-kyc-audit | team-verification | audit-query-filters | uses: AuditService.WriteAuditEvent | team-verification, platform-e2e |
| Dispute resolution is audited | engagement-dispute-audit | team-engagement | audit-query-filters | uses: AuditService.WriteAuditEvent | team-engagement, platform-e2e |
| Admin listing edit/delete is audited | domain-admin-listing-audit | team-domain | audit-query-filters | uses: AuditService.WriteAuditEvent | team-domain, platform-e2e |
| Admin order/return/shipment/force-fail is audited | order-admin-audit | team-order | audit-query-filters | uses: AuditService.WriteAuditEvent | team-order, platform-e2e |
| Admin refund is audited | payment-admin-refund-audit | team-payment | audit-query-filters | uses: AuditService.WriteAuditEvent | team-payment, platform-e2e |
| Platform voucher and admin-for-seller campaigns are audited | promotion-admin-audit | team-promotion | audit-query-filters | uses: AuditService.WriteAuditEvent | team-promotion, platform-e2e |
| Admin audit viewer page | frontend-audit-viewer | team-frontend | audit-query-filters | uses: AuditService.QueryAuditLog | team-frontend, platform-e2e |

Hot files edited by more than one slice:
- Root `docker-compose.services.yaml`: only the contract slice (adds `UPSTREAM_AUDIT_ADDR=team-audit-svc:50066` to verification, domain, order, payment, promotion; engagement already has it).
- Proto copies: contract slice only; consumers regenerate and never edit them.
- `platform-e2e/tests/e2e/support/audit_support.py` (query events by target via QueryAuditLog as admin, plus a way to stop/start team-audit for the fail-closed scenarios): written by audit-query-filters only; producer slices import it.
- Each producer slice writes its own feature and step files (`platform-e2e/tests/e2e/features/<area>/*_audit.feature`).

## Out of initiative
- Stale lines in `team-audit/README.md` Known gaps ("No FEATURES.yaml", target_id filter) are fixed inside audit-query-filters as part of its docs task.
- Per-environment `UPSTREAM_AUDIT_ADDR` values beyond local compose (gitops) — human release item, as in Q2.

## NFR and data (initiative level; each slice's proposal.md refines them)
- NFR:
  - Added latency on admin paths bounded by the producer's upstream timeout (≤ 2 s), no retry.
  - QueryAuditLog p95 < 300 ms for a filtered query on 100k rows (non-gating).
  - Owner (non-admin) paths: no added call, no added latency.
- Personal data: audit rows hold the admin's user id, target ids, and for `kyc.review` the subject's `user_id`; no names, documents, addresses or free text beyond existing reason fields. Purpose: accountability of privileged actions (Decree 13/2023 processing records). Basis: legal obligation / legitimate interest. Retention: team-audit policy (immutable; erasure out of scope).
- Risk routing expected: auth (scope-dependent audit, service principals with `audit.write`), payment (admin refund path), pii-light (user_id in KYC audit metadata), proto (contract slice).

## Conventions
- Producer audit client: `team-<x>/internal/upstream/audit.go` mirroring `team-engagement/internal/upstream/audit.go`; a small `AuditWriter` interface injected into the service/handler so unit tests use a fake; `cmd/server/main.go` wires it only when the client is non-nil.
- Handler tests: mirror each repo's existing authz tests; table cases for admin path (audited), owner path (not audited), audit down (Unavailable, no change).
- E2E: each producer scenario asserts the event through `audit_support.py` (QueryAuditLog by `target_type` + `target_id` as admin), and the fail-closed scenario stops team-audit and asserts the target unchanged.
- Frontend: `team-frontend/src/app/(shop)/admin/audit/page.tsx`, gateway client `src/lib/gateway/audit.ts`, Vietnamese UI copy; mirror `admin/cockpit/page.tsx` gate.
- Do not touch: team-identity (scopes are self-asserted by service principals), team-ai, team-referral, team-analytics beyond its proto copy.

## Locked
2026-10-15 — Confirmed by Bùi Đăng Phúc. Admin-path mutations (12-action catalogue) write a team-audit event first, fail closed (Unavailable, no change); owner paths unaffected; per-producer service principal with audit.write; QueryAuditLog gains target_id/action/time filters; /admin/audit viewer; AGENTS.md rule + auth-scope-reviewer check; stacked on Q2; no outbox, no idempotency key, seller/system money flows deferred.
