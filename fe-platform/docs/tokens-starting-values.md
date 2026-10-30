# Starting token values (P0)

Authority moves to `packages/tokens` in P2. Until then these values are copied, with no sync, from `team-frontend`.

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

## Known issue for the P2 contrast gate

Text-inverse (white) on action-primary (`#ee4d2d`) computes to about 3.66:1, below the 4.5:1 AA threshold for normal text; it passes only as large text (3:1). P2 must either darken the action token for text use (primary 600 `#d73211` is darker) or declare that pairing as large-text only.
