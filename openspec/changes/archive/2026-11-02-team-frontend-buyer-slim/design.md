## Decisions

- Dead code was found with a reachability scan from the Next entry files (pages, layouts, routes, middleware) plus a per-export scan, run on this branch and on the split base `585b958f`; only exports that became unused because of the move were removed. Code that was already unused before the split (for example `FlashSaleSection`, `listSponsoredSlots`) and code still rendered by the buyer app (`VoucherManager` on `/vouchers`, the review moderation controls on `/listing/[id]`) stays.
- Types still used inside a module (`ViewModeration`, `ListingPage`, `ViewBundle`, `DisputeStatusName`) stay even when no other file imports them.
- `next` is the cross-app parameter because `@fe-platform/session` already builds `loginPath?next=`; the seller and admin login pages accept it and keep `returnUrl` for their own guards. Both values go through the app's `safeReturnUrl`, so only same-origin paths are honoured; the buyer side also clamps `next` to a path before building the link.
- A signed-in user with the app's access is redirected away from `/login`, so the buyer link works for a seller who already has a seller session; a signed-in user without access still sees the form to switch account.
- Logout stays per app (D3): each app's `logoutAction` deletes only the names from its own session config. The new tests seed all six cookie names and assert the delete set is exactly the app's pair and never a `__Host-` variant of another app's.
- Gitops: the seller and admin baselines copy the buyer layout (`envs/services/<app>.yaml`, overlays under `envs/{staging,prod}/services/`). `healthPath` is `/login` because `/` redirects in both apps. They are not added to any ApplicationSet yet, which is recorded in the gitops README.

## Risks

- Removing `listMyListings` from the buyer drops one buyer-side assertion of the moderation mapping; `getListing` and `listListings` still assert that screen flags never reach the view, and the seller app keeps the seller-surface test.
- The seller logout button is a server-action form inside client components; it posts to the seller app's own action only.
