## 1. Run

- [x] 1.1 Start only the needed mainx containers, build and serve the three apps; verify each app answers on its port

## 2. Fixes

- [x] 2.1 Keep the pending submit width in `PlaceOrderForm` and `ListingForm`; verify the d2 width scenarios pass
- [x] 2.2 Default the seller and admin login `returnUrl` to the app home; verify UI login lands on the home page
- [x] 2.3 Test-side: wait for streamed segments, fix the quick actions, plans, voucher and seller host locators; verify the seller and admin groups pass

## 3. Verification

- [x] 3.1 `npm run check` and `npm run build` in the three apps
- [x] 3.2 Grouped e2e rerun, E1c voucher check and E4 independent login check
- [x] 3.3 `openspec validate team-frontend-g-e2e --strict`
