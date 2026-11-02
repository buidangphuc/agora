## Decisions

- Reference language: `fe-platform/templates/app` (`Page`: `gap-8`, `py-8 md:py-12`, large type) and the platform `card` tokens (`rounded-2xl`, `p-6`). The seller shell keeps its 20 px page title from `SellerPageHeader`, which other pages share and is out of scope here.
- Layout only through semantic token classes and the facades (`Card`, `Link`, `Statistic`, `Descriptions`, `Tabs`, `Table`); no arbitrary Tailwind values, no new tokens.
- Touch targets: nav rows and shell buttons use `min-h-touch`; the mobile menu button is `h-11 w-11` (44 px); secondary links that were text-sized ("Xem tất cả", "Xem gian hàng công khai", quick actions) become button-shaped `Link` variants, which carry `min-h-touch`. Inline text links inside table cells (order ids) stay inline (WCAG 2.5.8 inline exception).
- Sidebar widths stay `w-64` / `w-16` (tests and the stored collapse preference depend on them); only padding and rhythm change.
- `KpiRow` keeps every `Statistic` as a direct child of `data-testid="kpi-row"` (platform-e2e reads `:scope > div`), and the skeleton keeps sharing the row class.
- Order detail grid: the summary column is first in the DOM and placed in column 3 at `lg` (`lg:col-start-3 lg:row-start-1`), so phones show status and recipient before the item table, desktops show items on the left. No `<header>` elements were added inside the packing slip, because `PrintStyles` hides `header` when printing.
- `<main>` landmark added in the seller layout; no seller page renders its own `main`.

## Visual check

Built app on port 3121 with a fake, unsigned `seller_session` cookie (the session middleware only checks the JWT shape and `exp`; the gateway is not running, so every read fails). Pages render the shell with their error states: `/seller` (KPI warning, orders and products error alerts), `/seller/orders` (error alert), `/seller/orders/<id>` (not-found inside the shell), plus `/login` without a cookie. Checked with Playwright at 375, 768 and 1280 px: `scrollWidth <= innerWidth`, and shell controls (buttons, button-shaped links, nav links) at least 44 px high. A data-filled layout was not exercised (no gateway).

## Risks

- `SellerLogoutButton` is also rendered on `/forbidden`; it only gained a larger radius and lost vertical padding (height is still `min-h-touch`).
- The SSR sidebar is expanded (`w-64`) until hydration collapses it on tablets; unchanged behaviour.
