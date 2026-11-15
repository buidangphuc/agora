## 1. Code track (team-frontend-seller, team-frontend)

Depends on slices verification-kyc-documents and payment-payout-kyc-gate; regenerate proto stubs from the contract slice first.

- [ ] 1.1 Unit tests first (red): file validation helper (count, type, 0 byte, 5 MiB and 5 MiB + 1) and status/reason-code text map incl. unknown-code fallback (scenarios: A file over 5 MiB is rejected before upload, A file of exactly 5 MiB is accepted, A wrong file type or an empty file is rejected, More than three files are refused, An unknown reject reason code falls back to a generic text)
- [ ] 1.2 team-frontend-seller: `src/lib/gateway/verification.ts` (server-only: `getVerificationStatus`, `createKycUploadUrl`, `submitKyc`); component tests with mocked actions (scenarios: A seller who never submitted sees NOT_SUBMITTED and the form, A pending submission hides the form, A verified seller sees the verified state, A rejected submission shows the reason and allows resubmission, Status read failure is shown as an error, not as not-submitted)
- [ ] 1.3 `/seller/verification` page + `loading.tsx` with seller guard (`redirectGuestToLogin`, `hasScope("listing.write")`), principal id only (scenarios: A guest is sent to login, A buyer without seller scope cannot use the page)
- [ ] 1.4 Upload form client component and Server Actions: sequential presign + PUT + `SubmitKyc`, consent checkbox, double-submit guard, retry reusing uploaded refs, Vietnamese errors (scenarios: Three valid files are uploaded and submitted, Consent unticked blocks submission, A failed PUT stops the submission, The server refuses a doc_ref, Submit cannot be sent twice, Image bytes and keys never reach logs or the server action payload)
- [ ] 1.5 Nav entry in `SELLER_NAV` and nav tests (scenario: Nav lists the verification page)
- [ ] 1.6 Wallet: status notice, payout error mapping by gateway code in `requestWalletPayoutAction`/`PayoutButton` (scenarios: Wallet shows a notice while not verified, A refused payout shows the verification message, Verification outage during payout shows a retry message, A verified seller sees no notice and can pay out)
- [ ] 1.7 team-frontend: delete `/account/verification` route, `features/account/verification/*`, `AccountShell` item, `ConsumerShell` top-bar link (hot file, serialised), fix tests that list the route (scenarios: The buyer route is gone, Buyer navigation has no verification link)
- [ ] 1.8 team-frontend: `getPublicBadges` in `src/lib/gateway/verification.ts`, badge in `/shop/[id]` with failure fallback (scenarios: Verified seller's shop shows the badge, Non-verified shops show no badge and no status, Badge lookup failure does not break the page, The badge call asks for the shop seller only)
- [ ] 1.9 Gates through `scripts/asdlc/gate_digest.py` for team-frontend-seller and team-frontend

## 2. E2E track (platform-e2e; FEATURES.yaml by e2e-author)

- [x] 2.1 Feature files `seller/kyc_upload.feature`, `seller/kyc_payout_notice.feature` and `buyer/shop_verified_badge.feature` covering every scenario in `specs/seller-kyc-ui/spec.md`; replace each `VERIFIED BY: TBD` with path and scenario title
- [x] 2.2 Fixtures: small valid PNG/PDF, a 5 MiB and a 5 MiB + 1 byte file generated at run time (scenarios: A file of exactly 5 MiB is accepted, A file over 5 MiB is rejected before upload)
- [x] 2.3 Steps that approve/reject through the real `ReviewKyc` RPC as admin to reach `VERIFIED`/`REJECTED` (scenarios: A verified seller sees the verified state, A rejected submission shows the reason and allows resubmission, Verified seller's shop shows the badge); demo seed verifies sellers through real RPCs (shared with payment-payout-kyc-gate)
- [x] 2.4 Network-level steps for PUT failure and `GetPublicBadges` failure using a real fault toggle on the stack, no route fulfils (scenarios: A failed PUT stops the submission, Badge lookup failure does not break the page)
- [x] 2.5 `team-frontend-seller/FEATURES.yaml` and `team-frontend/FEATURES.yaml` entries (e2e-author)
