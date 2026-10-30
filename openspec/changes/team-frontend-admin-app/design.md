## Decisions

- Paths stay `/admin/*` on the admin host (root redirects to `/admin/cockpit`). Moving the screens to `/` would have touched about 115 path strings, `revalidatePath` calls and every e2e page object for no user gain; the host, not the path, is the boundary.
- Session reuse: the moved gateway modules call a synchronous `getToken()`. Instead of rewriting every gateway factory to the async `createSessionServer`, `src/lib/gateway/session-cookie.ts` takes the cookie names and options from `defineSession("admin")` (`cookieNames`, `cookieOptions`, `isSecureRequest` of `@fe-platform/session`), and `getToken()` reads the secure-aware name. The middleware is the package's `createSessionMiddleware`.
- Scope gate in the layout: `src/app/admin/layout.tsx` renders the shared "Không có quyền" view inline (no redirect), so the URL stays on the requested screen as the old 403 view did; the pages and actions keep their own scope checks; `/forbidden` exists as the session package's `forbiddenPath`.
- The forbidden subtitle avoids the words "không có quyền" so `get_by_text("Không có quyền")` in e2e matches only the title.
- Referrer: the admin header stays `strict-origin-when-cross-origin` (same as the buyer app, which the referrer e2e asserts per path); every admin page declares `referrer: no-referrer` in its metadata.
- Duplication accepted: gateway helpers, ui facades and auth helpers are copied, not shared; the README lists them under Known gaps until step D. `LinkButton` and `usePending` came from `features/seller` and now live in `src/components` and `src/lib`.
- Dispute actions split: the buyer app keeps `createDisputeAction`; the admin app keeps `resolveDisputeAction` (it revalidates `/admin/cases`). The buyer app also drops `lib/gateway/{audit,cockpit,moderation,users}` that only admin used.
- Buyer `guardPath` loses the admin special case (path-only return URL for `/admin`), since the buyer app has no admin route; the admin copy keeps it.
- Vendor targets: `fe-platform/package.json` lists the buyer, seller and admin `vendor/` dirs; `vendor` and `vendor:check` skip a target whose app directory is missing, so the seller app can land later without touching the script.
- e2e: `BasePage.base_url_setting` picks the host per page object; `login_via_api` also sets `admin_session` on the admin host, so "an admin is logged in" works for both apps; the new 404 scenario uses only `@admin` (no seeding tag) and plain HTTP.

## Risks

- e2e was not run against the new app (stack not started); scenarios that expected GA4 `page_location` pushes on admin pages will fail because the admin app ships no analytics.
- The admin ingress is not restricted (VPN or IP allow-list is a platform-gitops follow-up, open risk in ADR-0006).
- Copied code can drift from the buyer app until it is promoted to `fe-platform`.
