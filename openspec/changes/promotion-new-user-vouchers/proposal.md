## Why

Every new account should hold exactly one new-user voucher ("Bạn mới" welcome, or the larger "Bạn được mời" when invited), and any voucher should be able to carry a per-user limit and a new-customers-only rule. Today the `vouchers` table has no owner or per-user columns, `ValidateAndReserve` checks only active, dates, min spend, global quota and shop match, and team-promotion has no Kafka consumers (it only produces to `promotion.events` through the outbox). The buyer also has no way to list the vouchers bound to them.

initiative: q7-q37-referral-welcome

## What changes

- Migration `0008_user_vouchers`: `vouchers` gains `owner_user_id`, `kind` (1 VOUCHER_KIND_STANDARD default, 2 VOUCHER_KIND_NEW_USER), `per_user_limit` (0 = off), `new_user_only`; partial `UNIQUE (owner_user_id, kind) WHERE kind = 2`; `CHECK (kind <> 2 OR owner_user_id <> '')`; index on `owner_user_id`. `voucher_reservations` gains `expires_at` (existing `reserved` rows backfilled to `created_at` + 15m; new holds get now + `VOUCHER_RESERVATION_TTL`, default 15m, the saga reservation TTL of ADR-0008 `RESERVATION_TTL`). New tables `promotion_processed_events` (consumer idempotency) and `new_user_voucher_state` (per user: registered, has_code, verdict, issued).
- First Kafka consumers in team-promotion (go-sdk `kafkax`): `identity.events` (`UserRegistered`) and `referral.events` (`ReferralAttributed`, `ReferralRejected`), group `team-promotion.new-user`, idempotent on event id, poison records to `identity.events.promotion.dlq` and `referral.events.promotion.dlq`.
- Issue rule (wait-for-verdict): empty `referral_code` issues "Bạn mới" at once; non-empty waits for the verdict. `ReferralAttributed` issues "Bạn được mời", `ReferralRejected` issues "Bạn mới". A verdict that arrives before `UserRegistered` is stored and used when `UserRegistered` arrives. Fallback (clarification C2): a sweeper (`NEW_USER_VERDICT_SWEEP_INTERVAL`, default 1m) issues "Bạn mới" for each pending state row older than `NEW_USER_VERDICT_TIMEOUT` (default 30m); a later `ReferralAttributed` does not upgrade it.
- Issued voucher: PLATFORM scope, FIXED discount, quota 1, `per_user_limit` 1, `starts_at` = issue time, `ends_at` = issue time + `NEW_USER_VOUCHER_TTL` (336h). Env: `NEW_USER_WELCOME_AMOUNT` 20000, `NEW_USER_INVITED_AMOUNT` 30000, `NEW_USER_MIN_SPEND` 100000.
- `ValidateAndReserve` adds, after the existing checks: owner check, `per_user_limit` (counted from `voucher_reservations`: reserved and unexpired, plus committed, for the buyer), `new_user_only` via `OrderService.HasCompletedOrder` (fails closed). Refusals use the existing shape (clarification C3): `valid=false` with reason `VOUCHER_NOT_ELIGIBLE` or `VOUCHER_PER_USER_LIMIT`, so the checkout saga is unchanged. Only an unavailable `HasCompletedOrder` answers gRPC `UNAVAILABLE` (fail closed).
- `CreateVoucher` (seller and admin) and `UpdateVoucher` (admin, authz unchanged) accept `per_user_limit` and `new_user_only`. `CreateVoucher` can never set owner or kind.
- `ListVouchers` excludes user-bound vouchers; `GetVoucher` answers NOT_FOUND for a user-bound voucher to anyone but the owner.
- New RPC `VoucherService.ListMyVouchers` (principal = buyer), routed at team-gateway for any signed-in principal. `OrderService.HasCompletedOrder` stays internal: the gateway must not route it.
- Deploy wiring: docker-compose (`team-promotion` env, `UPSTREAM_ORDER_ADDR`, kafka DLQ topics), `platform-gitops/envs/services/team-promotion.yaml` (env, `networkPolicy` of team-order allows team-promotion) and the team-promotion service credential: its service token (the secret `secrets.enabled` already mounts, and the local compose service-token set) is issued with scope `order.read`; deploy fails closed (UNAVAILABLE or PERMISSION_DENIED from order maps to UNAVAILABLE) when the scope is missing.

## Capabilities

### New Capabilities
- `new-user-vouchers`: event-driven issue of the single VOUCHER_KIND_NEW_USER voucher per account.
- `voucher-per-user-limits`: owner, per-user limit, new-customer-only enforcement and the admin/seller fields.
- `my-vouchers`: `ListMyVouchers`, exclusion of user-bound vouchers from public reads, gateway routing.

### Modified Capabilities
None. Existing `promo-vouchers`, `voucher-scope` and `promotion-admin-vouchers` main specs keep their requirements; the new checks only add refusals for vouchers that carry the new columns, and vouchers created before migration 0008 get `owner_user_id = ''`, `kind = 1`, `per_user_limit = 0`, `new_user_only = false` (no behaviour change).

## Impact

`team-promotion` (`internal/store/migrations/0008_*`, `internal/store`, `internal/voucher/voucher.go`, `internal/rpc/voucher.go`, `internal/config`, `internal/app`, new `internal/consumer`, new `internal/upstream/order.go`), `team-gateway` (`internal/edge/promotion.go`, route table, not `policy.go` admin map), `docker-compose.services.yaml`, `platform-gitops/envs/services/team-promotion.yaml` and team-order network policy. Vendored proto comes from `q7-referral-welcome-contract`. PR #97 changes promotion admin scope gating: this slice does not touch the admin screens or the `promotion.admin` gating and rebases on it before build.

## Non-goals

Admin screens for new-user voucher templates (values come from env); a UI (slice `frontend-welcome-checklist`); changing who may call `UpdateVoucher`; revoking an issued VOUCHER_KIND_NEW_USER voucher when a referral is later voided; notification of the issued voucher; spending or crediting xu.

## NFR

- Voucher issue latency: the VOUCHER_KIND_NEW_USER voucher exists within 10 s of `UserRegistered` (empty code) or of the verdict on the local stack (p95 < 3 s consumer lag). With no verdict it exists within `NEW_USER_VERDICT_TIMEOUT` + 2 x sweep interval.
- `ValidateAndReserve` p95 added latency without `new_user_only`: < 10 ms (two indexed queries). With `new_user_only`: plus one `HasCompletedOrder` call with a 500 ms timeout (`UPSTREAM_ORDER_TIMEOUT`).
- `ListMyVouchers` p95 < 100 ms, at most 50 items.
- Consumers are idempotent on event id; a poison record is sent to the DLQ after 3 attempts and does not block the partition.
- Availability: signup never depends on promotion (async); checkout with a non-`new_user_only` voucher does not depend on team-order.
- Limits: `per_user_limit` 0..1000; `ListMyVouchers` caps at 50.

## Data classification

- `owner_user_id` (user id, personal): purpose eligibility of a user-bound voucher; basis contract (a promised welcome benefit); retention until voucher expiry plus the existing order retention policy (no new period is introduced); never written to `promotion.events` for VOUCHER_KIND_NEW_USER vouchers and never returned to anyone but the owner.
- `new_user_voucher_state` (`user_id`, `registered`, `has_code` boolean, `verdict`): purpose wait-for-verdict; basis contract; the referral code itself is never stored here; rows purged 30 days after the voucher is issued.
- `promotion_processed_events` (event id only): purpose idempotency; purged after 30 days.
- Event payload ids (`user_id`, `referee_id`, `referrer_id`) are read, not republished. DPO sign-off stays NEEDS_HUMAN (tracked by the initiative).
- Logs carry `principal_fp` / `key_fp` only, never raw user ids, voucher codes or referral codes (sloglint forbidden keys).

## Threat model

- Assets: voucher value (20,000 to 30,000 đ per account), owner binding, per-user counters, eligibility answers.
- Abuse cases: (1) buyer B redeems A's VOUCHER_KIND_NEW_USER voucher by guessing the code -> refused `VOUCHER_NOT_ELIGIBLE`, and `GetVoucher` answers NOT_FOUND; (2) double issue by redelivery or by verdict-after-issue race -> UNIQUE `(owner_user_id, kind)` plus `ON CONFLICT DO NOTHING`; (3) parallel checkouts exceed `per_user_limit` -> per (voucher, buyer) advisory lock held across count and insert; (4) a seller sets `owner_user_id` or `kind` through `CreateVoucher` -> fields do not exist on the request, the repository always writes `VOUCHER_KIND_STANDARD` and empty owner; (5) buyer bypasses `new_user_only` when team-order is down or refuses the service token -> fail closed `UNAVAILABLE`, no reservation; (5b) a user-token caller reaches `HasCompletedOrder` -> order requires a SERVICE principal with `order.read`; (6) a forged event on the topic -> consumers only read from the broker with service credentials; no RPC accepts a user id for issue; (7) a caller enumerates other users' vouchers -> `ListMyVouchers` takes no user id, the owner is the principal; (8) calling `HasCompletedOrder` from the internet -> not routed at the gateway, network policy allows only team-promotion.
- Authz rule: `ListMyVouchers` needs any authenticated buyer principal (no scope); `CreateVoucher` / `UpdateVoucher` / `ListVouchers` / `ValidateAndReserve` keep their current gates; eligibility is decided from the principal or service-supplied `buyer_id`, never from a request field of a user principal.
