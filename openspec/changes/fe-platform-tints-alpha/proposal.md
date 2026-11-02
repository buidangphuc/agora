## Why

P10 left three pieces of colour debt in the apps. The platform has no subtle danger, success or warning surfaces, so `bg-red-50`, `bg-emerald-50` and `bg-yellow-50` tints stay raw. Semantic colours are plain `var(--color-*)` values, so `/alpha` modifiers do not work on them and translucent white or black stays raw. The platform Button widens while it shows its spinner, so two forms wrap their submit button in a fixed `w-44` div.

## What Changes

- `@fe-platform/tokens`:
  - New primitives for subtle danger, success and warning fills and borders.
  - Six new semantic colours, each with a light and a dark value: `surface-danger`, `surface-success`, `surface-warning`, `border-danger-subtle`, `border-success-subtle`, `border-warning-subtle`.
  - New contrast pairs, so status text on a status surface and primary text on a status surface must pass AA in both themes.
- `@fe-platform/tokens`, alpha support:
  - New helper `colorVar(name)`. It returns `color-mix(in srgb, var(--color-<name>) calc(<alpha-value> * 100%), transparent)`.
  - The preset uses `colorVar` for every semantic colour, so `bg-surface-card/80` works.
  - The `--color-*` variables stay hex, so the change is non-breaking.
- `@fe-platform/ui` Button, while `loading`:
  - Keeps its width. Label and icons are hidden with `opacity-0`, so the accessible name stays, and the spinner is centred over them.
  - `aria-busy` is unchanged.
- The buyer, seller and admin apps:
  - Tailwind configs use `colorVar` for their colour aliases.
  - Tarballs are revendored.
  - The two `w-44` button wrappers are removed.
- Buyer app class rewrites on light surfaces:
  - Raw `red`, `emerald`, `yellow`, `amber` and `slate` tint classes become the new semantic names.
  - White or black with alpha becomes a semantic name with alpha.
  - `tones.ts` soft tones use the new surfaces.
- The buyer `palette.test.ts` gate now also bans raw tint scales and white or black with an alpha modifier. Two kinds of code are exempt:
  - Gradient lines.
  - The intentionally dark or warm blocks in `StudentRentalView` and `LiveFlashSaleStock`.

## Capabilities

### New Capabilities
none (tokens, a component state and class names)

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `fe-platform/packages/tokens` (src, dist, types), `packages/ui/src/Button.tsx`, catalog, changesets (tokens minor, ui patch).
- `team-frontend{,-seller,-admin}`: tailwind config, vendor tarballs, lockfile integrity. Buyer: about 12 feature and shell files plus the palette gate.

## Non-goals

- Gradient heroes, dark hero blocks and the flash-sale bar; they stay raw.
- A `text-muted` token. No call site needs one, and neutral-500 fails AA on `surface-muted`.
- A dark theme for the apps.
- Switching Alert and Toast to tinted surfaces.

## Non-functional requirements

- The contrast gate stays green in light and dark, including the new pairs.
- A loading Button has the same rendered width as the idle Button.
- `npm run check` stays green in fe-platform and in all three apps, and `next build` passes in all three.
