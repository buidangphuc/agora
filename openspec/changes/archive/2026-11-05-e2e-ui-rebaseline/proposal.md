## Why

A full UI e2e run of main against the local `mainx` stack (2026-11-05, triage in `design.md`) left 326 red tests.
100 of them are test drift: the apps changed on purpose (seller and admin split into their own Next apps, semantic
token aliases, renamed copy, moved markup) and the steps did not follow, and 68 of those read a streamed page before
React swapped its hidden Suspense segment in. One red is a real bug: the buyer notifications page links a seller
notification to `/seller/...` on the buyer app, which has no seller routes since the seller-app split. Two main
specs still describe behaviour that was removed or widened by design. Without a rebaseline the suite cannot tell
a new regression from known drift.

## What Changes

- **team-frontend**: `NotificationsView` renders a `link_url` under `/seller` through `sellerAppUrl()`, so the
  new-order notification opens the seller app. Test-first vitest in `notifications/page.test.tsx`.
- **platform-e2e**:
  - `goto_streamed(page, url, wait_until)` in `src/core/base_page.py` (used by `BasePage.goto`) and
    `LoginPage.login` wait for React's hidden streaming segment to be swapped in; the step helpers that called a
    raw `page.goto` then read the page use it (pattern A).
  - Semantic token class names, app routing for `/seller*` and `/admin*` paths (`support/apps.py`), renamed copy,
    moved markup and missing waits (patterns B to E).
  - `promotion_admin_vouchers_steps.py` diffs `fe-platform/docs/gaps.md` against the pinned base `05978392`
    instead of `origin/main`.
- **Specs**: `ui-orders` (desktop detail cap 1024px, the `max-w-5xl` layout), `admin-moderation-ui` (admin links
  live in the admin app only; a non-admin sees "Không có quyền"), `order-lifecycle-ui` (seller links open on the
  seller app).
- **Known reds**: the 221 environment reds are listed by missing dependency in `design.md`; no fix here.

Repos touched: team-frontend, platform-e2e, openspec. No proto, API, data or deployment change.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `ui-orders`: the order detail content column is capped at 1024px instead of 960px.
- `admin-moderation-ui`: admin entry links are only in the admin app; a non-admin opening one is refused by the admin layout.
- `order-lifecycle-ui`: a `/seller` notification link opens on the seller app.

## Impact

- team-frontend `src/features/notification/NotificationsView.tsx` and its page test; container rebuild.
- platform-e2e step definitions, `base_page.py`, `login_page.py`, `support/apps.py`, three feature files
  (step wording only, no scenario renamed, so no FEATURES.yaml change), README "Streamed pages".

## Non-functional requirements

- No added latency on any page: the app change is a string prefix check per notification row at render time.
- E2E runtime: `wait_streamed` returns at once on a non-streamed page and waits at most 10 s otherwise.

## Data classification

No new data is collected, stored, logged or sent. Notification links carry an order id that was already in the
link; the seller app URL is a public build-time setting (`NEXT_PUBLIC_SELLER_APP_URL`).

## Threat model

- Open redirect: only a `link_url` whose path starts with `/seller` is rewritten, and only to the configured seller
  app origin; absolute or protocol-relative `link_url` values are rendered unchanged as before (no new sink).
- Authorization is unchanged: the seller app still checks that the order belongs to the signed-in seller (404 for
  another seller, scenario kept).
