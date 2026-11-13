## 1. Code track (team-frontend) - tests first

- [ ] 1.1 `src/features/shop/featured.test.ts` (red first): `caps at 6 and dedupes`, `drops a listing owned by another seller`, unpublished and missing skipped, stored order kept (scenarios: featured order, unpublished skipped, other shop, cap)
- [ ] 1.2 `src/features/shop/shopTheme.test.ts`: `unknown key resolves to default`, `every theme meets AA contrast` (scenarios: unknown theme, readable)
- [ ] 1.3 `ShopStorefrontView.test.tsx`: featured block order and absence, grid excludes featured, `renders without a storefront` (scenarios: order, no block, outage)
- [ ] 1.4 `src/features/shop/featured.ts` `resolveFeatured` and `shopTheme.ts` closed table
- [ ] 1.5 `page.tsx`: fetch featured ids missing from the shop listing result with `getListing` in parallel; pass to `ShopStorefrontView`; leave the bundles block untouched
- [ ] 1.6 `ShopStorefrontView.tsx` featured block (`data-testid="featured-listings"`), `ShopHeaderCard.tsx` hero themed with `data-theme` (scenarios: all)
- [ ] 1.7 `team-frontend` `npm run check` green

## 2. E2E track (platform-e2e, team-frontend/FEATURES.yaml)

- [ ] 2.1 `tests/e2e/features/frontend/shop_storefront_ui.feature`: scenarios "Featured listings appear first in the chosen order", "A listing unpublished after being featured is skipped", "A shop without featured listings has no featured block", "A themed shop renders its theme"
- [ ] 2.2 Steps seed through the real gateway API (seller registers, creates and publishes listings, `UpsertStorefront` with featured ids and theme); no route fulfils
- [ ] 2.3 `team-frontend/FEATURES.yaml` entry for `/shop/[id]` featured and theme, `covered_by` the feature above
- [ ] 2.4 `openspec validate shop-page-featured-theme --strict`, features check, pre-commit-ci
