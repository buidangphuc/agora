# P5 experiment: five-screen shopping flow

Date: 2026-10-29. One run, no control run (no agent without the written context), so this record supports "met / did not meet the criteria" only, not a claim that the written context improves coherence.

Note (2026-10-30): measured on the previous orange, dense look; P7 (ADR-0005) redesigned these screens and the numbers below are not re-measured.

## Method

- Brief: `docs/experiments/p5-brief.md`, committed before the run (baseline: `npm run check` green with 110 tests, build green, 7 disable comments).
- A fresh general-purpose subagent ran in a separate git worktree of the repository with the brief as its prompt, plus one logistics line (work only inside the worktree path, do not commit).
- The agent had the repository's own agent files available (`AGENTS.md`, `docs/agents/`, the `fe-platform` skill). Its report cites `docs/agents/frontend.md`, so it read the guidelines. It ran `npm run check` and the build; it never opened a browser.
- I verified independently in that worktree, did not rely on the agent's report: gates, disable count, build, metrics, then viewed the screens in a real browser in light and dark.

## Gate results (verified)

| Check | Result |
|---|---|
| `npm run check` | exit 0, 110 tests (unchanged), boundaries 0, tokens 0, proto 0, catalog 0 |
| Disable comments | 7, same as baseline, 0 violations |
| `npm run build -w templates/app` | passes; `/shop`, `/shop/[id]`, `/shop/cart`, `/shop/checkout`, `/shop/confirmation` built |
| New dependencies | none |

## Coherence metrics (`scripts/experiment-metrics.mjs`)

| Screen | Buttons (primary) | Spacing classes | Text sizes | Radius classes |
|---|---|---|---|---|
| browse | 0 | gap-4 p-6 | 2xl | none |
| product | 1 (1) | gap-3 gap-4 p-6 | 2xl | none |
| cart | 0 | gap-4 p-6 | 2xl xl | none |
| checkout | 1 (1) | gap-3 gap-4 p-6 | 2xl | none |
| confirmation | 0 | gap-4 p-6 | 2xl xl | none |

Union: spacing `gap-3 gap-4 p-6`, text `text-2xl text-xl`, no screen carries a radius class (radius comes from components). No screen has more than one primary Button. gap-3 appears only inside form-like blocks.

## Visual check (browser, light and dark)

Same page wrapper, heading size, table and field styling on all five screens in both themes; the primary button is the same colour and size on product and checkout. Differences: none in radius or spacing beyond the gap-3 inside forms.

## Findings

1. **Links look like plain text.** All "continue" and navigation actions are `next/link` links. In the browser a link's colour equals the body text colour and has no underline, so the way forward on cart and confirmation is not recognisable as actionable. Cause: the guidelines say navigation is a link, but `packages/ui` has no Link component and the template has no link style. Not fixed here (a new component is a non-goal); logged as a gap.
2. **"Page container" does not exist.** The guidelines named it; the agent copied `main flex flex-col gap-4 p-6` from the products pages. Fixed in guidelines v1 by naming the actual classes.
3. **Personal data in the URL.** Checkout is a GET form, so name, address and phone land in the query string and in `/shop/confirmation`. The agent flagged this itself. The guidelines were silent on forms. Rule added in v1. The shop screens are kept as produced, as the experiment's evidence, and are demo-only.
4. **Form participation of Select and Checkbox is undocumented.** The agent avoided both because their behaviour in a plain form submit is not stated. Not changed (unverified); open question for a later form pattern.
5. **No validation pattern.** Native `required` only; no server-side validation example in the repository.
6. **No Card, quantity control or summary component.** The agent composed Table plus paragraphs. Recorded; no component added.
7. **Hidden field.** `Input` requires a visible label, so a raw `<input type="hidden">` was used; the Biome a11y rules did not object.

Nothing the agent did broke a gate or needed a disable comment.

## Verdict against the criteria

- Every component and token from the shared packages: yes (Table, Input, Button; token-backed classes only).
- All gates green, no new disable comments: yes.
- Same radius, spacing, button hierarchy side by side: yes, measured and viewed.

Limits: one run, one task, no control, the agent could read the written context, and the flow is small. Guidelines v1 changes come only from the findings above.

## Fixes after the run (2026-10-29)

Applied on request after the record above; the metrics and verdict describe the unmodified run.

- Finding 1: added `Link` to `packages/ui` (wraps `next/link`, `next` is now a peer dependency), with `text-link` and `text-link-hover` tokens in both themes. The contrast gate rejected the first dark value (4.32:1 on `surface-muted`); dark uses primary-300. Every `next/link` in the shop and products pages now uses `Link`. Checked in the browser: links are coloured and underlined in light and dark.
- Finding 3 and 5: checkout is now a server action. It validates on the server, redirects with field codes only (`?error=phone`), and passes delivery details to the confirmation page in a 10-minute httpOnly cookie, so no personal data is in the URL. Known limit: after a validation error the fields are empty again (values are not echoed).
- Findings 2 and 6 are covered by guidelines v1 only; findings 4 and 7 are unchanged.

## Boundary review (repo-doctor, after the run)

`repo-doctor` structural sweep: no fe-platform finding (15 warnings, all about other services' ungenerated code in a clean checkout). `contract-boundary-reviewer` over `fe-platform/`: no blocking violation; the api-client reaches only the gateway and generates from `platform-core`. One non-blocking note: the `/shop` demo hardcodes products and computes cart totals in the frontend and has no order RPC. Guidelines now say `/shop` is a demo and must not be copied for data or pricing logic.

