# Starting token values (P0)

Historical since ADR-0005 (2026-10-30): the palette, radii and neutral scale below were replaced and no longer describe the package. Authority is `packages/tokens` since P2. This page records where the starting values came from; it is not kept in sync, and the package wins on any difference.

Source files: `team-frontend/tailwind.config.ts` (Tier 1 primitives) and `team-frontend/src/app/globals.css` (Tier 2 aliases).

## Palette

| Token | Value | Source |
|---|---|---|
| primary 50 / 100 / 200 / 300 / 400 | `#fff5f2` / `#ffe8e1` / `#ffd4c7` / `#ffb5a0` / `#ff8566` | tailwind.config.ts `primary` |
| primary 500 (default) | `#ee4d2d` | same |
| primary 600 / 700 / 800 / 900 / 950 | `#d73211` / `#b52309` / `#941e0c` / `#7a1d0f` / `#430b05` | same |
| neutral 50 to 900 | `#f9fafb #f3f4f6 #e5e7eb #d1d5db #9ca3af #6b7280 #4b5563 #374151 #1f2937 #111827` | `neutral` (Tailwind gray) |
| danger / danger-dark | `#d0011b` / `#b00016` | `accent` |
| promo / promo-dark | `#ffbe00` / `#8a5a00` | `accent` |
| success / success-dark | `#00bfa5` / `#007a69` | `accent` |

Semantic aliases (light): action-primary = primary 500, action-primary-hover = primary 600, surface-page = neutral 100, surface-card = white, surface-muted = neutral 50, border-subtle = neutral 200, border-strong = neutral 300, text-primary = neutral 900, text-secondary = neutral 600, text-disabled = neutral 400, text-inverse = white, focus-ring = primary 300. Source: globals.css `:root`. No dark theme exists in the source; P2 defines it.

## Type scale

12 / 14 / 16 / 20 / 24 px with line heights 16 / 20 / 24 / 28 / 32 px; display sizes 30 and 36 px for hero and page titles only. Body 14 px. Font stack starts with Inter, then system fonts. Source: `fontSize`, `fontFamily`.

## Spacing and radius

Spacing: Tailwind 4 px base step, plus 18 (72 px). Radius: xs 2 px (hairline chips), lg 8 px (controls), xl 12 px (cards), 2xl 16 px (modal, hero). Source: `spacing`, `borderRadius`.

## Changes made in P2

- White on `#ee4d2d` computes to about 3.66:1, below the 4.5:1 AA threshold. `action-primary` is primary 600 `#d73211` (about 4.8:1 with white); `action-primary-hover` is primary 700. Primary 500 stays a primitive and is not exposed as a utility.
- Focus ring primary 300 was about 1.7:1 on white, below the 3:1 non-text threshold. `focus-ring` is primary 600 in light and primary 400 in dark.
- New primitive `danger-light` (`#ff8a98`) for danger text in dark; new semantic names `text-danger`, `text-success`, `text-warning`.
- Dark theme is defined in `packages/tokens/src/tokens.mjs`; every declared pair is checked in both themes.
