## 1. Code track: team-frontend-seller

- [ ] 1.1 Failing tests then `SellerOnboarding` component (terms text, version constant, accept button, pending-disabled, error line, denied notice) and layout branch order in `seller/layout.tsx`. Scenarios: Seller onboarding UI: a buyer sees the onboarding screen, an existing seller never sees onboarding, the accept button is disabled while pending, a restricted account sees no onboarding button; Users UI: Seller layout shows the seller-account notice to a buyer, Seller layout shows the restricted notice
- [ ] 1.2 Server action `acceptSellerTerms`: `BecomeSeller` with the displayed version, set `seller_session` only, redirect `/seller/new`; PERMISSION_DENIED -> denied notice with fixed text; UNAUTHENTICATED -> login redirect; INVALID_ARGUMENT/UNAVAILABLE -> error line; no session -> error, no RPC. Scenarios: Seller onboarding UI: accepting sends the displayed version and updates the session cookie, the access token never reaches the client, the accept action refuses to run without a session, an unknown terms version shows an error, an unavailable gateway shows an error, an expired session on accept goes to login, a denied acceptance shows the denied notice
- [ ] 1.3 Remove the "Không có quyền" / "Cần tài khoản Người Bán" branch from `SellerAccessDenied` (restricted only); `/sell` stays a redirect. Scenarios: Seller onboarding UI: /sell keeps redirecting; Users UI: Seller pages do not send a principal without the listing.write scope to login
- [ ] 1.4 Update unit tests named in the MODIFIED specs: `layout.test.tsx` (`buyer sees onboarding not login`, `no seller orders without listing scope`), `restrictedGuard.test.tsx`. Scenarios: Signed-in user without the scope is not redirected to login; Order lifecycle: a session without seller scope sees no seller orders
- [ ] 1.5 `npm run vendor:check`, tests, lint, typecheck in team-frontend-seller

- [ ] 1.6 Remove or update every remaining "Cần tài khoản Người Bán": `platform-e2e/tests/e2e/step_definitions/seller_cockpit_steps.py` (~line 181), `team-frontend-seller/FEATURES.yaml` (~lines 1305, 1420, 1448), `team-frontend-seller/README.md` (~line 16), `SellerAccessDenied.tsx`, `layout.test.tsx`; add the static scan test `removedText.test.ts`. Scenarios: Seller onboarding UI: no source file shows the removed text, the screens offer the shopping and logout actions, an empty token is an error

## 2. Docs

- [ ] 2.1 Remove row G15 from `fe-platform/docs/gaps.md`; document `SELLER_TERMS_VERSION` in team-frontend-seller README. Scenario: Seller onboarding UI: a buyer becomes a seller in the browser (gap closed)

## 3. E2E track

- [ ] 3.1 `platform-e2e/tests/e2e/features/frontend/seller_onboarding_ui.feature`: Seller onboarding UI: a buyer becomes a seller in the browser, a returning seller is not asked again, an anonymous visitor is sent to login, a suspended buyer is sent to login in the browser, a banned buyer is sent to login in the browser, a restricted seller sees the inline 403 in the browser, a buyer restricted after page load is refused
- [ ] 3.2 Update `user_enforcement_ui.feature` scenario "Users UI: Buyer sees the seller-account notice in the browser" to assert the onboarding button and the absence of "Cần tài khoản Người Bán"; update `platform-e2e/tests/e2e/features/auth/login_return.feature` scenario "Signed-in user without the scope is not redirected to login" to assert the onboarding button. Scenarios: Users UI: Buyer sees the seller-account notice in the browser; Signed-in user without the scope is not redirected to login
- [ ] 3.3 `openspec validate seller-onboarding-ui --strict`
