## Decisions

- Error and empty states stay in team-frontend. The platform has no Alert or Empty yet, and Alert belongs to another branch, so `Descriptions` and `Statistic` take a generic `fallback` node. The team facade fills it with `ErrorState` or `Empty`. If the caller passes no fallback, platform `Descriptions` shows `emptyText` as plain muted text.
- The platform components take no `className`. Call sites that set a width or flex on `Skeleton` or `Progress` now set it on a wrapper `div`, as C1b did for Card. KpiRow's `border-danger` class sat on a wrapper with no border, so it had no visible effect. It becomes a semantic `tone="danger"` that colours the value.
- Spacing preset: it has no `0.5`, `1.5`, `2.5` or `14` steps, so the progress heights become `h-2`/`h-3`, the large avatar becomes `h-12`, and the Statistic trend row margin becomes `mt-3`.
- Aspect ratios `4/3`, `3/4` and `2/1` are added to the token preset through `extend`, so the template's tailwind-content test can generate `skeleton.aspect`. team-frontend already defines these ratios itself.
- Progress fill width stays an inline `style` inside the component. This is the only way to set a dynamic width without arbitrary values, and the ban covers only the `style` prop.
- team-frontend's Tailwind `content` scanned only `src`, so a platform class that no team source file used generated no CSS. For example, `sm:w-1/3` went missing after Descriptions moved to the platform. The config now also scans the vendored `@fe-platform/tokens/src/tokens.mjs`.
- Statistic sets `aria-busy` on its own root wrapper, because platform `Card` overwrites a passed `aria-busy`.

## Risks

- Branches that run in parallel edit `index.ts`, `states.tsx`, `catalog.test.mjs` and `tokens.mjs` too, so merging them causes textual conflicts.
