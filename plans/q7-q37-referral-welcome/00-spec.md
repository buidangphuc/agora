# 00-spec — Referral done right + new-user welcome package

initiative: q7-q37-referral-welcome

Source: backlog Q7 and Q37 (`plans/pm-review-2026-10/BACKLOG.md` on branch `docs/pm-review-backlog`), evidence `growth-retention.md` #1, #2, #10 and `trust-ops.md` #6 and quick win 4.
Base: `origin/main` at b126fcd7 (Q1 login return, Q4 loop events, Q10 order lifecycle with `OrderCompleted.return_window_ends_at`, P10 go-sdk v1.0.3 merged). The promotion admin screens are also changed by PR #97 (`fix/open-gaps-wording`); rebase on it before building the promotion slice.

## Goal
A buyer can share an invite link from `/account/referral`. A friend who opens it and signs up is attributed to the inviter without typing a code. Every new account gets exactly one new-user voucher: an invited account gets the larger "Bạn được mời" voucher, and everyone else gets the "Bạn mới" welcome voucher. After signup the buyer sees a banner with the voucher and a 3-step checklist: follow a shop, add an address, place a first order. The inviter's 10,000 xu reward is created as pending when the invitee's first order completes, and released only once that order's return window ends with no return. Rewards are capped per inviter per day and over the inviter's lifetime, and are shown as xu, not đ.

## User stories
1. **Invite link carries attribution.**
   - Given buyer A on `/account/referral`.
   - When A taps "Chia sẻ link mời", the page creates a share link `createShareLink("referral", <A's code>, {utm_source: "referral", utm_medium: "invite"})` and copies `/s/<shortCode>`.
   - Then a visitor opening `/s/<shortCode>` is redirected to `/register?ref=<code>` with the stored `utm_*` kept on the URL, and a first-party `agora_ref` cookie (code only, 7 days, `SameSite=Lax`, HttpOnly) is set.
   - Edge cases:
     - Non-referral share links (listing, shop) also keep their `utm_*` on the redirect; they set no `agora_ref` cookie.
     - An unknown short code still redirects to `/` with no cookie.
2. **Signup attributes the referral asynchronously.**
   - Given visitor B with `?ref=<code>` or an `agora_ref` cookie.
   - When B registers, the register form sends `referral_code` and team-identity stores the user and writes a `UserRegistered{user_id, referral_code, registered_at}` outbox row in the same transaction.
   - Then team-referral consumes `UserRegistered`. A valid code owned by another user creates the referral row (status `ATTRIBUTED`) and emits `ReferralAttributed`. An unknown code, self-referral or inviter over the lifetime cap emits `ReferralRejected{reason}`. Registration never fails because of the referral code.
   - Edge cases: redelivery of the same `UserRegistered` creates nothing twice (referee_id stays UNIQUE, event ids are stable).
3. **Manual code entry stays for 7 days.**
   - Given buyer B registered without a code, whose account is under 7 days old (`REFERRAL_REDEEM_MAX_ACCOUNT_AGE`, default 168h) and who has no completed order.
   - When B enters a friend's code on `/account/referral`.
   - Then the referral is attributed exactly as in story 2, but B gets no extra voucher (B already holds the welcome voucher).
   - Edge cases: an older account, an account with a completed order, a second redeem or a self-referral is refused with `FAILED_PRECONDITION` and a Vietnamese message. Redeeming no longer credits anything immediately.
4. **One new-user voucher per account.**
   - Given a new account.
   - When `UserRegistered` arrives with an empty `referral_code`, team-promotion issues the "Bạn mới" voucher bound to that user. When the code is non-empty, team-promotion waits for the referral verdict: `ReferralAttributed` → the "Bạn được mời" voucher, `ReferralRejected` → the "Bạn mới" voucher.
   - Then the user holds exactly one voucher of kind `NEW_USER`: platform-wide, single use, expiring 14 days after issue. Defaults (env): welcome 20,000đ off with min spend 100,000đ; invited 30,000đ off with min spend 100,000đ.
   - Edge cases:
     - Redelivered events, or a verdict arriving after a voucher already exists, issue nothing more (UNIQUE `(owner_user_id, kind)`).
     - A user-bound voucher is refused at `ValidateAndReserve` for any other buyer (`FAILED_PRECONDITION`, `VOUCHER_NOT_ELIGIBLE`) and after its single use.
5. **Per-user limits on all vouchers.**
   - Given any voucher with `per_user_limit = N > 0`.
   - When a buyer already has N committed or live reserved uses.
   - Then `ValidateAndReserve` refuses with `VOUCHER_PER_USER_LIMIT`. With `new_user_only = true`, a buyer whose account has a completed order is refused with `VOUCHER_NOT_ELIGIBLE`. `per_user_limit = 0` keeps today's behaviour (global quota only).
6. **The buyer sees their own vouchers.**
   - Given a signed-in buyer.
   - When they open `/vouchers` or reach checkout.
   - Then a "Voucher của bạn" section lists their own active user-bound vouchers (`ListMyVouchers`) above the platform list. The checkout voucher picker offers them too.
7. **Welcome banner and checklist.**
   - Given a buyer whose account is under 14 days old.
   - When they land on `/` or `/account`.
   - Then a dismissible banner shows their new-user voucher (code, amount, expiry). A 3-step checklist shows "Theo dõi một shop", "Thêm địa chỉ giao hàng" and "Đặt đơn đầu tiên", each ticked from existing reads: `ListFollowedSellers`, `ListAddresses` and `ListBuyerOrders`. The checklist hides itself when all three are done or the user dismisses it; dismissal is remembered in a first-party cookie.
   - Edge cases: when any read fails, that step shows as not done and the rest still render. The voucher may not exist yet because issue is async: the banner then shows "Voucher đang được chuẩn bị" and no code.
8. **Inviter reward: pending, then released.**
   - Given A invited B, and B's referral is `ATTRIBUTED`.
   - When B's first order emits `OrderCompleted` on `order.events`, team-referral writes a `PENDING` reward of 10,000 xu (`REFERRAL_REWARD_XU`) for A. The reward is linked to that order, with `release_at = return_window_ends_at`, and the referral moves to `QUALIFIED` in the same transaction.
   - Then a sweeper releases each due `PENDING` reward (status `RELEASED`) and emits `ReferralRewardReleased`. `/account/referral` shows pending and released totals as "xu", never with `formatPrice`/đ.
   - Edge cases:
     - `ReturnRequested` for the linked order before release sets the reward to `VOID` and the referral back to `ATTRIBUTED`, so B's next `OrderCompleted` can qualify again.
     - A's released rewards are capped at 5 per day (`REFERRAL_DAILY_CAP`, counted on release day in Asia/Ho_Chi_Minh) and 50 over A's lifetime (`REFERRAL_LIFETIME_CAP`). A due reward over the daily cap stays `PENDING` until the next day. Once the lifetime cap is reached, new pending rewards are not created, and the referral is marked `CAPPED`.
     - `OrderCompleted` for a buyer with no attributed referral is ignored. Redelivery creates nothing twice (UNIQUE on `order_id`).

## Non-goals
- Phone OTP, device or IP fingerprints, risk scores (trust #6, later items).
- Unifying the referral xu ledger with check-in coins, or spending xu (Q30).
- Real money: xu stay a mock ledger (AGENTS.md §7).
- Admin screens for referral or for new-user voucher templates. Values come from env.
- Notifications and push for released rewards (Q36).
- A login throttle or lockout per account.

## Architecture decisions
- **D1 Async event chain, no sync cross-service calls on signup.** identity → `identity.events/UserRegistered` → referral and promotion; referral → `referral.events/{ReferralAttributed, ReferralRejected, ReferralRewardReleased}` → promotion. Why: Register must not depend on referral or promotion being up; AGENTS.md §3.5.
- **D2 team-referral adopts the go-sdk outbox and consumer** (as identity and order do). It gets a `referral_outbox_events` table and consumers for `identity.events` and `order.events`. Why: atomic referral row, reward row and event (Q7 cap/atomic requirement); no in-memory fallbacks (feedback rule).
- **D3 team-promotion gets its first Kafka consumers** (`identity.events`, `referral.events`) through go-sdk `kafkax` consumer with an idempotent handler table. Why: issue is event-driven.
- **D4 Reward state machine** `PENDING → RELEASED | VOID`; referral state `ATTRIBUTED → QUALIFIED → REWARDED | (VOID→)ATTRIBUTED | CAPPED`. Release uses `release_at` from `OrderCompleted.return_window_ends_at`, plus a sweeper (`REFERRAL_RELEASE_SWEEP_INTERVAL`, default 1m). No new team-order event. Why: order already emits the window end time; a consumer-side timer avoids changing team-order.
- **D5 User-bound vouchers live in the existing `vouchers` table.** New columns: `owner_user_id`, `kind` (`STANDARD`, `NEW_USER`), `per_user_limit`, `new_user_only`. Per-user usage is counted from `voucher_reservations` (reserved and not expired, or committed). Why: reuses the checkout saga and reservation path unchanged.
- **D6 `new_user_only` eligibility asks team-order** through a new `OrderService.HasCompletedOrder(buyer_id)` internal RPC, which is not routed at the gateway. Why: AGENTS.md §3.3 (call the owner's gRPC, never join DBs). It is called only when the voucher has `new_user_only`.
- **D7 Checklist is a frontend aggregate of three existing reads.** No new backend. Why: the signals exist; no event or state is needed.
- **D8 Invite link reuses team-sharing** with `target_type="referral"` (team-sharing accepts any non-empty type today). `/s/[code]` maps `referral` to `/register?ref=<target_id>`. Why: click counting and UTM storage already exist.

## Contracts
```proto
// platform/identity/v1/identity.proto
message RegisterRequest { string username = 1; string password = 2; string role = 3;
  string referral_code = 4; }               // optional; never fails Register
// identity.events, key = user_id, envelope type "platform.identity.v1.UserRegistered"
message UserRegistered { string user_id = 1; string referral_code = 2;
  google.protobuf.Timestamp registered_at = 3; }

// platform/referral/v1/referral.proto
// referral.events, key = referee user_id, envelope types "platform.referral.v1.<Msg>"
message ReferralAttributed { string referee_id = 1; string referrer_id = 2;
  google.protobuf.Timestamp attributed_at = 3; }
message ReferralRejected { string referee_id = 1; ReferralRejectReason reason = 2; }
enum ReferralRejectReason { REFERRAL_REJECT_REASON_UNSPECIFIED = 0; UNKNOWN_CODE = 1;
  SELF_REFERRAL = 2; REFERRER_CAPPED = 3; }
message ReferralRewardReleased { string reward_id = 1; string referrer_id = 2;
  string referee_id = 3; int64 amount_xu = 4; google.protobuf.Timestamp released_at = 5; }
enum ReferralRewardStatus { REFERRAL_REWARD_STATUS_UNSPECIFIED = 0; PENDING = 1; RELEASED = 2; VOID = 3; }
// ReferralReward gains: ReferralRewardStatus status; google.protobuf.Timestamp release_at;
// GetMyReferralResponse gains: int64 pending_xu; int64 released_xu (rewards_total kept = released_xu)

// platform/promotion/v1/promotion.proto
enum VoucherKind { VOUCHER_KIND_UNSPECIFIED = 0; STANDARD = 1; NEW_USER = 2; }
// Voucher gains: string owner_user_id; VoucherKind kind; int32 per_user_limit; bool new_user_only;
// CreateVoucherRequest / UpdateVoucherRequest gain per_user_limit, new_user_only
rpc ListMyVouchers(ListMyVouchersRequest) returns (ListMyVouchersResponse); // principal = buyer
message ListMyVouchersRequest {}
message ListMyVouchersResponse { repeated Voucher vouchers = 1; }

// platform/order/v1/order.proto — internal, not routed at the gateway
rpc HasCompletedOrder(HasCompletedOrderRequest) returns (HasCompletedOrderResponse);
message HasCompletedOrderRequest { string buyer_id = 1; }
message HasCompletedOrderResponse { bool has_completed = 1; }
```
Existing contracts consumed unchanged: `order.events/OrderCompleted`, `order.events/ReturnRequested`, `SharingService.CreateShareLink`, `SharingService.ResolveShareLink`, `EngagementService.ListFollowedSellers`, `AddressService.ListAddresses`, `OrderService.ListBuyerOrders`, `VoucherService.ValidateAndReserve`.

## Capabilities → slices
| capability (user-observable) | slice id | owner | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| contract | q7-referral-welcome-contract | platform-core | – | provides: all of Contracts | platform-core (+ regenerated stubs in consumers) |
| signup carries a referral code and announces the new user | identity-user-registered | team-identity | contract | provides: identity.events/UserRegistered | team-identity, team-gateway (Register field pass-through) |
| order answers "has this buyer completed an order" | order-has-completed-order | team-order | contract | provides: OrderService.HasCompletedOrder | team-order |
| referral attributed from signup, inviter reward pending → released, caps | referral-attribution-reward | team-referral | contract, identity-user-registered | uses: identity.events/UserRegistered, order.events/OrderCompleted, order.events/ReturnRequested; provides: referral.events/* | team-referral, platform-gitops, docker-compose |
| one new-user voucher per account, per-user limits, my vouchers | promotion-new-user-vouchers | team-promotion | contract, identity-user-registered, referral-attribution-reward, order-has-completed-order | uses: identity.events/UserRegistered, referral.events/ReferralAttributed, referral.events/ReferralRejected, OrderService.HasCompletedOrder; provides: VoucherService.ListMyVouchers | team-promotion, team-gateway, platform-gitops, docker-compose |
| invite link, attribution through /s and register, xu display | frontend-referral-invite | team-frontend | referral-attribution-reward | uses: SharingService.CreateShareLink, ReferralService.GetMyReferral | team-frontend |
| welcome banner, my vouchers, activation checklist | frontend-welcome-checklist | team-frontend | promotion-new-user-vouchers | uses: VoucherService.ListMyVouchers, EngagementService.ListFollowedSellers, AddressService.ListAddresses, OrderService.ListBuyerOrders | team-frontend |

Hot files edited by more than one slice (initiative-build serialises those slices):
- `team-gateway/internal/edge/server.go` / `policy.go` — identity-user-registered, promotion-new-user-vouchers.
- `docker-compose*.yml` and `platform-gitops/envs/services/*.yaml` — referral-attribution-reward, promotion-new-user-vouchers.
- `team-frontend/src/app/(shop)/account/referral/page.tsx` and `src/app/(shop)/vouchers/page.tsx` — the two frontend slices.
- `platform-e2e/tests/e2e/support/` helpers — every slice with e2e.

## Out of initiative
- Rebasing onto PR #97 (`promotion.admin` gating of the voucher admin screens) before building promotion-new-user-vouchers.
- `trackEcommerce("referral_redeem")` already fires on manual redeem. The event stays; there is no new analytics schema work.

## NFR and data
- NFR:
  - Register latency is unchanged (+1 outbox insert in the same transaction).
  - The new-user voucher appears within 10 s of signup on the local stack.
  - The reward sweeper releases within 2 × the sweep interval after `release_at`.
  - Consumers are idempotent on event id.
- Personal data:
  - `referral_code` in `UserRegistered`: purpose attribution, contract basis, kept with the referral row.
  - referrer/referee ids in `referral.events`: Kafka key = referee id. Purpose reward accounting, retention = topic retention.
  - `agora_ref` cookie (code only): functional, 7 days.
  - `owner_user_id` on vouchers: purpose eligibility, kept until voucher expiry + the order retention policy.
  - Logs use `principal_fp`/`key_fp` only, never raw ids or codes (sloglint forbidden keys).
  - DPO sign-off listed as NEEDS_HUMAN.
- Risk routing expected: `proto` (contract), `pii` (identity, referral, promotion events), `auth` (gateway routing of `ListMyVouchers`, internal `HasCompletedOrder` must not be routed). No `payment` (xu are a mock ledger).

## Conventions
- Go services use go-sdk v1.0.3 (`outbox`, `kafkax`, `logx`, `lifecycle`). Mirror team-identity `internal/store/outbox.go` + `internal/events/publisher.go` for the referral outbox. Mirror team-order consumers for idempotent handling.
- Migrations: identity `0022_*`, promotion `0008_*`, referral `002_*`. No DB enable flags or in-memory fallbacks.
- e2e in `platform-e2e/tests/e2e/`. Time-dependent checks use env overrides (`RETURN_WINDOW`, `REFERRAL_RELEASE_SWEEP_INTERVAL`) instead of sleeping for hours.
- Frontend copy in Vietnamese. Amounts in xu use a new `formatXu`, never `formatPrice`.
- The local stack is shared with another session: start only the services under test, and only when it is free.

## Locked
2026-11-06 — Confirmed by Bùi Đăng Phúc. Async event chain (UserRegistered → referral verdict → promotion), reward pending on first OrderCompleted and released after return window (void on ReturnRequested, next order qualifies), caps 5/day 50 lifetime, manual redeem within 7 days, one NEW_USER voucher per account (20k welcome / 30k invited, min 100k, 14 days), frontend-aggregate checklist, order adds internal HasCompletedOrder only, promotion gets its first Kafka consumers.

## Clarifications (2026-11-06, spec review round 1, answered by Bùi Đăng Phúc)
- C1 Account age: the contract adds `rpc GetMyAccount(GetMyAccountRequest) returns (GetMyAccountResponse)` to `AuthService` (principal-scoped, routed at the gateway for any signed-in user; response `{user_id, created_at}`). team-identity implements it; the frontend uses it for the 14-day rule. No session-based proxy.
- C2 Verdict fallback: if no referral verdict arrives within 30 min (`NEW_USER_VERDICT_TIMEOUT`, default 30m) after a `UserRegistered` with a non-empty code, promotion issues the "Bạn mới" voucher; a later `ReferralAttributed` does not upgrade it.
- C3 Refusal shape: `ValidateAndReserve` refuses not-eligible / over-per-user-limit vouchers with `valid=false` and reasons `VOUCHER_NOT_ELIGIBLE` / `VOUCHER_PER_USER_LIMIT` (same shape as today's refusals; checkout saga unchanged). Only an unavailable `HasCompletedOrder` answers gRPC `UNAVAILABLE` (fail closed).
- C4 Hold counting: promotion migration adds `expires_at` to `voucher_reservations` (= the saga reservation TTL); per-user usage = reserved-and-unexpired + committed.
- C5 Pre-existing accounts with no stored registration in referral are refused manual redeem (`FAILED_PRECONDITION`).
- C6 Checklist "first order" ticks on any non-cancelled order.
- Defaults accepted without a question: `?ref` beats the `agora_ref` cookie; the seeded admin gets no `UserRegistered`; `HasCompletedOrder` caller = SERVICE principal with `order.read`, any `completed_by` counts, logs carry no buyer id in any form; referral code bounds `^[A-Za-z0-9_-]{1,32}$` after trim, case kept (codes are `REF`+8 hex); NFR latency budgets = +10 ms p95 on Register, +10% p99 on referral RPCs.
