## Why

Plan P10 section F rebuilds the main buyer pages of team-frontend in the fe-platform visual language. The buyer orders pages (`/account/orders` and `/account/orders/[id]`) still use the dense pre-P7 layout: small text, tight padding, `xs` action links and a single narrow column on wide screens.

## What Changes

- Order list: airy page header (`text-2xl md:text-3xl`), larger gaps between cards, order cards with wider padding, 80 px thumbnails from `sm`, `text-sm`/`text-base` copy instead of `text-xs`, and `sm` action links that stack full width on mobile.
- Order detail: header and progress band become platform `Card`s, the title grows, the items table and amounts sit in a two-thirds column next to the order information on `lg`, and the payable link uses the `md` size.
- Timeline card, return section and route states (loading, error, 403/404) follow the same radius, padding and gap scale.
- Presentation only: no change to server actions, data fetching, `data-testid`, roles, accessible names or headings.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src/app/(shop)/account/orders/**` and the buyer components in `team-frontend/src/features/order` (`OrderList`, `OrderItemThumb`, `OrderDetailView`, `OrderTimeline`, `ReturnRequestSection`).
- Visible changes: more whitespace, larger type, a two-column detail on wide screens.

## Non-goals

Checkout components in `src/features/order`, shared `src/components/ui`, fe-platform, tailwind config, shared layouts, seller and admin pages.
