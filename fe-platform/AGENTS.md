# fe-platform agent guide

AI-ready frontend platform template: shared packages plus a Next.js app shell, with conventions enforced by commands.

- Read `docs/agents/frontend.md` (guidelines) before writing UI, and `docs/agents/catalog.json` for every component (props, variants, sizes, client or server) and token name. The catalog is generated from code (`npm run catalog`); never edit it by hand.
- Run `npm run check` before reporting done; facts in `docs/agents/project.md`.
- Decisions: `docs/ADR/`. Changing a published package (`tokens`, `ui`, `api-client`): follow `docs/governance/` (changeset required, breaking and deprecation rules). Token authority: `packages/tokens/src/tokens.mjs`; origin of the starting values: `docs/tokens-starting-values.md`.
- Build screens from `@fe-platform/ui` components; do not hand-roll buttons, fields, dialogs, toasts or tables. Components take `variant`/`size`, never `className` or `style`. Client components (Select, Checkbox, Dialog, Toast) carry `"use client"`; add it to any file that uses hooks or Radix.
- Style only with token-backed Tailwind utilities (`bg-surface-card`, `text-text-primary`, `p-4`, `rounded-lg`); no hex, no `[...]` values. New colour pairs go in `packages/tokens/src/pairs.mjs`.
- Dependency direction: app, then features, then `packages/ui`, then `packages/tokens`; packages never import from an app; public routes never import internal code.
- Fetch data only through `@fe-platform/api-client` (server side; gateway :8080, `GATEWAY_URL`). App code never imports generated proto files, `@connectrpc/*` or `@bufbuild/*`; client components never import the api-client or `lib/session`. Failures arrive as `ApiError` (`kind`). Copy `app/(public)/products` for a data-backed page.
- After a backend proto change run `npm run proto -w packages/api-client` and commit `src/generated`; `npm run proto:check` fails on any diff.
- No disable comments without a reason; no hand edits to generated code.
