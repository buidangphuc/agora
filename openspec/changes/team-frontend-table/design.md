## Decisions

- Generic platform Table: `TableProps<T>` defaults `T` to `Record<string, ReactNode>`, so the template app and consumer example that pass `rows` keyed by column keep working; `render` is optional and the default cell is `row[key]`.
- No `className` on columns: the three uses (sticky first column, hide below a breakpoint, minimum width) become `sticky`, `hideBelow` and a min-width on the cell content, since the platform spacing scale has no 48/56/64 step for a token-backed `min-w`.
- Caption: the platform keeps `caption` mandatory and adds `captionHidden`; the facade always hides it, matching the old `sr-only` caption, so `get_by_role("table", name=...)` still resolves.
- Row hooks: only `data-testid` was used through `rowProps`, so the platform gains `rowTestId` instead of arbitrary row attributes; the facade keeps the `rowProps` name for call sites.
- Error state is owned by the platform (`role="alert"` plus a retry Button with `retryLabel`), so the facade no longer depends on the local ErrorState, Empty and Skeleton.
- Tailwind content: team-frontend did not scan `@fe-platform/tokens`, so classes that only the platform uses were never generated (hidden columns would stay hidden on desktop, the checkbox fill was missing). The config now scans `node_modules/@fe-platform/tokens/src/tokens.mjs`.

## Risks

- Tables inside cards lose their own border and radius and may look flatter.
- Other C3 branches may edit the same `tailwind.config.ts` content list.
