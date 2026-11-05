## MODIFIED Requirements

### Requirement: Order screens follow the type scale, colour and responsive rules

Order screens SHALL use only the 12/14/16/20/24 type scale (no `text-[Npx]` or other arbitrary values) and
design tokens. Brand colour SHALL appear only on primary CTAs, `PriceTag` and badges/counts; statuses SHALL use
semantic tones. At a 375px viewport the page SHALL have no horizontal page scroll, status tabs SHALL scroll
horizontally inside their own container, order rows and the items `Table` SHALL stack, the `Stepper` SHALL be
vertical, header actions SHALL wrap into full-width buttons with the primary action first, and the Modals SHALL
be full width. At 1024px and wider the detail content SHALL be at most 1024px wide (the `max-w-5xl` content
column of the order detail layout), the `Stepper` horizontal, the items shown as table columns and header actions
inline at the right of the header.

#### Scenario: Mobile layout at 375px

- **WHEN** `/account/orders` and a detail page are opened at a 375px wide viewport
- **THEN** `document.documentElement.scrollWidth` does not exceed the viewport width, the tab bar scrolls inside
  its own container, and the action buttons are full width

#### Scenario: Desktop layout at 1280px

- **WHEN** a detail page is opened at 1280px wide
- **THEN** the `Stepper` is horizontal, the items are shown as table columns and the content column is at most
  1024px wide
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/ui_orders_layout.feature` `Desktop layout at 1280px`

#### Scenario: No arbitrary type sizes

- **WHEN** the token lint runs over `src/features/order/` and `src/app/account/orders/`
- **THEN** it reports no `text-[..px]`, hex colours or other arbitrary values
