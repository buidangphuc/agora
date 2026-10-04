## 1. Code — team-frontend: shell, loading and error states

- [x] 1.1 Add `src/features/account/AccountShell.tsx` (server): `Breadcrumb`, `h1`, left menu on `lg+`, scrollable strip at 375px, `aria-current="page"` from a `current` prop; verify a Vitest test renders it and asserts the active item and the nav landmark label
- [x] 1.2 Add `loading.tsx` (shell-matching `Skeleton`) for `/account/addresses`, `/account/security`, `/account/verification`, `/account/referral`, `/account/following`, `/favorites`, `/notifications`, `/login`, `/register`; verify each renders in Vitest and measured CLS on a throttled load is 0 (CLS measured with Playwright on the agora production frontend (CDP-throttled network and 4x CPU): `/account/addresses`, `/security`, `/verification`, `/referral`, `/following`, `/favorites`, `/notifications` and `/register` 0 at 1280px and 375px, `/login` 0.0002 at 1280px.)
- [x] 1.3 Add `error.tsx` (`Result status="error"` + "Thử lại" via `reset()`) for `src/app/account/` and for `/favorites` and `/notifications`; verify a Vitest test calls `reset` on click

## 2. Code — team-frontend: Server Action contract

- [x] 2.1 Normalise `features/address/actions.ts`, `features/account/actions.ts`, `features/account/referral/actions.ts`, `features/account/verification/actions.ts`, `features/notification/actions.ts`, `features/engagement/actions.ts` to `{ ok, error?, data? }` with `revalidatePath` on the owning route; verify the existing `actions.test.ts` files are updated and pass, plus a new test that a gateway error resolves to `{ ok: false, error }` (note: add/removeFavorite, follow/unfollow, add/removeFromCollection, checkIn, createCollectionAction, subscribeAlertAction and unsubscribeAlertAction keep their legacy shape because the listing page and shell components call them; ActionResult-shaped createCollectionResultAction and removeAlertSubscriptionAction serve these routes)
- [x] 2.2 Make `loginAction`/`registerAction` return `{ ok: false, error, fields? }` on failure and keep `redirect()` on success; verify `features/auth/actions.test.ts` passes for both outcomes

## 3. Code — team-frontend: login and register

- [x] 3.1 Rebuild `LoginForm`/`RegisterForm` on `Card` + `FormItem` + `Input`/`Select`, client validation (required, min 3/4), pending `Button`, `Alert type="error"` with `role="alert"`, preserved `name`/`id`; verify a Vitest test covers empty-field, pending, and wrong-credentials scenarios
- [x] 3.2 Update `app/login/page.tsx` and `app/register/page.tsx` headings to the 24/20 scale and link between them; verify `auth/login.feature` and `auth/register.feature` selectors still match (run both) (note: selectors verified by unit test and render check; e2e run needs the stack)

## 4. Code — team-frontend: addresses

- [x] 4.1 Rebuild `AddressManager` as a server-rendered card list plus `AddressActions` client island; default `Tag`, `Empty` state, "Thêm địa chỉ mới" primary `Button`; verify the empty and populated render tests pass
- [x] 4.2 Rebuild `AddressModal` on `Modal` + `FormItem` (pending, field help, success/error toast) and replace `window.confirm` with a confirm `Modal`; verify tests for add, cancel-with-Escape (focus returns) and failure toast
- [x] 4.3 Wrap `/account/addresses` in `AccountShell`; verify the page test and `identity/addresses.feature` locators pass (note: page test passes; identity/addresses.feature locators checked against the new markup, the feature itself needs the stack)

## 5. Code — team-frontend: security and verification

- [x] 5.1 Rebuild `/account/security` with `Table` sessions and history, `Tag` result, `Empty`, inline `Alert` + retry per section, `Pagination` via `?page=`; verify tests for empty, failure and revoke
- [x] 5.2 Rebuild `RevokeSessionButton` with confirm `Modal`, pending and toast; verify the revoke scenario test passes
- [x] 5.3 Rebuild `/account/verification`: `Descriptions` + status `Tag` (Verified/Pending/Rejected/Not submitted) with text; verify a test per status
- [x] 5.4 Fix `SubmitKycForm` a11y: `FormItem` labels bound with `htmlFor`/`id`, `Select`, `Input` with help as `aria-describedby`, submit disabled until the reference is non-empty, pending + toast; verify with `getByLabelText("Loại giấy tờ")` and `getByLabelText("Mã tham chiếu tài liệu")` in Vitest (no new test dependency)

## 6. Code — team-frontend: referral and following

- [x] 6.1 Rebuild `/account/referral` with `Statistic`, `Descriptions`, copy `Button`, `Timeline` ledger and `Empty`; `GenerateReferralCodeButton` and `RedeemReferralForm` on `Button isLoading`/`FormItem`; verify tests for generate, invalid redeem, empty rewards
- [x] 6.2 Rebuild `/account/following` with summary `Card`, `Tabs` as links on `?tab=`, `Avatar`/`Image` shop cards showing the real shop display name (depends on `shop-display-name`; "Shop #<6 chars>" only for an empty name), Suspense + `Skeleton` for the product feed, `Empty` states; verify `?tab=items` renders the items tab selected without JS and tests cover the real name and the empty-name fallback (note: link-variant Tabs mark the selected tab with aria-current="page", not aria-selected)

## 7. Code — team-frontend: favorites and notifications

- [x] 7.1 Rebuild `/favorites` header (`Badge` total), `?page=` `Pagination`, `Empty`, unknown-collection `Result`; keep `ListingGrid` and its props unchanged; verify `git diff` shows no change under `src/features/listing/` and the existing favorites tracking test still passes (note: the repo has no existing favorites tracking test; verified by an empty git diff under src/features/listing and a page test asserting ListingGrid receives only `listings`)
- [x] 7.2 Give `CollectionsManager` pending/disabled/toast on create and remove; verify its test and `engagement/collections.feature` pass (note: collections.feature itself needs the stack; its selectors are preserved and unit-tested)
- [x] 7.3 Split `NotificationsView` into a server list + `Tabs` on `?tab=`/`?page=` and client islands (`NotificationPrefsForm`, `AlertSubscriptions`); do not render any "Mark all read" control (hidden until a backend exists); preserve `data-testid`/`data-type`; verify a test asserts both attributes, the tab-from-URL render and that no mark-all-read control exists
- [x] 7.4 Rebuild `NotificationPrefsForm` on `Checkbox`/`Select`/`FormItem` with pending + toast; verify save success and failure tests

## 8. Code — team-frontend: quality gate

- [x] 8.1 Add a responsive Vitest/Playwright-component check at 375px and 1280px for the nine routes (no horizontal scroll, menu placement); verify it passes (note: jsdom has no layout, so the Vitest check asserts the responsive class contract for all routes; layout measured at 375px and 1280px in the browser pane: no page-level horizontal scroll once the root layout header, which overflows at both widths and is outside this change, is excluded)
- [x] 8.2 Verify tracking hooks untouched: `git diff --stat` shows no edits to `src/features/tracking/`, `src/features/listing/`, `src/features/recommendations/`; verify `npm run check` and `npx next build` are green and the token lint reports 0 for the changed files

## 9. E2E — platform-e2e

- [x] 9.1 Add `team-frontend/FEATURES.yaml` entries (`status: planned`): `account.shell-nav`, `account.address-delete-confirm`, `account.security-revoke`, `account.kyc-form-labels`, `account.referral`, `account.following-tabs`, `account.favorites-url-state`, `account.notifications-tab-url`, `auth.login-inline-error` (frontend-owned UI behaviour, `acceptance` lines 1:1 with the spec scenarios); verify `make -C platform-e2e features-check` (note: ran `scripts/features.py --strict` from the e2e venv, which is what `make features-check` runs; all 9 entries valid, status planned)
- [x] 9.2 Add `tests/e2e/features/frontend/account.feature` with steps and page objects (`AccountShellPage`, `AddressesPage`, `VerificationPage`, `NotificationsPage`) covering: menu `aria-current` and Back, address delete confirm modal, KYC labels by accessible name, notification tab in URL after reload, unknown favorites collection `Result`, wrong-password inline `Alert`, 375px no horizontal scroll; verify green and flip the entries to `automated` (account.feature runs green against the agora stack (7 scenarios, the login steps are now bound); `account.shell-nav`, `address-delete-confirm`, `kyc-form-labels`, `notifications-tab-url` and `auth.login-inline-error` flipped to `automated`; `account.referral`, `following-tabs`, `favorites-url-state` and `security-revoke` stay `planned`.)
- [ ] 9.3 Extend, not duplicate: add one step to `frontend/account_security.feature` for the revoke confirm modal and keep `identity/addresses.feature`, `auth/login.feature`, `auth/register.feature`, `engagement/collections.feature`, `buyer/consumer_pages.feature` green; verify `make -C platform-e2e` runs all of them green (NOT TICKED: identity/addresses, auth/login, auth/register, engagement/collections and buyer/consumer_pages run green against the agora stack. The revoke-confirm scenario cannot pass here: team-identity never records a session at login, so the sessions list is empty and there is no `Thu hồi` button.)
- [x] 9.4 Run `openspec validate ui-phase-account --strict`; verify it is valid
