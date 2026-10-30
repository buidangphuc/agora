## Context

B1 made `gray` equal to `neutral` and recoloured the legacy names, so the app already renders in the new palette. What is left is vocabulary: raw scale names carry no meaning and cannot follow a theme. The platform semantic names (class forms `text-text-primary`, `bg-surface-card`, `border-border-subtle`, `bg-action-primary`, ...) resolve to CSS variables written by the B1 plugin.

## Decisions

1. **Mapping table, applied by a scripted one-shot codemod** (kept out of the repo, the table below is the record). Light-theme values are compared by hex.

| From | To | Delta |
|---|---|---|
| `text-white` | `text-text-inverse` | none |
| `bg-white` | `bg-surface-card` | none |
| `text-brand`, `border-brand`, `bg-brand` | `text-action-primary`, `border-action-primary`, `bg-action-primary` | none |
| `bg-brand-dark` | `bg-action-primary-hover` | none |
| `brand/N`, `from-brand` | `primary-600/N`, `from-primary-600` | none |
| `text-gray-900`, `text-slate-900` | `text-text-primary` | none |
| `text-gray-800`, `text-slate-800` | `text-text-primary` | `#1e293b` to `#0f172a` |
| `text-gray-600` | `text-text-secondary` | none |
| `text-gray-700` | `text-text-secondary` | `#334155` to `#475569` |
| `text-gray-500` | `text-text-secondary` | `#64748b` to `#475569` (darker, passes AA on `surface-page`/`surface-muted` where `#64748b` is 4.3 to 4.5) |
| `text-gray-400`, `placeholder-gray-400` | `text-text-disabled`, `placeholder-text-disabled` | none |
| `border-gray-200`, `border-slate-200` | `border-border-subtle` | none |
| `border-gray-100` | `border-border-subtle` | `#f1f5f9` to `#e2e8f0` |
| `border-gray-300`, `border-slate-300` | `border-border-strong` | none |
| `bg-gray-50`, `bg-slate-50` | `bg-surface-page` | none |
| `bg-gray-100` | `bg-surface-muted` | none |
| `indigo-N` | `primary-N` | none (same scale) |
| `orange-N` | `primary-N` | orange becomes indigo (intended: chat, assistant, quick buttons) |
| `text-red-700` | `text-text-danger` | none |
| `text-red-600` | `text-text-danger` | `#dc2626` to `#b91c1c` |
| `border-red-500/600/700`, `ring-red-600` | `border-border-danger`, `ring-border-danger` | `500`, `600` slightly darker |
| `bg-red-600` | `bg-danger` | none |
| `bg-red-700` | `bg-accent-danger-dark` | none |
| `text-emerald-700` | `text-text-success` | none |
| `text-emerald-600` | `text-text-success` | `#059669` to `#047857` |
| `bg-emerald-500`, `border-emerald-500` | `bg-success`, `border-success` | none |
| `bg-emerald-600` | `bg-accent-success-dark` | `#059669` to `#047857` (white text now passes AA) |
| `shadow-shopee`, `shadow-orange-500/N` | `shadow-card`, `shadow-primary-500/N` | softer card shadow |
| other `gray-N` | `neutral-N` | none |

2. **Left on purpose** (listed so nothing is silently skipped): dark-surface strings and the cockpit; `slate-*` text/border on dark surfaces; `bg-neutral-200` skeleton pulses; light tints with no semantic name (`bg-red-50`, `bg-emerald-50`, `border-emerald-200`, amber and yellow warning tints, the flash-sale stock bar in orange/amber); anything with an opacity modifier on a semantic name; `bg-black/N` scrims; `fill-white`, `ring-white`, `border-white`; the student rental and AI gradients that mix purple, blue, teal, cyan. Follow-ups: tint tokens (`surface-success`, `surface-danger`, `surface-warning`), an alpha-capable colour form, a `surface-overlay` scrim use, and the dark design.
3. **Dark detection** is per string literal: a literal containing `bg|from|via|to-(slate|gray|neutral)-(800|900|950)` is skipped, plus `CockpitView.tsx` as a whole. `slate` is treated as the dark-console family and only its light steps convert.
4. **Gate:** `src/app/palette.test.ts` scans non-test source for the legacy and raw names the codemod removed; the flash-sale bar and cockpit may keep orange.
5. **Verification:** vitest, biome, `check-tokens`, `next build`, a Tailwind build probe is unnecessary (every target name already exists and B1's test asserts the aliases); before/after computed styles on the main pages in the browser, and the B1 e2e colour scenarios.

## Risks / Trade-offs

- [A converted class does not exist in the Tailwind config, so the rule silently disappears] -> all targets are existing names (`tokens.test.ts` asserts every platform semantic name is a colour); the browser check reads computed colours of converted elements.
- [Muted copy loses hierarchy because 500, 600 and 700 collapse to `text-secondary`] -> accepted for now; a `text-muted` token needs a platform change and contrast pairs, recorded as a follow-up.
- [Dark literals that sit inside a light parent stay raw] -> visible in the leftover list; moves with the dark design.
