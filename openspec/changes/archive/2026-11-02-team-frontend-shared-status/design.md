## Decisions

- Tones use existing semantic colours: the platform has no tint tokens, so coloured tones are `bg-surface-card` with `text-text-<tone>` and `border-current`; neutral and info keep muted surfaces. Every pair is already in the contrast list.
- `ClosableTag` is a separate client export so `Tag` stays server-safe, matching the team-frontend split between `Tag` and `TagClient`. Its close button is `min-h-touch` with negative vertical margin, so the hit area is 44 px while the chip keeps its height.
- Badge solid fills reuse `action-primary`, `action-danger` and the `text-*` colours as backgrounds with `text-inverse`; `mall` maps to solid danger and `discount` to solid warning because the platform has no promo colour.
- The platform keeps English default status labels; the facade passes Vietnamese ones through `statusLabel`, so accessible names in tests and e2e are unchanged.
- `whitespace-nowrap` and `shrink-0` move into the tag and badge base classes because every `className` on Tag and Badge was one of those two.
- The catalog test compares Button union members as sets: TypeScript orders union members by creation, and the new Badge tone type creates `"danger"` before Button is read.
- team-frontend tailwind did not scan platform class maps, so classes used only by platform components (for example `h-18`, `aria-checked:bg-action-primary`) were missing from the CSS; the content list now includes the vendored tokens and ui sources.

## Risks

- Colour change on tags and badges (no tint) is visible in order, verification and admin screens.
- The closable tag's negative margin can overlap a neighbour's hit area in tight rows.
