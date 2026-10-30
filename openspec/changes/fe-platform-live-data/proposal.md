## Why

The template's screens still mix real and invented data. `/shop` serves hardcoded products and does cart maths in `lib/shop.ts`, carries the cart in the URL, keeps delivery details in a cookie and never places an order; the internal dashboard shows fixed "Order 1001" rows; sign-in is a cookie nobody can obtain. A template that agents copy must show the real path: every value on a screen comes from a gateway API, and anything the backend cannot yet provide is a recorded gap, never an invented value.

## What Changes

- Remove all hardcoded product, cart, order and delivery data from `templates/app`: delete `lib/shop.ts` and `lib/shop-delivery.ts`; screens read through `@fe-platform/api-client` only.
- Catalogue: `/products` (list) and `/products/[id]` (detail) are the only catalogue pages, backed by `ListingService` (`ListListings`, `GetListing`); `/shop` and `/shop/[id]` are removed and redirect to `/products`. Optional search box on the list through `SearchService.SearchListings`.
- Cart: `/shop/cart` uses `CartService` (`GetCart`, `AddToCart`, `UpdateCartItem`, `RemoveFromCart`) through server actions; the cart lives on the server, not in the URL.
- Checkout: `/shop/checkout` lists the buyer's saved addresses (`AddressService`), lets them add one (`CreateAddress`), quotes with `OrderService.PreviewCheckout`, and places the order with `CreateOrder`. Confirmation reads the order with `GetOrder`. Field validation is the backend's; the screen shows its errors.
- Sign-in: real `/login` and `/register` pages calling `AuthService.Login` and `Register`; the token goes into an httpOnly session cookie; cart, checkout, confirmation and the dashboard require a session; a logout action clears it.
- Dashboard: lists the buyer's orders from `OrderService.ListBuyerOrders` instead of fixed rows.
- Gap register `docs/gaps.md`: each missing or doubtful backend capability (no image URL on listings, no stock display, anything a screen needs that no RPC returns) with the screen it affects, what was done instead, and the owning service. Screens show honest empty or unavailable states for a gap.
- Verified against a real gateway: a minimal local stack (gateway, identity, listing, order and what they require) is started for the check and stopped afterwards; the buying flow runs end to end and the result is recorded.
- Docs: `docs/agents/frontend.md` drops the "/shop is a demo" paragraph and states the no-hardcoded-data rule; `api-client` exports the view types the screens need (minor changeset).

## Capabilities

### New Capabilities
none (the template consumes existing backend capabilities; no new product behaviour is defined here)

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `templates/app/{app,lib,components}`, `packages/api-client/src` (exports only), `docs/{gaps.md,agents/frontend.md}`, `.changeset/`.
- Routes: `/shop` and `/shop/[id]` removed (redirect); `/login`, `/register` added.
- No proto change, no backend change. If the backend lacks something, it becomes a gap entry, not a workaround in the template.

## Non-goals

Fixing backend gaps, vouchers, payment execution beyond choosing a method the backend accepts, seller screens, returns and shipment tracking, product images beyond what the contract provides, password reset, refresh-token rotation (gap if the session expires), native apps, and publishing packages.

## Non-functional requirements

- No price, product, cart line, order or address literal remains in `templates/app` source or tests outside test fixtures that stand in for gateway responses.
- Money, totals and field rules are the backend's; the screen formats but never computes.
- No personal data in URLs or logs; the token is only in an httpOnly cookie; client components never import the api-client or `lib/session` (existing boundary rules stay green).
- `npm run check` and the build stay green; the real-gateway run is recorded with what ran and what did not.
