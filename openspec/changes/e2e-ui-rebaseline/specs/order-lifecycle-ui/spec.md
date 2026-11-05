## MODIFIED Requirements

### Requirement: Lifecycle notifications open real pages

The notification shown to a seller for a new order SHALL open `/seller/orders/{id}` of the seller's own order on the seller app, and the pay reminder shown to a buyer SHALL open `/checkout/pay/{id}` on that buyer's own unpaid order. The buyer app's notifications page SHALL render a `link_url` whose path is `/seller` or starts with `/seller/`, `/seller?` or `/seller#` as an absolute link to the seller app (`sellerAppUrl`, `NEXT_PUBLIC_SELLER_APP_URL`), and SHALL render every other `link_url` unchanged.

#### Scenario: Order lifecycle: the new-order notification opens the seller order
- **WHEN** a buyer places an order and the seller opens the notifications page and follows "Đơn hàng mới"
- **THEN** the seller lands on `/seller/orders/{id}` of the seller app showing that order with status 200
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: the new-order notification opens the seller order`; `team-frontend/src/app/(shop)/notifications/page.test.tsx` `[Order lifecycle: the new-order notification opens the seller order] a /seller link opens on the seller app, buyer links stay relative`

#### Scenario: Order lifecycle: another seller cannot open the new-order link
- **WHEN** a different seller opens `/seller/orders/{id}` of an order that is not theirs, taken from the first seller's notification
- **THEN** the response is the not-found page (404) and shows no order data
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: another seller cannot open the new-order link`

#### Scenario: Order lifecycle: the payment reminder opens the pay page
- **WHEN** the stack runs with the short reminder lead of the lifecycle overlay and a buyer's online order nears its deadline
- **THEN** the buyer's notifications page shows one payment reminder whose link opens `/checkout/pay/{id}` with the payment simulator visible
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: the payment reminder opens the pay page`

#### Scenario: A link that only looks like a seller path stays on the buyer app
- **WHEN** a notification's `link_url` is `/sellers/x`, `/checkout/pay/{id}` or empty
- **THEN** the item links to `/sellers/x`, `/checkout/pay/{id}` or `#` respectively, never to the seller app
- **VERIFIED BY**: `team-frontend/src/app/(shop)/notifications/page.test.tsx` `[Order lifecycle: the new-order notification opens the seller order] a /seller link opens on the seller app, buyer links stay relative`
