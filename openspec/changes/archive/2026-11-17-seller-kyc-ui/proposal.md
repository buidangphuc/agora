## Why

KYC today is a free-text "document reference" form in the buyer app (`team-frontend/src/features/account/verification/SubmitKycForm.tsx`,
route `/account/verification`), so a seller can "verify" by typing any string, and the only people who need KYC (sellers, whose payouts
will be gated by slice payment-payout-kyc-gate) never see it in their own app. The seller app has no verification page, no nav entry and no
explanation when a payout is refused. The shop page shows no verified badge.

initiative: trust-legal

## What changes

team-frontend-seller:
- New route `/seller/verification`: status panel (`NOT_SUBMITTED`, `PENDING`, `VERIFIED`, `REJECTED` with the reject reason shown in Vietnamese),
  and an upload form for 1-3 files (image/jpeg, image/png, application/pdf, each 1 byte to 5 MiB), a document type select, and a required
  explicit consent checkbox for processing the ID document. Flow: Server Action `CreateKycUploadUrl` per file, browser `PUT` straight to the
  presigned URL (content type and length bound), then Server Action `SubmitKyc` with `doc_ref` plus `additional_doc_refs`. The form is hidden
  while `PENDING` and `VERIFIED`; shown again after `REJECTED`.
- New server-only gateway module `src/lib/gateway/verification.ts` (mirrors the buyer module: per-request client, bearer from the session cookie).
- Nav entry "Xác minh danh tính" in `SELLER_NAV`.
- Wallet page (`/seller/wallet`): when status is not `VERIFIED`, a notice with a link to `/seller/verification`; a payout refused with
  `FAILED_PRECONDITION "seller verification required"` shows the same message and link instead of the raw error; `UNAVAILABLE` shows a
  retry message.

team-frontend (buyer app):
- Remove `/account/verification` (page, loading, form, actions, status helper, tests), the "Xác minh" item in `AccountShell` and the
  "Xác minh" link in `ConsumerShell` top bar (hot file shared with frontend-legal-pages and frontend-account-privacy).
- `/shop/[id]` shows a "Đã xác minh" badge when `GetPublicBadges` reports `verified` for the shop's seller; new gateway function
  `getPublicBadges` in `src/lib/gateway/verification.ts` (kept, the status helper is dropped). Only the boolean is shown, never the status.

Contract consumed (slice trust-legal-contract, not redefined): `VerificationService.CreateKycUploadUrl`, `SubmitKyc` (`additional_doc_refs`),
`GetVerificationStatus` (`reject_reason_code`, `NOT_SUBMITTED`), `GetPublicBadges`, and payout `FAILED_PRECONDITION`/`UNAVAILABLE` semantics.

## Capabilities

### New Capabilities
- `seller-kyc-ui`: seller-app KYC page, payout-blocked messaging, buyer KYC removal and the public shop badge.

### Modified Capabilities
none

## Impact

- team-frontend-seller: `src/app/seller/verification/{page,loading}.tsx`, `src/features/seller/verification/*`, `src/lib/gateway/verification.ts`,
  `src/features/seller/nav.tsx`, `src/app/seller/wallet/page.tsx`, `PayoutButton.tsx`, regenerated proto stubs.
- team-frontend: delete `src/app/(shop)/account/verification/*`, `src/features/account/verification/*`; edit `AccountShell.tsx`,
  `ConsumerShell.tsx`, `src/app/(shop)/shop/[id]/page.tsx` and its storefront view, `src/lib/gateway/verification.ts`; update tests that
  list `/account/verification`.
- platform-e2e / `platform-core/tools/seed-marketplace.sh`: demo sellers verified through the real RPCs (shared with payment-payout-kyc-gate).

## Non-goals

Admin queue UI (slice admin-kyc-queue). Gating listing creation on KYC. Bank-holder name matching. Real eKYC, liveness. Any buyer-side KYC.
Showing a badge anywhere other than the shop page (listing cards, search) in this slice.

## Non-functional requirements

- Status page server render p95 <= 400 ms locally (one `GetVerificationStatus` call, no blocking on badges).
- Image bytes never pass through the Next.js server: only the browser talks to the presigned URL; Server Actions carry metadata only.
- Per-file client validation runs before any network call; upload of 3 files at 5 MiB each completes within the 15 min presign TTL on a local stack.
- Shop page adds at most one `GetPublicBadges` call (1 id) and degrades to "no badge" if it fails, never blocking the page.
- UI copy Vietnamese; every control labelled (FormItem), keyboard operable, status not conveyed by colour alone.

## Data classification

- ID document images (face photo, ID number): sensitive personal data. Purpose: seller identification for payouts (Decree 85/2021 platform
  duty as cited by the initiative, unverified). Basis: explicit consent checkbox on the form, unchecked by default and required before the
  upload starts. Retention: owned by team-verification (VERIFIED kept while the account exists; REJECTED/superseded deleted after 90 days,
  DPO sign-off pending). This slice stores nothing: files go browser to private bucket; the UI holds `File` objects and `doc_ref` keys only in
  component state, never in localStorage, URLs, logs or analytics events.
- `doc_ref` keys: opaque; never rendered, logged or sent to GA4/tracking.
- Reject reason code: seller's own review outcome, shown only to that seller.
- Public badge: boolean only, no personal data.

## Threat model

- Assets: ID images in transit, KYC status, payout eligibility.
- Abuse: (1) seller submits a doc_ref issued to another user: server rejects, UI shows a generic error and does not retry; (2) oversized or
  wrong-type file: blocked client side and again by the signature; (3) IDOR on status: the page always requests the principal's own user id,
  never a query-param id; (4) buyer without seller scope opening `/seller/verification`: existing seller access guard applies; (5) status
  leakage via shop page: only `verified` boolean consumed; (6) presigned URL leakage via logs/referrer: URL stays in the Server Action
  response and the PUT call, never logged or put in the DOM.
- Authz rule: seller session with `listing.write` (same guard as the other `/seller/*` pages); guests redirected to login.
