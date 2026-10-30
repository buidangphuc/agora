## 1. Contract survey and gap register

- [x] 1.1 Read the listing, order, identity and search protos for every field and error the screens need; create `docs/gaps.md` with the table and an initial row per confirmed gap (images, refresh token, anything else found); verify each row names a screen, the missing capability and an owner service
- [x] 1.2 Link the register from `docs/agents/project.md` and state the rule in `docs/agents/frontend.md` (no hardcoded data; a missing capability is a gap row plus an honest empty state)

## 2. Sign-in

- [x] 2.1 Add `/login` and `/register` server-action forms using `auth.login` and `auth.register`; set the httpOnly `session` cookie (and a refresh cookie only if the response carries one); restrict `next` to same-origin paths; verify with fake-transport tests for success, `invalid` and `unauthenticated`
- [x] 2.2 Add `logout`; change `requireSession` to redirect to `/login?next=...`; apply it in the `(internal)` layout and the cart, checkout and confirmation pages; verify existing session tests are updated and a test proves an anonymous request to each page redirects

## 3. Catalogue and cart

- [x] 3.1 Turn `/shop` and `/shop/[id]` into redirects to `/products` and `/products/[id]`; add the add-to-cart server action on the detail page calling `cart.addToCart`; verify a test covers the redirect and the action
- [x] 3.2 Add the optional search form to `/products` using `search.searchListings`; verify with a fake-transport test that `q` switches to search and an empty `q` lists
- [x] 3.3 Rebuild `/shop/cart` on `lib/cart.ts` (`GetCart`, `UpdateCartItem`, `RemoveFromCart`) with quantity change and remove actions; totals come from the response only; verify tests for an empty cart, a filled cart and `unauthenticated`

## 4. Checkout and orders

- [x] 4.1 Add `lib/checkout.ts` and rebuild `/shop/checkout`: saved addresses, add-address form (`CreateAddress`), `PreviewCheckout` quote, payment-method select from the contract enum; verify tests for each loader and the `invalid` error path
- [x] 4.2 Add the `placeOrder` action using `order.createOrder` and redirect to `/shop/confirmation?order=<id>`; delete `lib/shop.ts`, `lib/shop-delivery.ts` and their tests; verify no import of either remains
- [x] 4.3 Rebuild `/shop/confirmation` on `order.getOrder` and the dashboard on `order.listBuyerOrders` with `lib/order-status.ts`; verify tests for success and `not_found`
- [x] 4.4 Add the source-scan test that fails on `SHOP_PRODUCTS`, price-literal fixtures or `lib/shop` imports in non-test app code; verify it goes red when one is added, then green

## 5. Docs and exports

- [x] 5.1 Export any view types the app needs from `api-client` (changeset minor if exports are added) and run `npm run proto:check`; remove the "/shop is a demo" paragraph from `docs/agents/frontend.md`; verify `npm run catalog:check` is green

## 6. Real gateway verification

- [x] 6.1 Work out and start the minimum local stack (gateway, identity, listing, order, search if feasible) using the repo's local stack recipe and starting only needed services; verify the gateway answers on `GATEWAY_URL`
- [x] 6.2 Seed through the APIs (a buyer, a seller, at least one published listing); run the flow in the browser pane: register, browse, search, add to cart, add address, preview, place order, confirmation, dashboard; verify and record each step with what was seen
- [x] 6.3 Turn every deviation into a fix in the template (if the template is wrong) or a row in `docs/gaps.md` (if the backend is missing something); stop the services and verify nothing is left running
- [x] 6.4 Record the run (stack used, steps, results, what could not be run) in `docs/experiments/live-data-run.md`

## 7. Done-condition

- [x] 7.1 `npm run check` exits 0 after a fresh `npm ci`, `npm run build -w templates/app` passes, no new disable comments, `npm audit` clean, `openspec validate fe-platform-live-data --strict` passes
- [x] 7.2 e2e track: none (template only, no FEATURES.yaml entry); confirm `repo-doctor` reports no new drift in the fe-platform scope
