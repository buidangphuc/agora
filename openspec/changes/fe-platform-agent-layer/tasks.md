## 1. Catalog export

- [ ] 1.1 Add `scripts/catalog.mjs` and `npm run catalog`: parse `packages/ui/src/index.ts` with the TypeScript compiler API and write `docs/agents/catalog.json` (components with purpose, props, variants, sizes, client or server; token names); verify a vitest case checks the entry for every current component and that output is byte-identical on a second run
- [ ] 1.2 Fail generation when an exported component has no JSDoc purpose; verify with a fixture source lacking one
- [ ] 1.3 Add `catalog:check` (regenerate in memory, diff against the committed file, print the fix command) to `npm run check`; verify it goes red after adding a `variant` key in `packages/tokens` and after editing a prop type, and green after `npm run catalog`

## 2. Agent entry points

- [ ] 2.1 Point `fe-platform/AGENTS.md` and `docs/agents/frontend.md` at `docs/agents/catalog.json`; verify both link it and `docs/agents/project.md` lists `catalog:check` in the check order
- [ ] 2.2 Write `.claude/skills/fe-platform/SKILL.md` (English, trigger in the description, routes to the guide, catalog and `npm run check`, no rules of its own); verify the frontmatter parses and it is listed by the Skill tool

## 3. Five-screen experiment

- [ ] 3.1 Record the baseline (disable count, check and build state) and commit `docs/experiments/p5-brief.md` with the exact brief; verify the brief names the five screens, static data, and "use only the repository"
- [ ] 3.2 Run a fresh agent in its own worktree with only the brief; verify it produced the five routes under `templates/app/app/(public)/shop/` and note its stuck points as it reports them
- [ ] 3.3 Verify in that worktree: `npm run check` exits 0, template build passes, disable count equals the baseline; if any fails, record it and do not merge
- [ ] 3.4 Add `scripts/experiment-metrics.mjs` and run it on the five screens; verify it reports Button variants and radius, spacing and text classes per screen
- [ ] 3.5 View the five screens in the browser in light and dark; verify same radius, spacing and button hierarchy, listing any difference
- [ ] 3.6 Write `docs/experiments/p5-five-screens.md` (method, gates, metrics, screenshots or notes, every deviation and finding, verdict limited to the criteria); verify it states one run and no control

## 4. Guidelines v1 and done-condition

- [ ] 4.1 Revise `docs/agents/frontend.md` to v1, each new rule citing the finding behind it; verify it stays under one page
- [x] 4.2 Done-condition: `npm run check` exits 0 after a fresh `npm ci`, no new disable comments, `npm audit` clean, `openspec validate fe-platform-agent-layer --strict` passes
- [x] 4.3 e2e track: none (no user-facing capability, no FEATURES.yaml entry); confirm `repo-doctor` reports no new drift

## 5. Follow-up fixes from the experiment (requested after the run)

- [x] 5.1 Add `Link` to `packages/ui` with `text-link` tokens and contrast pairs in both themes; verify the contrast gate, axe state matrix and catalog entry, and replace `next/link` in the shop and products pages
- [x] 5.2 Replace the GET checkout with a server action that validates on the server and passes delivery details in an httpOnly cookie; verify unit tests (validation, redirect without personal data, empty cart) and a browser run through the flow
