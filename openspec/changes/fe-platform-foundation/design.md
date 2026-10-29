## Context

See proposal.md for motivation. Current FE: single app `team-frontend` (Next 15, React 19, Tailwind 3.4, Biome 1.9, Vitest, `scripts/check-tokens.mjs`). Planning notes (Obsidian "AI-Ready Frontend Platform") target a greenfield workspace of `tokens`, `ui`, `api-client`, `config` packages plus an app template; decisions were settled 2026-10-29. This change delivers P0 (decisions, starting values, guidelines v0) and P1 (workspace, config, shell, single check command).

## Goals / Non-Goals

**Goals:** a workspace where `npm run check` is the single feedback loop for agents and CI; decisions recorded as ADRs; guidelines v0 an agent can read in one page.
**Non-Goals:** anything in P2-P6; touching `team-frontend`; publishing packages (workspace links only until P6).

## Decisions

1. **Repo placement: new sibling `fe-platform/`**, matching the polyrepo layout. Alternative: a folder inside `team-frontend` — rejected because the platform must not be coupled to one app.
2. **npm workspaces** (no pnpm/turbo). Matches `team-frontend`'s npm; avoids a new tool. Revisit if build times hurt.
3. **Biome as the single linter/formatter**, extended by repo scripts (token check, import-boundary check) instead of ESLint/Stylelint. Notes assumed ESLint; agora already standardises on Biome and one tool means one feedback command. Gap: no `jsx-a11y`/`no-restricted-imports` equivalent guaranteed in Biome → P1 pins the installed version, checks which Biome a11y rules exist, and adds ESLint only for rules Biome cannot express (recorded in the ADR).
4. **Check command order, cheapest first:** `biome check` → `tsc --noEmit` → repo scripts → `vitest run`. Mirrors CI exactly.
5. **API contract is Connect/protobuf, not OpenAPI.** The notes' `api-client` is generated from OpenAPI; the gateway speaks Connect-ES with buf stubs. P4 will generate from proto. Recorded as an ADR deviation now so P4 is not a surprise.
6. **Starting token values** are copied from `team-frontend` (`tailwind.config.ts`, `globals.css`; primary `#ee4d2d`) into the ADR with file references; authority moves to the new `tokens` package in P2.
7. **Reuse by copy, not import:** `check-tokens.mjs` (including the `tokens-allow: <reason>` escape hatch), vitest `server-only` alias, 3-tier token shape.
8. **Disable-comment accounting** is part of the guidelines now (require reason, no blanket disables); the CI counter lands with P1's check script.

## Risks / Trade-offs

- [Biome lacks an a11y or import-restriction rule the notes rely on] → verify against the installed version in P1; add minimal ESLint only for the gap.
- [Two token sources until P2 (team-frontend vs new)] → ADR names `fe-platform` as authority going forward; no sync is built.
- [Platform drifts from team-frontend since mapping is deferred] → accepted per user decision; revisit when mapping is planned.
- [Notes' rule/tool names unverified] → pin versions in P1, record in `docs/agents/project.md`.

## Migration Plan

None; additive new repo. Rollback: delete `fe-platform/` and the AGENTS.md line.

## Open Questions

- Git hosting/remote name for `fe-platform` (local-first per project preference; can be decided at first push).
