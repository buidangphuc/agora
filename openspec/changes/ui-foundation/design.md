## Context

The design doc already chose a 3-tier token model and Tailwind. Ant Design 5 uses the same idea
(Seed token → derived Map tokens → Alias tokens → Component tokens) through CSS-in-JS. We keep Tailwind and
express the tiers as Tailwind config (Tier 1) + CSS variables (Tier 2) + component-local class maps
(Tier 3). The WIP already in the tree (12 primitives, `primary`/`surface` scales, `preline-*` shadows) is
the starting point.

## Goals / Non-Goals

Goals: one place per tier, a lint that keeps it that way, a green gate, Exception/loading shells.
Non-goals: dark mode, runtime theming UI, `antd` dependency, page redesign.

## Decisions

1. **Ant Design mapping.**

   | Ant Design | agora |
   |---|---|
   | Seed token (`colorPrimary`, `borderRadius`, `fontSize`, `sizeUnit`) | Tier 1 in `tailwind.config.ts` |
   | Map token (`colorPrimaryBg`, `colorPrimaryHover`, ...) | Tier 1 scales 50–950 (pre-derived, not computed) |
   | Alias token (`colorText`, `colorBgContainer`, `colorBorderSecondary`, ...) | Tier 2 CSS variables |
   | Component token (`Button.primaryColor`, ...) | Tier 3 class maps inside `src/components/ui/*` |
   | Layout `Grid` 24 columns / `Space` | Tailwind grid + `gap-*` on the 4px scale |
   | Exception 403/404/500 | `not-found.tsx`, `error.tsx` |

2. **CSS variables for Tier 2** (`--color-text-primary: theme(colors.neutral.900)` etc.), mapped in Tailwind
   as `colors: { "text-primary": "var(--color-text-primary)" }`. This keeps a later dark mode to one block.
3. **Token lint is a plain Node script** (no new dependency): regexes for `#[0-9a-f]{3,8}\b`, `rgba?\(`,
   `hsla?\(`, `\b[\w:-]+-\[[^\]]+\]`, and `style={{` with a colour key. It honours `// tokens-allow:`.
4. **Fixing the gate first**: run `biome check --write` (safe fixes), then fix the 11 `tsc` errors against
   the real types (no `any`, no `@ts-ignore`) and the 5 Vitest failures; if a test asserts stale behaviour,
   fix the test only when the current behaviour is the intended one, and say so in the commit.
5. **Violation clean-up** maps each arbitrary value to the nearest token. Where a value has no token and is
   genuinely needed (e.g. a fixed hero aspect), add it to Tier 1 instead of allow-listing.

## Risks / Trade-offs

- Mapping 173 values to the nearest token causes small visual shifts. Mitigation: screenshot the top 6
  offending pages before and after and review.
- The lint regex can flag text that is not a class (e.g. hex in copy). Mitigation: `tokens-allow` comment.

## Open Questions

None blocking.
