## Why

A new buyer gets a new-user voucher (slice `promotion-new-user-vouchers`) but nothing in the storefront shows it. The
`/vouchers` page lists only platform and shop vouchers, the checkout picker cannot offer a user-bound voucher, and there
is no prompt toward the three actions that activate an account (follow a shop, add an address, place a first order).
Backlog Q7 and Q37 ask for a visible welcome voucher and an activation checklist.

initiative: q7-q37-referral-welcome

## What Changes

- `/vouchers` renders a "Voucher của bạn" section from `VoucherService.ListMyVouchers` above the platform list, for
  signed-in buyers only.
- The checkout voucher picker (`VoucherModal`, platform slot) also offers the buyer's own vouchers.
- A `WelcomePanel` renders on `/` and on every `/account/*` page for accounts under 14 days old. It holds a dismissible
  banner (voucher code, amount, expiry; "Voucher đang được chuẩn bị" while none exists) and a 3-step checklist ("Theo
  dõi một shop", "Thêm địa chỉ giao hàng", "Đặt đơn đầu tiên") ticked from `EngagementService.ListFollowedSellers`,
  `AddressService.ListAddresses` and `OrderService.ListBuyerOrders`.
- Banner and checklist dismissal are remembered in first-party cookies written by a server action.
- Account age comes from `AuthService.GetMyAccount` (`{user_id, created_at}`, provided by the contract and identity slices,
  clarification C1). No session-based proxy.
- "Đặt đơn đầu tiên" ticks on any order that is not cancelled (clarification C6).
- No new RPC, event or proto change in this slice. New gateway helper `listMyVouchers()` in `src/lib/gateway/promotion.ts`.

## Capabilities

### New Capabilities
- `frontend-welcome-checklist`: own-voucher listing, checkout picker offer, welcome banner, activation checklist.

### Modified Capabilities
none (deltas are additive; `ui-discovery` and `ui-cart-checkout` are not edited).

## Impact

- `team-frontend/src/lib/gateway/promotion.ts` (+`listMyVouchers`, `ViewVoucher` gains `kind`, `ownerUserId`),
  `src/lib/gateway/auth.ts` (+`getMyAccount`), `src/features/voucher/`, `src/features/order/VoucherModal.tsx`,
  `src/app/(checkout)/checkout/(summary)/page.tsx`, `src/app/(shop)/vouchers/page.tsx` (hot file shared with
  `frontend-referral-invite`: rebase on it, keep its invite entry intact), `src/app/(shop)/(home)/page.tsx`,
  `src/features/account/AccountShell.tsx`, new `src/features/welcome/`.
- Depends on regenerated `src/generated` containing `ListMyVouchers` and `Voucher.owner_user_id/kind`.

## Non-goals

- Any backend change, a new "account created" field or RPC, or a welcome-state service.
- Admin screens, notifications for the voucher, a `/account` index route (none exists today).
- Showing the voucher to anonymous visitors or sellers' shop vouchers management (unchanged).

## Non-functional requirements

- Each panel read and the checkout picker read have a 2 s timeout (`AbortSignal.timeout(2000)`); a read exceeding it or
  failing degrades only its own part, silently. The three checklist reads and the voucher read run in parallel, so panel latency is the slowest single read.
- `/` streams the panel in its own Suspense boundary (fallback null) so a slow read never delays the hero or the feed.
- Signed-out visitors cost zero extra gateway calls.
- Dismissal cookies: `httpOnly`, `sameSite: lax`, `path: /`, `maxAge` 1209600 s (14 days), `secure` equal to the
  `session` cookie, value `1` only.
- Voucher pending text is shown only while the account is under 1 hour old (the referral verdict fallback is 30 min, C2).

## Data classification

| Field | Purpose | Consent basis | Retention |
|---|---|---|---|
| `agora_welcome_dismissed`, `agora_checklist_dismissed` cookies (value `1`) | remember the buyer closed the banner or checklist | functional, no consent banner needed | 14 days, then expire |
| Voucher code, amount, expiry shown in the banner | tell the buyer their entitlement | contract (it is the buyer's own voucher) | not stored by the frontend; rendered per request |
| Account `created_at` from `GetMyAccount` | decide whether the panel shows | contract | not stored, not logged, not sent to analytics |

No personal value is written to logs or analytics; the dismissal and checklist events, if tracked later, carry no ids.

## Threat model

- Assets: a buyer's own voucher codes; the signed-in session.
- Abuse cases: reading another user's vouchers (the frontend sends no user id; `ListMyVouchers` takes the principal from
  the bearer, enforced by the gateway and team-promotion); forging a dismissal cookie (only hides UI, no authority gained);
  showing the panel to an old account by clearing cookies (harmless, the vouchers are server-enforced).
- Authz rule: every call carries the buyer's bearer; anonymous visitors never trigger `ListMyVouchers`, `GetMyAccount`,
  `ListFollowedSellers`, `ListAddresses` or `ListBuyerOrders`. Voucher eligibility stays enforced by
  `ValidateAndReserve`; the picker offering a voucher is not a grant.

## Open issues

- Contract dependency: `VoucherService.ListMyVouchers`, `Voucher.owner_user_id/kind` and `AuthService.GetMyAccount` must be in the regenerated `src/generated` before the code track starts.
- `/account` has no route; the panel is mounted in `AccountShell` (accepted), so it shows on every `/account/*` page.
- `src/app/(shop)/vouchers/page.tsx` is shared with `frontend-referral-invite`; rebase on it.
