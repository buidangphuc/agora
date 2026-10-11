## Why

The six UI phases (Discovery, Product Detail, Cart & Checkout, Orders, Account, Seller) each re-invent
buttons, tags, empty states and loaders. `src/components/ui/` has 11 WIP primitives, but they do not yet
meet the doc's state matrix (`UI_SYSTEM_DESIGN.md` §4): no `aria-disabled`, no consistent focus ring, no
Skeleton, no Empty, and several primitives still use arbitrary values. Ant Design solves the same problem
with a fixed component set grouped as General / Layout / Navigation / Data Entry / Data Display / Feedback,
each with defined states and a11y. We adopt that set and contract (API shape and states), implemented
natively on our tokens, so the phase changes only compose components.

## What Changes

- Bring the 11 existing primitives to the contract: Button, Input, Badge, Card, Modal, PriceTag, Result,
  Descriptions, Statistic, Stepper (Ant `Steps`), Tabs.
- Add the missing core components, mapped from Ant Design:
  - Navigation: `Breadcrumb`, `Pagination`.
  - Data Entry: `FormItem` (label, required mark, help, validation status), `Select`, `Checkbox`, `Radio`,
    `QuantityPicker` (Ant `InputNumber` with steppers, min/max), `Rate` (read-only + input).
  - Data Display: `Tag`, `Table` (columns, empty, loading, row key), `Timeline`, `Skeleton`, `Empty`,
    `Avatar`, `Image` (fixed aspect, fallback, lazy).
  - Feedback: `Alert`, `Spin`, `Drawer`, `Progress`; keep `ToastProvider` as Ant `message`.
- One shared state contract for every interactive component (default, hover, active, focus-visible,
  disabled, loading) and for data components (empty, error, loading).
- A component catalogue page `/dev/ui` (dev builds only) that renders every component in every state,
  used by unit tests and by visual review.
- Every component exported from `src/components/ui/index.ts`, Tier 3 tokens only (no raw values).

Repos: `team-frontend` only.

## Capabilities

### New Capabilities
- `ui-components`: the core component set, its state contract and its a11y rules.

### Modified Capabilities
None.

## Non-goals

- No `antd` dependency; no CSS-in-JS.
- Not every Ant component: no DatePicker, Upload, Tree, Cascader, Transfer, Calendar, charts (add them in a
  phase change when a screen needs them).
- No page adoption here; phases C–H migrate screens.
- No change to the tracking hooks (`TrackLink`, `data-*` attributes) on existing feature components.

## Impact

- `team-frontend/src/components/ui/*` (11 updated, ~18 new), `src/app/dev/ui/page.tsx`, unit tests per
  component.
- Depends on `ui-foundation` (tokens + green gate). Blocks the six phase changes.
