## 1. Tests first

- [x] 1.1 `team-frontend/src/lib/react-dom-boundary-patch.test.ts` (unit, below the spec scenarios): upstream regression case on the vendored `react-dom/server` plus an "every bundle patched" check; verify red with a no-op script
- [x] 1.2 e2e scenario "The page streams to the end whatever the description length" in `product_detail_ui.feature`, steps in `d1_pdp_steps.py`, FEATURES.yaml entry

## 2. Fix

- [x] 2.1 `team-frontend/scripts/patch-react-dom-boundary.mjs` backports facebook/react#34694; wired into `dev` and `build`; vitest green

## 3. Verification

- [x] 3.1 Rebuild `team-frontend:local` into `mainx`; the new e2e scenario passes and a 1-60 curl sweep completes
- [x] 3.2 The five UI e2e tests pass on `mainx`: admin_cases_ui dismiss, admin_listing_moderation_ui approve, listing_report_ui guest and buyer, seller_listing_status_ui
- [x] 3.3 `openspec validate pdp-stream-cutoff --strict`, team-frontend `npm run check`, features check, pre-commit-ci
