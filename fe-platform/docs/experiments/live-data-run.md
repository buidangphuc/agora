# Live-data run: buying flow against a real gateway

Date: 2026-10-30. One run by one agent, in a real browser, against the local `mainx` compose stack (images built from the go-sdk branch, so the gateway and services are the migrated ones). The app ran with `next dev` and `GATEWAY_URL=http://127.0.0.1:8080`.

## Stack

Started only what the flow needs (existing stopped containers, `docker start`): postgres, redis, redpanda, flipt, team-identity, team-domain (listings), team-ai, team-order, team-payment, team-promotion, team-audit, team-gateway; later opensearch, team-search and team-search-indexer for search. Everything was stopped afterwards (0 team or mainx containers running).

## Seed

A seller registered through `AuthService.Register` (role `seller`) and three published listings created through `ListingService.CreateListing`. The stack also held about fifty old e2e listings.

## Steps and results

| Step | Result |
|---|---|
| `/products` | Real listings rendered as cards; first page was old e2e data, not the new listings (gap G7, cursor paging added) |
| `/shop/cart` signed out | Redirected to `/login?next=%2Fshop%2Fcart` |
| `/register` with `next` | Account created through the gateway, session cookie set, returned to the product page |
| Add to cart | `CartService.AddToCart`, landed on the server-side cart |
| Quantity 1 to 2 | `UpdateCartItem`; subtotal came back from the backend (900,000) |
| `/shop/checkout` without an address | Empty-state prompt, quote hidden |
| Add address | `CreateAddress`, new address selected |
| Quote | `PreviewCheckout`: items 900,000, shipping free, total 900,000 |
| Place order (cash on delivery) | `CreateOrder` returned one order; confirmation page showed it with status "Waiting for payment" |
| `/dashboard` | `ListBuyerOrders` listed the order; cart was empty afterwards |
| Expired access token with a valid refresh token | Curl with a bad `session` cookie and a real `refresh` cookie: `/shop/cart` redirected to `/session/refresh`, both cookies rotated, cart rendered (200) |
| Bad access token and no refresh token | Redirected to `/login?next=%2Fshop%2Fcart` |
| `/products?q=lamp` with search stopped | Error page (500) |
| `/products?q=lamp` with search running | One result, resolved through `GetListing` for the hit |
| Mobile 375 px, dark | No horizontal overflow, no control under 44 px |

## Defects found by the run (fixed)

- A server action file exported a plain helper; Next refuses it ("Server Actions must be async functions") and the whole app returned 500. jsdom tests and the build gates did not see it. Fixed by moving the helper to `lib/auth-errors.ts` and adding `lib/server-actions.test.ts`, which fails when a `"use server"` file exports anything but async functions.
- The catalogue showed none of the new listings because only the first page of a large catalogue was read; the list now pages with the response cursor.
- Unit prices next to a quantity read like totals; they now say "each".
- On checkout the order card sat below a long form; it now comes before "add another address".

## Not exercised

Mock payment methods (only cash on delivery), vouchers, several sellers in one cart (so the order split in G4), listing variants (the seeded listings had none), `image_url` on cart and order lines, a suspended or banned account, logout, the account blocked and unavailable messages against a real service, dark mode beyond the products and checkout pages, and any seller screen.

## Limits

One run, one agent, no repeat. Seeded data was created through the public API and left in the local database volumes. Search needed OpenSearch and the indexer; without them the search box returns the error page (the template does not hide an unavailable dependency).
