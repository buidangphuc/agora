## Decisions

User choices (2026-11-02):

- **Spinner as an overlay.** The Button base gains `relative`. The content always sits in one inline-flex span; while loading that span gets `opacity-0` and the spinner sits in an `absolute inset-0 flex items-center justify-center` span. `opacity-0`, not `invisible`: `visibility: hidden` drops the label from the accessible name. The width is the idle width.
- **Alpha via `color-mix`, not RGB channels.** Tailwind 3 replaces `<alpha-value>` in any colour string, with `1`, the opacity variable or the modifier. `color-mix` keeps `--color-*` as hex, so `dist/tokens.css` and anyone reading the variables are unaffected; `tokens` takes a minor bump. The cost is support for `color-mix`, which is Baseline 2023.
- **Leftovers: tints and alpha only.** Gradients and dark hero blocks stay raw.
- **No `text-muted`.**

## Token values

| Semantic | Light | Dark |
|---|---|---|
| surface-danger | danger-subtle `#fef2f2` | danger-deep `#450a0a` |
| surface-success | success-subtle `#ecfdf5` | success-deep `#022c22` |
| surface-warning | promo-subtle `#fffbeb` | promo-deep `#451a03` |
| border-danger-subtle | danger-light `#fca5a5` | danger-darker `#991b1b` |
| border-success-subtle | success-light `#a7f3d0` | success-dark `#047857` |
| border-warning-subtle | promo-light `#fde68a` | promo-dark `#92400e` |

## Class mapping (buyer app, light surfaces)

| Raw | Semantic |
|---|---|
| `bg-red-50`, `bg-red-50/50`, `bg-red-100` | `bg-surface-danger` |
| `border-red-200`, `border-red-300` | `border-border-danger-subtle` |
| `bg-emerald-50` | `bg-surface-success` |
| `border-emerald-200` | `border-border-success-subtle` |
| `bg-yellow-50`, `bg-amber-50`, `hover:bg-yellow-100` | `bg-surface-warning` |
| `bg-amber-200/90` (badge) | `bg-surface-warning` with `border-border-warning-subtle` |
| `border-yellow-200`, `border-amber-200` | `border-border-warning-subtle` |
| `text-yellow-700`, `text-yellow-800`, `text-amber-600`, `text-amber-950` | `text-text-warning` |
| `text-slate-700` | `text-text-secondary` |
| `text-slate-400` | `text-text-disabled` |
| `bg-slate-50/50` | `bg-surface-page/50` |
| `text-white/N`, `bg-white/N`, `border-white/N` on primary or dark surfaces | `text-inverse/N` with the same property |
| `hover:bg-white/80` on light lists | `hover:bg-surface-card/80` |
| `bg-black/50` modal backdrop | `bg-surface-overlay` |
| `bg-black/N` on images | `bg-text-primary/N` |
| `bg-red-500`, `bg-red-600/90` | `bg-action-danger` |
| `bg-yellow-300`, `bg-yellow-400` (notification dot) | `bg-promo` |
| tones soft `bg-accent-*/10` etc. | `bg-surface-*`, `text-text-*`, `border-border-*-subtle` |

## Left raw on purpose

- Lines with `bg-gradient-*` and their `from`, `via` and `to` stops.
- `StudentRentalView`, which has a dark emerald hero and price block.
- `LiveFlashSaleStock`, which uses the warm palette.
- The LoanCalculator dark hero.
- The `primary-N` and `neutral-N` primitive scales with alpha. These are hex values, so alpha already worked on them.

## Risks

- Text that was `amber-950` or `yellow-800` on a pale yellow fill becomes `promo-dark` (`#92400e`). This is lighter, but still AA on `promo-subtle`, which the new pair checks.
- `bg-text-primary/N` is a text colour used as a fill; the alternative is a new overlay token. It is accepted for one call site.
