# fe-platform agent guide

AI-ready frontend platform template: shared packages plus a Next.js app shell, with conventions enforced by commands.

- Read `docs/agents/frontend.md` (guidelines) before writing UI.
- Run `npm run check` before reporting done; facts in `docs/agents/project.md`.
- Decisions: `docs/ADR/`. Starting token values: `docs/tokens-starting-values.md`.
- Dependency direction: app, then features, then `packages/ui`, then `packages/tokens`; packages never import from an app; public routes never import internal code.
- No disable comments without a reason; no hand edits to generated code.
