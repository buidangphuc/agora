## Why

P0 to P4 give an agent components, tokens, a client and checks, but the knowledge of how to use them is spread over source files, and nothing proves it yields coherent screens. An agent that must read every component file to learn the props will guess; written context that drifts from code is worse than none.

## What Changes

- A catalog export generated from code: `fe-platform/docs/agents/catalog.json` lists each `@fe-platform/ui` component (purpose, props with types, variants, sizes, client or server) and the token names (semantic colours, spacing, radius, font sizes). `scripts/catalog.mjs` writes it; `catalog:check` regenerates and fails on any diff, and joins `npm run check`.
- `fe-platform/AGENTS.md` and `docs/agents/frontend.md` point to the catalog; guidelines v0 are revised to v1 from what the experiment shows going wrong.
- An agent skill `.claude/skills/fe-platform/SKILL.md` (agora root) that routes an agent to the guide, catalog and check loop when it builds or edits `fe-platform` UI.
- The verification experiment: a fresh agent, given only the repository and a fixed brief, builds a five-screen flow (browse, product, cart, checkout, confirmation) on static data under `templates/app/app/(public)/shop/`. The brief is committed before the run; the result, with failures observed, is recorded in `docs/experiments/p5-five-screens.md`.
- A one-off metrics script compares the screens (button variants, radius, spacing and gap classes used) so "same radius, spacing and button hierarchy" is measured, not eyeballed.
- Storybook and Storybook MCP are out (user decision 2026-10-29).
- No change to `team-frontend`.

## Capabilities

### New Capabilities
none (agent tooling and documentation; no product behaviour)

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- New files: `scripts/catalog.mjs`, `docs/agents/catalog.json`, `docs/experiments/`, `.claude/skills/fe-platform/SKILL.md` (outside `fe-platform/`), shop routes and static data in `templates/app`.
- Root `npm run check` gains `catalog:check`; no new runtime dependency (the catalog script uses the `typescript` compiler already in devDependencies).
- The experiment spends one subagent run; the cost is accepted.

## Non-goals

Storybook, Storybook MCP, order or payment RPCs (cart and checkout use static view models), a control run without the written context, any claim that the context improves coherence beyond "this run met the criteria" (one run, no control), a design-tool sync, new components added just to make the flow easier (a missing component is recorded as a finding).

## Non-functional requirements

- `catalog.json` is deterministic: same source gives byte-identical output; sorted keys and lists.
- `npm run check` stays under 60 s locally.
- The experiment agent works in its own worktree so it cannot touch the main checkout.

## Data classification

| Data element | Class | Retention | Controls |
|---|---|---|---|
| Catalog, guidelines, skill text | public | n/a | none beyond repo access |
| Experiment screens and static product data | public | n/a | static fixtures only, no personal data |

No personal data handled.

## Threat model

none - no new trust boundary. The experiment agent runs in a worktree and its output passes the same gates and review before commit.
