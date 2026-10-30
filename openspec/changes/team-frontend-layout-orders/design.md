## Decisions

- Facades only: every surface is the `@/components/ui` `Card`, `Link`, `Button`, `Table`, `Descriptions` and `Stepper`; layout uses tailwind with semantic token classes, so the palette and check-tokens gates stay green.
- Mobile first: one column below `lg`; actions stack full width (`fullWidth="mobile"`) and every control keeps the platform `min-h-touch` (44 px).
- Detail grid: items and amounts take two thirds on `lg`, the order information (`Descriptions column={1}`) takes the last third; the timeline and returns tabs stay full width below.
- Long strings (address, tracking code) wrap with `break-words`/`break-all` so a 375 px viewport never scrolls horizontally.
- Loading skeletons keep the footprint of the new layout.

## Risks

- The detail is wider on desktop (`max-w-5xl` instead of `max-w-4xl`).
- Visual review covered only the empty and error states, because the gateway was not running during the check.
