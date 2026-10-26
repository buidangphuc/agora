# cart-selection-ui Specification

## Purpose
Buyer cart line and shop selection, and checkout of only the selected lines.

## Requirements

### Requirement: The cart lets the buyer tick lines and shops and checks out only the ticked lines

The cart page SHALL render a checkbox per cart line and a select-all checkbox per shop group. The initial selection SHALL be every line, or the lines named by `?items=` that exist in the cart. The "Mua hàng" action SHALL link to `selectionHref(<ticked line ids in cart order>)`, that is `/checkout?items=` followed by the ids joined by commas and URL-encoded as `URLSearchParams` does (`items=l1%2Cl3`), with no `voucher` parameter (checkout takes vouchers per scope in its own `sv`/`pv` fields, and frontend-checkout-coherence ignores a legacy `voucher`). It SHALL be disabled when no line is ticked or when checkout is switched off, and the summary item count and subtotal SHALL cover only the ticked lines. WHILE the selection is partial the summary SHALL hide the voucher discount row and SHALL show "Giảm giá được tính ở bước thanh toán". The cart page SHALL remain a server component whose first HTML response contains the line rows.

#### Scenario: Cart selection: every line is ticked by default
- **WHEN** a signed-in buyer opens `/cart` with 3 lines from 2 shops and no `items` parameter
- **THEN** all 3 line checkboxes and both shop checkboxes are checked and "Mua hàng" links to `/checkout?items=` followed by the 3 line ids in cart order joined by `%2C`
- **VERIFIED BY**: `team-frontend/src/features/cart/CartSelection.test.tsx` `all lines ticked by default`

#### Scenario: Cart selection: ticking a subset changes the link and the summary
- **WHEN** the buyer unticks 2 of the 3 lines (line A 300000 x 1 stays ticked)
- **THEN** "Mua hàng" links to `/checkout?items=<A id>` only, the summary shows 1 sản phẩm and a subtotal of 300000, and no network call is made
- **VERIFIED BY**: `team-frontend/src/features/cart/CartSelection.test.tsx` `subset updates href and summary`

#### Scenario: Cart selection: a shop select-all toggles that shop's lines
- **WHEN** the buyer unticks the select-all of shop A, which has 2 lines, then ticks it again
- **THEN** after the first click both shop A lines are unticked and shop B's line keeps its state, after the second both are ticked, and the shop checkbox is indeterminate while only one of its lines is ticked
- **VERIFIED BY**: `team-frontend/src/features/cart/CartSelection.test.tsx` `shop select all toggles its lines`

#### Scenario: Cart selection: nothing ticked disables the buy action
- **WHEN** the buyer unticks every line
- **THEN** "Mua hàng" renders as a disabled button with no `href` and the summary shows 0 sản phẩm
- **VERIFIED BY**: `team-frontend/src/features/cart/CartSelection.test.tsx` `nothing ticked disables buy`

#### Scenario: Cart selection: the checkout kill switch keeps the buy action disabled
- **WHEN** the checkout kill switch is off and every line is ticked
- **THEN** "Mua hàng" is disabled and the existing unavailable notice is shown
- **VERIFIED BY**: `team-frontend/src/app/(shop)/cart/page.test.tsx` `kill switch disables buy even with a selection`

#### Scenario: Cart selection: a partial selection hides the cart discount and carries no voucher
- **WHEN** the buyer has voucher "V1" applied on `/cart` and ticks 2 of 3 lines
- **THEN** "Mua hàng" links to `/checkout?items=<id1>%2C<id2>` with no `voucher`, `sv` or `pv` parameter, and the summary omits the discount row and shows "Giảm giá được tính ở bước thanh toán"
- **VERIFIED BY**: `team-frontend/src/features/cart/CartSelection.test.tsx` `partial selection hides discount and carries no voucher`

#### Scenario: Cart selection: the back link from checkout restores the selection
- **WHEN** the buyer opens `/cart?items=<B id>,<ghost id>` where `<ghost id>` is not a cart line
- **THEN** only line B is ticked and the ghost id is ignored
- **VERIFIED BY**: `team-frontend/src/features/cart/CartSelection.test.tsx` `initial selection comes from items param`

#### Scenario: Cart selection: an items parameter with no current line ticks everything
- **WHEN** the buyer opens `/cart?items=<ghost id>` and no id is a cart line
- **THEN** every line is ticked
- **VERIFIED BY**: `team-frontend/src/features/cart/selection.test.ts` `no valid id falls back to all lines for the cart page`

#### Scenario: Cart selection: an anonymous visitor cannot reach the selectable cart
- **WHEN** an anonymous visitor opens `/cart?items=l1`
- **THEN** the response redirects to `/login` and no cart line or checkbox is rendered
- **VERIFIED BY**: `team-frontend/src/app/(shop)/cart/page.test.tsx` `anonymous is redirected to login`

#### Scenario: Cart selection: the cart first response still holds the line rows
- **WHEN** the cart page is requested with 3 lines
- **THEN** the HTML response already contains the 3 line rows and the page source has no "use client" directive
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/ui_cart_page.feature` `Cart page is not a client component`

### Requirement: Selection helpers resolve `?items=` against the buyer's own cart

`parseItemsParam` SHALL split a comma list, trim parts, drop empty parts and de-duplicate keeping first occurrence. `resolveSelection` SHALL return the cart line ids that appear in the list, in cart order, and SHALL ignore ids absent from the cart. The builder `selectionHref` SHALL produce `?items=` from ids joined by commas, encoded with `URLSearchParams` (commas become `%2C`, the same encoding as frontend-checkout-coherence's `checkoutHref`), and SHALL work for at least 50 ids.

#### Scenario: Cart selection: parsing trims, drops empties and de-duplicates
- **WHEN** `parseItemsParam` receives `" a, b,,a ,c "`
- **THEN** it returns `["a","b","c"]`
- **VERIFIED BY**: `team-frontend/src/features/cart/selection.test.ts` `parse trims drops empties and dedupes`

#### Scenario: Cart selection: ids absent from the cart are ignored
- **WHEN** `resolveSelection` gets cart ids `[l1,l2,l3]` and the list `"l3,zzz,l1"`
- **THEN** it returns `["l1","l3"]` in cart order
- **VERIFIED BY**: `team-frontend/src/features/cart/selection.test.ts` `unknown ids are ignored and order follows the cart`

#### Scenario: Cart selection: an empty or unmatched list resolves to no line
- **WHEN** `resolveSelection` gets the list `""` or a list whose ids match no cart line
- **THEN** it returns an empty array
- **VERIFIED BY**: `team-frontend/src/features/cart/selection.test.ts` `empty or unmatched resolves to nothing`

#### Scenario: Cart selection: a 50 line selection fits one link
- **WHEN** `selectionHref` receives 50 UUID line ids
- **THEN** it returns a path of at most 2200 characters that `parseItemsParam` parses back to the same 50 ids
- **VERIFIED BY**: `team-frontend/src/features/cart/selection.test.ts` `fifty ids round trip`

#### Scenario: Cart selection: a line id that is not the buyer's own resolves to nothing
- **WHEN** `resolveSelection` gets buyer X's cart ids `[l1,l2]` and the list `"y9"` where `y9` is a line of buyer Y (not the owner)
- **THEN** it returns an empty array, so no foreign line can be selected
- **VERIFIED BY**: `team-frontend/src/features/cart/selection.test.ts` `foreign line id resolves to nothing`

### Requirement: Checkout quotes and places only the selected lines

The checkout page SHALL, when `parseItemsParam(items)` yields at least one id, resolve it with `resolveSelection` against the buyer's `GetCart`, SHALL redirect to `/cart` when no resolved line remains, and SHALL use the resolved ids as `item_ids` for `PreviewCheckout`, for `CreateOrder` and for the `begin_checkout` items. WHEN `items` is absent or parses to no id (for example `?items=`) THEN the page SHALL follow frontend-checkout-coherence's rule (empty = whole cart: `PreviewCheckout` and `CreateOrder` get an empty `item_ids`) and SHALL NOT redirect. The "Quay lại giỏ hàng" link SHALL be `/cart` plus `selectionHref`'s query for the resolved ids (`/cart?items=l1%2Cl3`), or `/cart` when no selection was given. After the order is placed the lines that were not selected SHALL remain in the cart.

#### Scenario: Cart selection: checkout uses only the selected ids
- **WHEN** a buyer with lines `[l1,l2,l3]` opens `/checkout?items=l2`
- **THEN** `PreviewCheckout` is called with `item_ids = [l2]`, the begin_checkout event holds only line l2, and the confirm step lists only line l2
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `selected ids reach preview and beacon`

#### Scenario: Cart selection: placing the order sends the selected ids
- **WHEN** the buyer confirms a checkout opened with `?items=l1,l3`
- **THEN** `CreateOrder` is called with `item_ids = [l1,l3]` and never with an empty list
- **VERIFIED BY**: `team-frontend/src/features/order/PlaceOrderForm.test.tsx` `submits the selected item ids`

#### Scenario: Cart selection: a selection naming no cart line returns to the cart
- **WHEN** a buyer opens `/checkout?items=ghost` (or `items=ghost1,ghost2`) and the cart has lines
- **THEN** the response is a redirect to `/cart` and neither `PreviewCheckout` nor `CreateOrder` is called
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `dead selection redirects to cart`

#### Scenario: Cart selection: ids not in the cart are ignored at checkout
- **WHEN** a buyer opens `/checkout?items=l1,ghost`
- **THEN** the checkout covers line l1 only and no error is shown
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `ghost ids are ignored`

#### Scenario: Cart selection: an empty items parameter is the whole cart, not a redirect
- **WHEN** a buyer with 3 cart lines opens `/checkout?items=`
- **THEN** the page does not redirect, `PreviewCheckout` is called with an empty `item_ids`, and the confirm step lists all 3 lines
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `empty items param is the whole cart`

#### Scenario: Cart selection: the back link keeps the selection
- **WHEN** a buyer on `/checkout?items=l1,l3` reads the "Quay lại giỏ hàng" link
- **THEN** its href is `/cart?items=l1%2Cl3`
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `back link carries the selection`

#### Scenario: Cart selection: another buyer's line id places nothing
- **WHEN** buyer X opens `/checkout?items=<a line id of buyer Y>`
- **THEN** the page redirects to `/cart` and buyer Y's cart is unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/cart_selection_ui.feature` `Cart selection: checkout refuses another buyer's line id`

#### Scenario: Cart selection: an anonymous visitor is sent to login
- **WHEN** an anonymous visitor opens `/checkout?items=l1`
- **THEN** the response redirects to `/login` and the return path keeps `items=l1`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/cart_selection_ui.feature` `Cart selection: an anonymous visitor keeps the selection through login`

#### Scenario: Cart selection: checking out some lines leaves the others in the cart
- **WHEN** a buyer with 3 lines from 2 shops ticks one line, presses "Mua hàng" and places the order
- **THEN** exactly one order with that one line is created and `/cart` afterwards still lists the 2 unticked lines
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/cart_selection_ui.feature` `Cart selection: checking out some lines leaves the others in the cart`

#### Scenario: Cart selection: a shop's select-all checks out that shop only
- **WHEN** a buyer with lines from shops A and B unticks shop B's select-all and places the order
- **THEN** every created order belongs to shop A and shop B's lines remain in the cart
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/cart_selection_ui.feature` `Cart selection: a shop's select-all checks out that shop only`

### Requirement: "Mua ngay" checks out only the line it added

After a successful `AddToCart` from the PDP the "Mua ngay" action SHALL open `/checkout?items=<id>` where `<id>` is the cart line whose listing and variant match the added variant in the `AddToCartResponse` cart. When that line already existed, the quantity checked out SHALL be its merged quantity. IF the response holds no matching line THEN the buyer SHALL stay on the PDP with an error message and no navigation SHALL happen. The inline panel and the mobile buy bar SHALL behave the same.

#### Scenario: Cart selection: Mua ngay opens checkout with the new line only
- **WHEN** a buyer with 2 lines in the cart presses "Mua ngay" on a listing variant that is not in the cart
- **THEN** `/checkout?items=<new line id>` opens and the checkout lists only the new line
- **VERIFIED BY**: `team-frontend/src/features/cart/PurchasePanel.test.tsx` `buy now navigates with the added line id`

#### Scenario: Cart selection: Mua ngay on an existing line checks out the merged quantity
- **WHEN** the cart already holds the variant with quantity 2 and the buyer presses "Mua ngay" with quantity 1
- **THEN** the checkout shows that line with quantity 3 and no other cart line
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/cart_selection_ui.feature` `Cart selection: Mua ngay on an existing line checks out the merged quantity`

#### Scenario: Cart selection: the action returns the matching line id
- **WHEN** `addToCartAction("L", "V", 1)` gets a cart with lines (L,V) and (L,W) from the gateway
- **THEN** it returns `ok` with `data.itemId` equal to the id of the (L,V) line
- **VERIFIED BY**: `team-frontend/src/features/cart/actions.test.ts` `returns the id of the added line`

#### Scenario: Cart selection: a listing without variants matches the empty variant id
- **WHEN** `addToCartAction("L", undefined, 1)` gets a cart whose line for L has an empty variant id
- **THEN** it returns `data.itemId` equal to that line's id
- **VERIFIED BY**: `team-frontend/src/features/cart/actions.test.ts` `matches a line without a variant`

#### Scenario: Cart selection: Mua ngay does not fall back to the whole cart
- **WHEN** the `AddToCartResponse` cart holds no line for the added listing and variant
- **THEN** the action returns `ok: false`, the buyer stays on the listing page, an error toast is shown and `router.push` is not called
- **VERIFIED BY**: `team-frontend/src/features/cart/PurchasePanel.test.tsx` `buy now with no matching line stays on the page`

#### Scenario: Cart selection: a failed add shows the error and does not navigate
- **WHEN** `AddToCart` fails with an error on "Mua ngay"
- **THEN** the existing error toast is shown, the buyer stays on the listing page and no checkout URL is opened
- **VERIFIED BY**: `team-frontend/src/features/cart/PurchasePanel.test.tsx` `failed add does not navigate`

#### Scenario: Cart selection: an anonymous visitor pressing Mua ngay is sent to login
- **WHEN** an anonymous visitor presses "Mua ngay" and `AddToCart` answers Unauthenticated
- **THEN** the visitor is sent to the login page with the listing as return path and no checkout URL is opened
- **VERIFIED BY**: `team-frontend/src/features/cart/PurchasePanel.test.tsx` `anonymous buy now goes to login`

#### Scenario: Cart selection: the mobile buy bar uses the same path
- **WHEN** the buyer presses "Mua ngay" in the mobile buy bar
- **THEN** it navigates to `/checkout?items=<added line id>` exactly as the inline panel does
- **VERIFIED BY**: `team-frontend/src/features/cart/PurchasePanel.test.tsx` `buy bar buy now uses the same selection`

#### Scenario: Cart selection: Mua ngay checks out only that line end to end
- **WHEN** a buyer with 2 other lines in the cart presses "Mua ngay" on a PDP and places the order
- **THEN** one order with only the new line is created and `/cart` still lists the 2 earlier lines
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/cart_selection_ui.feature` `Cart selection: Mua ngay checks out only that line`

### Requirement: Cart line ids never reach analytics

The GTM destination SHALL remove the `items` query parameter from every `page_location` it pushes (initial consent update and client-side navigation), the same way it removes `returnUrl`, so `/cart?items=` and `/checkout?items=` never send cart line ids to GTM or GA4. The `begin_checkout` and `purchase` events SHALL keep their existing item fields and SHALL NOT add a cart line id.

#### Scenario: Cart selection: page_location has no items parameter
- **WHEN** analytics consent is granted and the buyer navigates to `/checkout?items=l1%2Cl3&step=payment`
- **THEN** the pushed `page_location` is `<origin>/checkout?step=payment` and nothing in the data layer contains `l1` or `items=`
- **VERIFIED BY**: `team-frontend/src/lib/analytics/destinations/gtm.test.ts` `page_location has no items`

#### Scenario: Cart selection: begin_checkout carries no cart line id
- **WHEN** the checkout opened with `?items=l1` fires `begin_checkout`
- **THEN** its items hold the existing item fields only and do not contain the cart line id `l1`, and an invalid or empty `items` value pushes no `items=` fragment either
- **VERIFIED BY**: `team-frontend/src/lib/analytics/destinations/gtm.test.ts` `begin_checkout has no line id`
