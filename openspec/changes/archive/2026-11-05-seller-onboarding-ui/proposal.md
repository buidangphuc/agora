## Why

G15: a buyer who opens the seller app hits a dead end ("Cần tài khoản Người Bán", no button) and cannot become a seller. The backend half (`AuthService.BecomeSeller`, slice `identity-seller-onboarding`) grants the role at once; this slice is the seller-app half. Decision 2026-11-02 (spec review round 1): the seller-app UI moves out of `identity-seller-onboarding` into this slice, owner team-frontend-seller.

initiative: p10-backend-gaps

## What changes

- Seller layout (`team-frontend-seller/src/app/seller/layout.tsx`): a signed-in principal without `listing.write` and without `status: "restricted"` sees an onboarding screen in place (seller terms text, current terms version, button "Đồng ý và trở thành Người Bán"); no redirect, no page content.
- Accept server action: calls `AuthService.BecomeSeller` with the displayed current terms version, writes the returned access token to the `seller_session` cookie only (`seller_refresh` untouched), then continues to `/seller/new`.
- Restricted principal (token `status: "restricted"`): the existing inline 403 "Tài khoản Người Bán đang bị hạn chế" stays, no onboarding button.
- Suspended or banned account: suspend/ban bulk-revokes sessions, so `BecomeSeller` answers UNAUTHENTICATED and the browser is sent to the seller login (no onboarding, no cookie). The denied notice is reachable only for PERMISSION_DENIED (account restricted after page load).
- `/sell` keeps redirecting to `/seller/new`.
- The text "Cần tài khoản Người Bán" disappears from the seller app; main specs `login-enforcement-ui`, `login-return`, `order-lifecycle-ui` that assert it are MODIFIED.
- `fe-platform/docs/gaps.md`: row G15 removed as a task of this slice. Only this slice edits that row; `identity-seller-onboarding` already dropped its G15 task.
- Remaining occurrences of "Cần tài khoản Người Bán" are removed or updated: `platform-e2e/tests/e2e/step_definitions/seller_cockpit_steps.py`, `team-frontend-seller/FEATURES.yaml`, `team-frontend-seller/README.md`, `SellerAccessDenied.tsx`, `layout.test.tsx`, plus the feature files.

## Capabilities

### New Capabilities
- `seller-onboarding-ui`: onboarding screen, accept action, denied notice, session cookie update.

### Modified Capabilities
- `login-enforcement-ui`: buyer branch of the seller layout result becomes the onboarding screen; denied notice added; restricted notice unchanged.
- `login-return`: scenario "Signed-in user without the scope is not redirected to login" asserts the onboarding screen.
- `order-lifecycle-ui`: scenario "a session without seller scope sees no seller orders" asserts onboarding instead of the old text.

## Impact

`team-frontend-seller/src/app/seller/layout.tsx` (hot file shared with identity-sso-handoff), `src/features/seller/` (`SellerAccessDenied.tsx`, new onboarding component and action), `src/lib/gateway/session.ts` (cookie write helper), vendored proto already provided by `p10-gaps-contract`; `fe-platform/docs/gaps.md`; platform-e2e features. Depends on `identity-seller-onboarding` (RPC and gateway exposure).

## Non-goals

Terms text management beyond a constant text and version string; admin approval; un-restricting; revoking the seller role; any change to RPC behaviour (owned by `identity-seller-onboarding`).

## NFR

- Accept click to `/seller/new` rendered: p95 < 1.5 s locally (one RPC of p95 < 200 ms plus one navigation).
- While an accept is pending the button is disabled and at most one `acceptSellerTerms` call is in flight.
- Onboarding screen and denied notice render without any upstream call (session claims only), so they stay up when the gateway is down; the accept action reports an error line when the gateway is unavailable.

## Data classification

- Seller terms version string sent in `BecomeSeller`: not personal; purpose proof of acceptance, stored upstream (basis contract, retention account lifetime, owned by `identity-seller-onboarding`).
- Access token (bearer secret) returned by `BecomeSeller`: written only to the HttpOnly `seller_session` cookie; never logged, never rendered, never put in a URL or returned to the client component.
- No new personal data is stored by the seller app.

## Threat model

- Assets: seller role, `seller_session` cookie.
- Abuse cases: (1) restricted/suspended/banned user clicks accept to regain seller power -> upstream refuses (UNAUTHENTICATED for revoked sessions, PERMISSION_DENIED for restricted), UI shows no role change and no cookie change; (2) forged server-action call without a session -> error, no RPC, no cookie; (3) client supplies another user id or token -> action takes none, target is the cookie principal; (4) token leakage via the action result -> action returns only `{ok}` or an error code; (5) stale terms version -> INVALID_ARGUMENT shown as an error line.
- Authz rule: enforced upstream (`BecomeSeller` for ACTIVE authenticated users); the UI only hides the button for restricted and refused accounts and never decides authorization itself.

## Accepted assumption

No startup check ties the seller app's `SELLER_TERMS_VERSION` to identity's `SELLER_TERMS_VERSIONS`; a mismatch shows the INVALID_ARGUMENT error line.
