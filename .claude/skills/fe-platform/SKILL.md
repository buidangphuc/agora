---
name: fe-platform
description: Build or edit UI in agora/fe-platform (shared ui package, tokens, api-client, Next template). Use for any screen, component, token or data-fetching change there, or "làm màn hình" in fe-platform. Not for team-frontend.
---

# fe-platform UI work

Rules live in the repo; this skill only routes. Do not copy rules here.

1. Read `fe-platform/AGENTS.md`, then `fe-platform/docs/agents/frontend.md`.
2. Look up components, props, variants and token names in `fe-platform/docs/agents/catalog.json`. A missing component or variant is added to `packages/ui` or `packages/tokens`, not styled around.
3. Copy a reference page: `templates/app/app/(public)/products` for data-backed pages, `templates/app/app/(internal)/dashboard` for forms and tables.
4. From `fe-platform/`, run `npm run check` until it exits 0 (steps and fixes in `docs/agents/project.md`). After changing a component or token run `npm run catalog`; after a proto change run `npm run proto -w packages/api-client`.
5. Changing `packages/tokens`, `ui` or `api-client`: add a changeset and follow `fe-platform/docs/governance/` (breaking and deprecation rules).
6. Never add a disable comment or edit generated code to get green.
