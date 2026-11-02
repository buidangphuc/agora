## 1. Shell

- [x] 1.1 Seller layout `<main>`, page background and gutters; sidebar, nav list, drawer top bar, shop card and logout at 44 px with large radius; verify `SellerShell.test.tsx` and `layout.test.tsx` pass

## 2. Dashboard

- [x] 2.1 KPI section, recent orders beside quick actions from `lg`, icon quick actions, button-shaped "Xem tất cả", matching skeleton; verify `(dashboard)/page.test.tsx` passes

## 3. Orders

- [x] 3.1 Orders list card with tabs and filter in the top band; verify `orders/(list)/orders.test.tsx` passes
- [x] 3.2 Order detail two-column grid with the summary column first on small screens, matching skeleton; verify `orders/[id]/page.test.tsx` passes

## 4. Verification

- [x] 4.1 `npm run check` and `npm run build` in `team-frontend-seller`
- [x] 4.2 Visual check at 375/768/1280 px on the built app: no horizontal overflow, controls at least 44 px
- [x] 4.3 `openspec validate team-frontend-layout-seller-dash --strict`
