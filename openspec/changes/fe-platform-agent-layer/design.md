## Context

See proposal.md. After P4 an agent has `AGENTS.md`, `docs/agents/frontend.md` (v0), `docs/agents/project.md`, token and UI sources, and gates that block raw values, arbitrary Tailwind values, boundary breaks and generated-code edits. Missing: one machine-readable description of components and tokens kept in sync by CI, an entry point for agents outside the repo docs, and evidence. The planning notes state the coherence claim comes from practitioner articles, so P5's experiment is the project's own evidence.

## Goals / Non-Goals

**Goals:** a catalog that cannot drift; an agent route into the repo; a recorded five-screen run with measured coherence; guidelines revised from observed failures.
**Non-Goals:** Storybook, control run, order RPCs, new components.

## Decisions

1. **Catalog from code with the TypeScript compiler API.** `scripts/catalog.mjs` parses `packages/ui/src/index.ts`, resolves each exported component and its `*Props` type, and reads the JSDoc first sentence as purpose, prop names, optionality and type text, the literal keys of `variant`/`size` from the token class maps, and whether the file starts with `"use client"`. Token names come from `@fe-platform/tokens` (semantic colour names, space, radius, fontSize keys). Alternative: hand-written markdown — rejected, it drifts. Alternative: react-docgen — rejected, new dependency for little gain.
2. **Deterministic output.** Keys and lists sorted, two-space JSON, trailing newline, no timestamps or absolute paths. `catalog:check` writes to memory and compares with the committed file, mirroring `proto:check`; the fix command is `npm run catalog`.
3. **Purpose is required.** An exported component without a JSDoc purpose makes generation fail, so the catalog never has empty entries.
4. **Skill lives at agora root** (`.claude/skills/fe-platform/SKILL.md`) because skills are discovered there, not inside `fe-platform/`. It is short and English, routes to `AGENTS.md`, `docs/agents/frontend.md`, `catalog.json` and `npm run check`, and holds no rules of its own, so there is one source of truth. Its description names the trigger (building or editing `fe-platform` UI).
5. **Experiment protocol.** (a) Commit `docs/experiments/p5-brief.md`: the exact task, flow, static-data requirement, and "use only the repository". (b) Run a fresh subagent in a git worktree with only the brief as its prompt. (c) Verify gates in that worktree: `npm run check`, template build, `scripts/check-disables.mjs` count unchanged from the baseline recorded in the brief. (d) Run the metrics script. (e) Look at the screens in the browser, light and dark. (f) Write the result file, including every deviation, stuck point and rule the agent broke or nearly broke. (g) Merge the screens only if gates pass; otherwise the record says so and nothing is merged.
6. **Metrics.** `scripts/experiment-metrics.mjs` reads the five screen files and reports, per screen: Button variants used, `rounded-*`, spacing (`p-*`, `m-*`, `gap-*`, `space-*`) and `text-*` classes. Pass criteria: all radius classes drawn from the token set (already enforced), one primary Button per screen, and the sets of spacing and radius classes identical in kind across screens (differences listed, judged in the record). It is a measuring tool, not a permanent gate.
7. **Static data.** Products, cart lines and order summary come from a `lib/shop-data.ts` file with typed fixtures; the flow keeps cart state in the URL or a server-side constant, not a client store, so no new client-state library is needed. The brief says so.
8. **Guidelines v1.** `docs/agents/frontend.md` is edited from the failures recorded in the result file; each added rule cites the finding that caused it. If the run shows no failures, v1 equals v0 plus the catalog pointer, and the record says that.

## Risks / Trade-offs

- [One run, no control, so no causal claim] → stated in the record and non-goals; the result reads "met / did not meet the criteria".
- [A subagent may read this change's artifacts and learn the experiment] → the brief tells it to read only `fe-platform/`; the record notes whether it did.
- [Catalog parser breaks on unusual prop types] → type text is taken as printed by the compiler; a test covers each current component; unsupported shapes fail generation loudly.
- [Experiment output is low quality] → recorded as a finding; the gates still decide what is merged.
- [Skill outside `fe-platform/`] → one file, no logic, points back into the repo.

## Migration Plan

Additive. Rollback: remove the catalog script and file, the check step, the skill, the experiment folder and the shop routes.

## Open Questions

none (Storybook out and static data confirmed by the user 2026-10-29).
