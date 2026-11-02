## Why

Plan P10 section F rebuilds the main buyer pages on the `@fe-platform/ui` visual language: mobile-first, airy spacing, large radius, indigo, 44 px touch targets and no horizontal overflow. The checkout and pay pages still used the pre-platform spacing, text-size footer links below 44 px and button rows that squeeze on phones.

## What Changes

- Checkout shell (`src/app/(checkout)/layout.tsx`): airier padding per breakpoint, `rounded-2xl` header card with the platform shadow, a 44 px logo link, a muted "Thanh toán an toàn" pill, and footer links drawn as ghost button links.
- Checkout summary page: wider desktop gap, `min-w-0` grid columns so long content cannot push the page sideways, more bottom room for the mobile summary bar, the "Quay lại giỏ hàng" action as a full-width ghost link, the kill-switch notice in a centred column. The loading skeleton follows the same footprint.
- Pay page, its loading skeleton, error and not-found states: centred `max-w-xl` column with breakpoint spacing.
- `PaymentSimulator`: outcome actions stack full-width on phones and sit in a row from `sm`; the demo notice sits on a muted panel; "Bỏ qua và thanh toán sau" becomes a full-width ghost link.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src/app/(checkout)/**` and `team-frontend/src/features/payment/PaymentSimulator.tsx`.
- No change to server actions, data fetching, test ids, roles, accessible names or headings.

## Non-goals

Shared UI facades, `fe-platform`, tailwind config, the root and shop layouts, the shared `OrderSummary`/`MobileSummaryBar` (also used by the cart page) and the `features/order` step components; dark theme.
