## 1. Code track (team-frontend)

Depends on promotion-new-user-vouchers and a regenerated `src/generated` containing `ListMyVouchers`.
`src/app/(shop)/vouchers/page.tsx` is shared with frontend-referral-invite: rebase on it and keep its invite entry.

- [ ] 1.1 Add `listMyVouchers()` (tagged `ok | failed`, anonymous short-circuit) and `kind`, `ownerUserId` on `ViewVoucher` in `src/lib/gateway/promotion.ts` (W1-W4, W7)
- [ ] 1.2 Add `MyVouchersSection` in `src/features/voucher/` and render it in `/vouchers` above `VouchersView` (W1-W5)
- [ ] 1.3 Fetch own vouchers in the checkout payment step and merge them into the platform slot of `VoucherModal`, deduplicated by code (W6-W8, W22, W27)
- [ ] 1.4 Add `src/features/welcome/accountAge.ts` (`getMyAccount` from `AuthService.GetMyAccount`, fail closed) (W12, W13, W20)
- [ ] 1.5 Add 2 s read timeouts and `isFirstOrderDone` (non-cancelled filter) (W16, W21, W25, W27)
- [ ] 1.5b Add `WelcomePanel` server component with banner, checklist, parallel reads and per-part degrade (W9-W11, W15-W18, W21, W24, W26)
- [ ] 1.6 Add server action `dismissWelcome(part)` setting the two cookies and revalidating, plus the close controls (W14, W19)
- [ ] 1.7 Mount `WelcomePanel` in `(home)/page.tsx` (own Suspense, fallback null) and in `AccountShell` (W9, W10, W15, W23)
- [ ] 1.8 Unit tests colocated: `promotion.test.ts`, `accountAge.test.ts`, `WelcomePanel.test.tsx`, `actions.test.ts`, `VoucherModal.test.tsx`, `vouchers/page.test.tsx` (W1-W20)

## 2. E2E track

- [ ] 2.1 e2e: register a fresh buyer, open `/` and assert the banner shows a voucher code and amount, or "Voucher đang được chuẩn bị" then the code within 10 s (W9, W10)
- [ ] 2.2 e2e: `/vouchers` shows "Voucher của bạn" for that buyer and not for a second buyer or anonymous (W1, W4, W5)
- [ ] 2.3 e2e: checkout payment step lists and applies the voucher; below minimum spend the quote marks it invalid (W6, W8)
- [ ] 2.4 e2e: follow a shop, add an address, place an order and watch the checklist tick then hide (W16, W17)
- [ ] 2.5 e2e: dismiss banner and checklist, reload `/` and `/account/addresses`, assert cookies and absence (W14, W19)
- [ ] 2.6 e2e: a seeded account created 30 days ago sees no panel (W12, W20); anonymous `/` shows none (W15)
- [ ] 2.7 e2e: fault injection for `ListMyVouchers`, `ListAddresses` and `GetMyAccount` outages and a 2 s stall (W3, W7, W11, W13, W18, W21, W25, W27)
