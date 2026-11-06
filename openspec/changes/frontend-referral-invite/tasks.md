## 1. Code track (team-frontend)

Depends on referral-attribution-reward (and the contract slice): regenerate `src/generated` from the proto with `RegisterRequest.referral_code`, `pending_xu`, `released_xu`, `ReferralRewardStatus`, `ReferralReward.status/release_at` first.

- [ ] 1.1 `formatXu` in `src/components/ui/format.ts` (+ unit test) (S17, S19)
- [ ] 1.2 `lib/gateway/referral.ts`: map `pendingXu`, `releasedXu`, reward `status` and `releaseAt` into the view types (S17, S18, S20)
- [ ] 1.3 Server action `createReferralInviteLinkAction` in `features/account/referral/actions.ts` (own code -> `createShareLink("referral", code, utm)`), `ShareInviteButton` client component with clipboard copy and toasts; wire into `account/referral/page.tsx` (S1, S2, S3, S4)
- [ ] 1.4 Replace `src/app/(shop)/s/[code]/page.tsx` with `route.ts`: utm_* kept, referral -> `/register?ref=`, `agora_ref` cookie (HttpOnly, Lax, 604800, Secure via `isSecureRequest`), code validation, fallbacks to `/` (S5-S8, S9a, S9b)
- [ ] 1.5 `/register` page passes `ref` to `RegisterForm` hidden field; `registerAction` resolves the referral code (field, then cookie, validated), sends `referral_code`, deletes `agora_ref` on success only, keeps `returnUrl` redirect (S10-S16)
- [ ] 1.6 Referral page: totals "Xu đang chờ" / "Xu đã nhận" with `formatXu`, reward timeline with status badges and VOID styling, remove `formatPrice` (S17-S21)
- [ ] 1.7 `RedeemReferralForm`: rule text, FAILED_PRECONDITION fallback message, success toast (S22, S23, S24)
- [ ] 1.8 Unit tests colocated: `s/[code]/route.test.ts` (S5-S8, S9a, S9b), `auth/actions.test.ts` (S10-S16), `referral/actions.test.ts` (S1, S3, S23, S24), `RedeemReferralForm.test.tsx` (S22-S25), referral page test (S2, S17-S20)

## 2. E2E track (platform-e2e)

- [ ] 2.1 e2e: buyer A copies the invite link; fetching `/s/<shortCode>` without following redirects returns 307 to `/register?ref=<code>&utm_...` and sets `agora_ref` with the attributes (S1, S5)
- [ ] 2.2 e2e: listing and shop share links keep utm and set no cookie; unknown code and unknown type redirect to `/` (S6, S7, S8, S9a, S9b)
- [ ] 2.3 e2e: visit invite link, register B; assert `agora_ref` cleared and B is attributed (inviter `invited_count` +1 once the consumer ran); register with a malformed `ref` still succeeds; failed register keeps cookie (S10-S15)
- [ ] 2.4 e2e: `/register?returnUrl=/cart&ref=<code>` lands on `/cart` (S16)
- [ ] 2.5 e2e: referral page shows xu totals and statuses (needs a PENDING and a VOID reward via the order/return flow with env-shortened `RETURN_WINDOW`); no đ on the page (S17, S18, S19, S21)
- [ ] 2.6 e2e: guest redirect to login (S4); redeem refused for an old or already-redeemed account shows the Vietnamese message, rule text visible (S22, S23, S24, S25); rewards read failure (S20) and share failure (S3) covered by unit tests only
