# ADR-0001: Platform stack decisions

Status: accepted, 2026-10-29. Source: planning notes "Frontend Platform Open Decisions"; owner confirmed all rows.

| # | Decision | Chosen | Why | Reopen when |
|---|---|---|---|---|
| 1 | App framework | Next.js App Router | Public pages need indexing; one framework serves public and internal apps | A purely authenticated API-driven dashboard is the only consumer |
| 2 | Component distribution | Versioned packages (`tokens`, `ui`); copy-in only for app-specific parts | One source of truth, low drift, one component set for agents | Registry setup proves impractical |
| 3 | Accessible base | Headless primitives wrapped in own components | Uniform states and a11y without adopting a full library | A fitting library is already in use |
| 4 | Styling | Utility classes (Tailwind) with a token-driven preset; arbitrary values blocked by check | Tokens stay the only source of values | An existing codebase on another approach |
| 5 | Token source | Token file in code | No design file exists; makes contrast gate deterministic | A design file or team appears |
| 6 | Agent context delivery | Repository files first, Storybook MCP when stories are complete | Works with every agent at zero running cost | Queries Storybook cannot answer |
| 7 | Topology | One workspace (npm workspaces) for packages and template | Packages and template change together | Teams that cannot share a release cadence |
| 8 | Public/internal areas | One app, separate route groups; public routes must not import internal code | One shell, one set of conventions | Different deploy or auth boundaries |

Distribution beyond the workspace needs a private registry; it does not exist yet and is set up in P6. Until then packages link through the workspace.

Workspace tool: npm workspaces, matching `team-frontend`; no pnpm or turbo until build time hurts.
