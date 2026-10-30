# ADR-0005: Own visual direction, platform-neutral tokens

Status: accepted, 2026-10-30 (direction chosen by the user: indigo, airy, large radii).

## Decision

- The look is indigo primary, near-white page, thin borders, soft shadows (`shadow-card`, `shadow-pop`), controls at 12 px radius, cards and dialogs at 16 px, body text 16 px. The orange palette copied from `team-frontend` in P0 is dropped; `docs/tokens-starting-values.md` stays as history only.
- Token data holds raw numbers (space, radius, font size, breakpoints, touch target) and structured shadows (`x, y, blur, spread, color`). CSS and Tailwind builders add units. `dist/tokens.json` is the resolved, platform-neutral form for any future non-CSS consumer; a test keeps it equal to `dist/tokens.css`.
- Web and mobile web (WAP) are one codebase: mobile-first layouts, shared breakpoints (`sm 640`, `md 768`, `lg 1024`), every interactive control at least 44 px tall (`min-h-touch`), safe-area utilities (`pt-safe`, `pb-safe`), viewport `viewportFit: cover`.
- A native app shares tokens (`tokens.json`) and the backend contract (buf proto), not components. `packages/ui` runs on the DOM only. Native builders and a native component package are a later phase and need the app technology chosen first (open).

## Why

- A template that copies one marketplace's palette makes every generated screen look like that marketplace.
- Unit strings and CSS-shaped shadows in the source data would force a rewrite when a second platform appears; numbers and structured values cost nothing now.
- Radix and Tailwind cannot render natively, so promising shared components across web and native would be false; the shared layer that does hold is tokens plus contract.

## Consequences

- `tokens` takes a major version: primitives keep their names but move from orange to indigo, which the versioning policy treats as breaking. No class or token name changes, so the migration note is "appearance only".
- The catalog marks each component with `platforms` (`web`, `wap`); a native phase adds `native` without changing the shape.
