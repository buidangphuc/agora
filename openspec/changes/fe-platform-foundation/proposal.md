## Why

Agora's frontend has one app (`team-frontend`) with its tokens, UI kit, gateway auth and the single `check-tokens.mjs` gate all app-local, no FE job in CI, stale docs (Next 14 vs actual 15) and no FE guidance or skill for agents. AI agents generating UI therefore drift in radius, spacing and component choice. We want a reusable, machine-enforced FE platform so agent-generated UI is coherent by construction. This change lays the foundation (decisions + empty enforced workspace); later changes add tokens, UI, client, agent layer and governance.

## What Changes

- New sibling repo `fe-platform/` (npm workspaces): `packages/config`, `templates/app` (Next.js App Router shell, strict TypeScript), Biome, one CI command (`npm run check`) that fails on a deliberate lint error.
- ADRs recording the settled decisions (Next.js, packages, wrapped headless primitives, Tailwind + token preset, tokens in code, repo files then Storybook MCP, one workspace, one app with public/internal route groups) plus the Biome-over-ESLint and Connect/proto-over-OpenAPI deviations from the planning notes.
- Starting token values (palette, type scale, spacing step, radius) written down with their source (team-frontend).
- Guidelines v0 (`docs/agents/frontend.md`) and `docs/agents/project.md` listing the feedback-loop commands.
- No change to `team-frontend`; no mapping into it in this change.

## Capabilities

### New Capabilities
none (tooling and docs only; no user-facing or service behaviour)

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- New repo directory `fe-platform/`; new CI job for it. Root `AGENTS.md` repo map gains one line.
- No proto, gateway or service changes. No dependency on other repos at runtime.
- Follow-up changes (not in scope here): P2 tokens, P3 ui, P4 api-client + shell + reference feature, P5 agent layer + 5-screen experiment, P6 governance + registry.

## Non-goals

Mapping or migrating `team-frontend`; tokens, components, api-client (later changes); micro-frontends; themes beyond light/dark; design-tool sync.

## Non-functional requirements

- `npm run check` on the empty shell completes in under 60 s locally.
- Check is deterministic and identical locally and in CI.

## Data classification

| Data element | Class | Retention | Controls |
|---|---|---|---|
| Source, config, docs | public | n/a | none beyond repo access |

No personal data handled.

## Threat model

none - no new trust boundary
