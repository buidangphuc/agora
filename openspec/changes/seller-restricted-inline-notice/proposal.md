## Why

After the app split the seller layout redirects every signed-in principal without `listing.write` to `/forbidden`, which shows one combined notice ("Cần tài khoản Người Bán" plus the restricted line) because a restricted seller and a buyer looked the same. The live e2e run of 2026-10-30 failed the scenario "Restricted seller sees the notice in the browser" (it expects the browser to stay on `/seller/new`). Decided 2026-10-31: render the 403 inline on the requested URL, like the admin app, and tell the two cases apart with the `status` claim added by `identity-restricted-status-claim`.

## What Changes

- `@fe-platform/session`: `Principal` gains `status?: "restricted"`, read from the token's `status` claim; vendored again into the three apps. `forbiddenPath` stays in the config (admin and the shared helpers use it).
- team-frontend-seller: the seller layout renders `SellerAccessDenied` in place instead of redirecting. A buyer sees "Không có quyền" with "Cần tài khoản Người Bán"; a restricted seller sees "Tài khoản Người Bán đang bị hạn chế" with the existing restricted line. Both keep "Về trang mua sắm" and "Đăng xuất". The "Đăng ký bán hàng" button is dropped: `/sell` only redirects to `/seller/new`, so it led back to the same page (gap: no seller onboarding). The seller `/forbidden` route is deleted.
- platform-e2e: `still_on` compares against the seller host for seller paths; the restricted scenario asserts the new title; a new buyer scenario; `login_return` requests seller paths on the seller host.
- FEATURES: the restricted-seller entry moves from `team-frontend` to `team-frontend-seller`; the related seller entries follow the new wording.
- Docs: ADR-0006, `fe-platform/docs/gaps.md` (no seller onboarding), seller README.

## Capabilities

### New Capabilities
none

### Modified Capabilities
- `login-enforcement-ui`: the seller-pages requirement now distinguishes a restricted seller from a buyer and keeps the requested URL.

## Impact

`fe-platform/packages/session/src/token.ts` (+ test, changeset, vendor tarballs in three apps), `team-frontend-seller/src/app/seller/layout.tsx`, `src/features/seller/SellerAccessDenied.tsx`, `src/lib/gateway/session.ts`, deleted `src/app/(auth)/forbidden/`, `platform-e2e` steps and features, FEATURES.yaml of `team-frontend` and `team-frontend-seller`, ADR-0006, `fe-platform/docs/gaps.md`, seller README.

## Non-goals

Seller onboarding, `/sell`, restriction expiry, the admin app, backend changes (done in PR1).

## Data classification

The frontend reads the `status` claim of the signed-in user's own token to choose a message. Nothing new is stored, logged or sent.

## NFR

- The 403 is rendered by the same server layout render; no extra gateway call (the shop-name lookup is skipped for a principal without `listing.write`).
- Touch targets of "Về trang mua sắm" and "Đăng xuất" stay at the existing `min-h-touch` (44 px) for the logout button.

## Threat model

- Assets: seller-centre pages and the seller's listings.
- Abuse case 1: a user forges `status` or `scopes` in their own cookie to reach seller pages. Control: the claim only picks the notice text; page data still goes through the gateway, which verifies the RS256 signature and enforces `listing.write` per RPC.
- Abuse case 2: a restricted seller keeps using an access token issued before the restriction. Control: unchanged; the restriction revokes sessions so the next refresh fails, and the token expires within one hour.
- Authz rule: no new route or RPC; the layout renders no page content without `listing.write`.
