## Why

A code-reading audit of the thinly-guarded services (`team-promotion`, `team-audit`, `team-engagement`, `team-verification`,
`team-sharing`) found that several RPCs that move money-adjacent state or expose privileged data check **no principal, or only
a scope that every signed-in user holds**. The public gateway forwards almost all of them and its auth interceptor never
rejects (anonymous callers get the public scopes), so the exposure is reachable from the internet, not only from a pod that
can reach a service port (ADR-0010). Every finding below was re-read in code at the tip of each repo before being written
here; corrections to the audit are listed in `design.md` ("Corrections to the audit"). Nothing was run against a live stack.

1. **Voucher saga RPCs are open to anyone (CRITICAL).** `team-promotion` `ValidateAndReserve`, `CommitReservation` and
   `ReleaseReservation` (`internal/handler/voucher.go:99/115/126`) have no principal check, and `team-gateway`
   (`internal/edge/promotion.go:72-104`) exposes all three. Quota is consumed only by `CommitReservation`
   (`service/voucher.go:260-281`, `IncrementUsed`), which has no caller check and does not look at the buyer, so any caller
   can place a hold under an id of their choosing and commit it to burn a voucher's quota. `ValidateAndReserve` also trusts
   the request `buyer_id`/`seller_id`/`cart_subtotal`. These are saga RPCs whose only legitimate caller is `team-order`.
2. **Any buyer can mint platform-wide vouchers and flash sales (HIGH).** `CreateVoucher` with scope `PLATFORM`
   (`voucher.go:33-46`) and `CreateCampaign` (`flashsale.go:33-47`, any `listing_id`) need only a non-anonymous principal.
3. **The audit log is forgeable and world-readable (CRITICAL).** `team-audit` has no interceptor and no principal anywhere
   (`internal/grpcserver/server.go:22` builds a bare `grpc.NewServer()`); `WriteAuditEvent` stores the request `actor_id`
   (`handler/audit.go:41`) and `QueryAuditLog` (`:51`) returns the whole log; `team-gateway` exposes both
   (`internal/edge/audit.go`).
4. **Anyone can resolve any dispute; dispute reads are an IDOR; disputes and answers are unverified (HIGH/MEDIUM).**
   `ResolveDispute` needs `engagement:write`, which buyer, seller and admin all hold, and never looks at the caller
   (`handler/engagement.go:518-556`, `service/dispute.go:81-115`); the terminal states then lock. `GetDispute`
   (`:492-515`) needs only `engagement:read`. `CreateDispute` (`:459-489`, `service/dispute.go:35`) verifies neither that
   the order exists, nor that the caller is its buyer, nor that `defendant_id` is its seller. `AnswerQuestion`
   (`:395-422`, `service/qa.go:57`) has no ownership check and trusts the request's `is_shop_reply`, and the UI offers the
   answer form to every viewer (`team-frontend/src/features/listing/qa/QASection.tsx:112`, default `isShopReply = true`).
5. **Anyone can approve their own KYC and all anonymous users share one identity (HIGH/MEDIUM).** `ReviewKyc`
   (`team-verification` `handler/verification.go:59-67`) has no gate and is exposed
   (`team-gateway/internal/edge/verification.go:55`); `UserIDOrDemo` (`interceptor/principal.go:41-46`) maps "no principal"
   to the demo user `khach_hang_shopee` (handler lines 29 and 46); `GetVerificationStatus` honours any request `user_id`.
6. **Share links are anonymous and unbounded (LOW).** `CreateShareLink` is anonymous by design; this change records the
   decision instead of leaving it implicit.
7. **A false-positive e2e scenario hides #4.** `platform-e2e/tests/e2e/step_definitions/group_b_steps.py:81-94` registers a
   user with role `"admin"`; identity silently downgrades that to buyer (`NormalizeRole`), and the step asserts only
   `res is not None`. It passes *because* `ResolveDispute` has no admin gate.

## What Changes

Item numbers (#1..#7) are the numbers above, used in `design.md` and `tasks.md`.

- **team-promotion**
  - #1 `CommitReservation` and `ReleaseReservation` require a **service** principal holding the new service-only scope
    `promotion.reserve`. `ValidateAndReserve` accepts (a) a service principal with `promotion.reserve` (team-order: request
    fields trusted) or (b) an authenticated **user**: `buyer_id` is bound from the principal (request value ignored) and
    `reservation_id` must be in the caller's own preview namespace (`preview:<principal id>:`), which is exactly what the
    checkout preview in the frontend already sends. Anonymous is `UNAUTHENTICATED`.
  - #2 `CreateVoucher` requires `listing.write` (seller/admin); scope `PLATFORM` additionally requires `admin`.
    `CreateCampaign` requires `listing.write` and, unless admin, that the caller owns the listing (checked against
    `team-domain` `GetListing` as `service-team-promotion`).
  - `ListVouchers`/`GetVoucher` stay public (they back the public `/vouchers` hub); audit finding #3 is **not** a defect
    (design D6), recorded as an open question.
- **team-order**: the promotion client sends `service-team-order` / `service` / `promotion.reserve` on
  `ValidateAndReserve`, `CommitReservation` and `ReleaseReservation` (the existing `AsService` pattern used for
  `inventory.write`), in the buyer request, the `PaymentSettled` consumer and compensation alike.
- **team-audit**: add a principal interceptor; `WriteAuditEvent` requires a service principal with the new service-only
  scope `audit.write`; `QueryAuditLog` requires `admin`.
- **team-engagement**: `ResolveDispute` requires `admin`; `GetDispute` only the claimant, the defendant or an admin;
  `CreateDispute` verifies through the existing upstream order client (`service-team-engagement`, `order.read`) that the
  order exists, the caller is its buyer and `defendant_id` is its seller; `AnswerQuestion` derives `is_shop_reply` from
  whether the caller owns the question's listing (checked against `team-domain`) and ignores the request value (non-owners
  may still answer, flagged as non-shop, so the UI keeps working).
- **team-verification**: a real principal interceptor; `SubmitKyc`/own-status need an authenticated caller; `ReviewKyc`
  requires `admin`; `GetVerificationStatus` for another user's id requires `admin`; the demo-user fallback is deleted.
- **team-sharing**: no behaviour change; `CreateShareLink` stays anonymous (decision D11). Stale comments/manifest updated.
- **team-gateway**: a small edge **procedure policy** (`admin` required for `ReviewKyc`, `ResolveDispute`,
  `QueryAuditLog`; anonymous `401`, non-admin `403`, upstream never called) as defence in depth, following the
  `RequireScope("admin")` precedent of the cockpit route; and **removal** of the routes that have no legitimate external
  caller: `VoucherService/CommitReservation`, `VoucherService/ReleaseReservation`, `AuditService/WriteAuditEvent`
  (they answer `unimplemented`, like the stock RPCs). `ValidateAndReserve` stays routed (checkout preview).
- **team-identity**: no runtime change. The scope coverage test declares `admin` as an enforced scope and
  `promotion.reserve`/`audit.write` as explicit **service-only** scopes, with the negative test iterating that list.
  **No new user scope is introduced, so no token-lag window exists.**
- **platform-core**: comment-only proto edits (required scope per RPC); ADR-0003 and ADR-0010 amendments. **No message or
  RPC shape change** (`buf breaking` must stay clean).
- **platform-e2e** and the owning repos' `FEATURES.yaml`: API-level scenarios for every rule above, **and the fix of the
  admin-dispute false positive** (seeded `admin`/`admin123` login plus a buyer-gets-403 negative case).

**BREAKING (intended, security):** anonymous and non-admin callers lose access to the RPCs above. Consumers affected:
`platform-e2e` (audit write/query round trip, admin-dispute step, dispute `defendant_id`), nothing in `team-frontend`
(caller audit in `design.md`). Stacking and rollout order (services that send principals deploy before the enforcing side):
`design.md` "Migration Plan".

## Capabilities

### New Capabilities

- `promotion-access-control`: who may call the voucher saga RPCs, how `ValidateAndReserve` binds identity, who may create
  vouchers and flash sales, and that `team-order` calls promotion as itself (`team-promotion`, `team-order`).
- `audit-access-control`: service-only audit writes, admin-only audit reads (`team-audit`).
- `dispute-and-qa-access`: admin-only dispute resolution, party-only dispute reads, order-verified dispute creation,
  ownership-derived shop replies (`team-engagement`).
- `verification-access-control`: admin-only KYC review, authenticated submit/status, no shared fallback identity
  (`team-verification`).
- `edge-route-policy`: edge admin gate for admin-only RPCs and the set of internal-only RPCs that are not routed
  (`team-gateway`).
- `share-link-creation`: the recorded decision that share-link creation stays anonymous and edge-rate-limited
  (`team-sharing`, `team-gateway`).

### Modified Capabilities

- `auth`: adds requirements that service-only scopes are declared and granted to no user role (now
  `inventory.write`, `order.read`, `promotion.reserve`, `audit.write`) and that admin-only RPCs use the `admin` scope
  (existing capability in `openspec/specs/auth`; ADDED requirements only, no existing requirement changes).

## Impact

- Repos: `team-promotion`, `team-order`, `team-audit`, `team-engagement`, `team-verification`, `team-gateway`,
  `team-identity` (tests only), `team-sharing` (docs only), `platform-core` (ADRs + proto comments), `platform-e2e`, and
  `platform-gitops`/compose (`UPSTREAM_DOMAIN_ADDR` for `team-promotion` and `team-engagement`). `team-frontend`,
  `team-domain`, `team-search`, `team-ai`, `team-payment` do **not** change.
- No database migration. No new user-facing scope. No proto message/RPC shape change.
- New runtime dependency edges: `team-promotion -> team-domain` (`GetListing`, service principal `listing.read`) and
  `team-engagement -> team-domain` (`GetListing`, same); both are read-only gRPC calls (Rule 3 respected).

## Non-goals

- Rate limiting and abuse controls beyond what is listed (the edge limiter already exists; no new limiter).
- mTLS / service-identity zero-trust (ADR-0010 follow-up): service principals here are still unauthenticated metadata
  behind the gateway and the NetworkPolicy; this change narrows what a *forged or legitimate* principal can do, it does
  not authenticate the caller.
- Frontend changes (none forced; every legitimate UI call keeps working, see `design.md` caller audit).
- Wiring audit-event **producers** (nothing in the stack calls `WriteAuditEvent` today; this change only makes the
  endpoint safe to use).
- Changing voucher/dispute business rules, refunding a committed voucher, per-buyer voucher caps, dispute workflow
  states, or making preview a no-hold call (a `PreviewVoucher` RPC is recorded as a follow-up).
- The other `team-promotion` services (subscriptions, sponsored slots, ad campaigns) and other engagement RPCs.
