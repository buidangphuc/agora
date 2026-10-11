---
name: spec-clarify
description: Before an OpenSpec change is locked, find the ambiguities that would make its e2e tests guessable (missing trigger or actor, unmeasurable outcome, missing error or permission case, unclear data classification), ask the human at most 8 numbered questions with a suggested default each, and record the answers in the change's design.md "Clarifications" section. Use after team-spec-author / spec-quality-reviewer and before the human lock (initiative-plan step 3d), or whenever a change "looks done" but its scenarios could be tested two ways.
allowed-tools: Bash, Read, Edit, Write, Glob, Grep
license: MIT
metadata:
  author: asdlc-v2
  version: "1.0"
  plan: ASDLC-v2-PLAN.md W2-4
---

# spec-clarify — ask before the spec is locked

Run by the MAIN session. No subagent: the questions go to the human, and only the human
answers them. This skill never edits `specs/**` or `proposal.md` without an answer.

`PY=platform-e2e/.venv/bin/python` (system python lacks PyYAML).

## Inputs
- `change` — an id under `openspec/changes/<change>/` (not archived).
- Optional: `initiative` id (to read `plans/<initiative>/00-spec.md` for intent), the latest
  spec-quality-reviewer report (its QUESTION findings and OPEN_ISSUES are candidate questions).

## Steps
1. **Machine findings first.**
   ```bash
   openspec validate <change> --strict
   $PY scripts/asdlc/spec_lint.py <change> --json --warn-only     # read findings[].rule/line
   ```
   Lint rules that map straight to e2e ambiguity: `SCN-when` (no trigger), `SCN-then` /
   `THEN-vague` (unmeasurable outcome), `ERR-scenario` (no error case), `PERM-scenario`
   (no permission case), `EARS-shall`, `PROPOSAL-sections` (missing NFR / Data classification /
   threat model). Do not ask about anything the lint already states precisely and the author
   can fix alone (wording); route those back to the author instead.
2. **Read the change** (`proposal.md`, `specs/**/spec.md`, `tasks.md`, `design.md` if any) and,
   for each `#### Scenario:`, try to write the e2e test in your head. Every point where you
   would have to *choose* is a candidate:
   | Category | Ambiguity that blocks an e2e |
   |---|---|
   | trigger/actor | who acts (buyer, seller, admin, anonymous, service) and through which entry (route, RPC, event)? |
   | outcome | what observable, persisted state proves success (row, status, UI text, event)? numbers/limits? |
   | error | what happens on invalid input, upstream down, timeout, duplicate request? |
   | permission | who must be refused, with what status/message; ownership checks? |
   | data | which personal fields, purpose, consent basis, retention; is it logged or sent out? |
   | boundary | empty, max page size, zero results, concurrent edits? |
3. **Resolve from the repo before asking.** Check proto (`platform-core/packages/proto`),
   the owner repo's `FEATURES.yaml`, `openspec/specs/`, AGENTS.md rules. A question the code or
   an existing spec already answers is not a question; note the source instead.
4. **Write the question list** (max 8, most e2e-blocking first). Format, exactly:
   ```
   Q<n> [<category>] <scenario id or file:line>
       Question: <one closed question>
       Why it matters for e2e: <which assertion/fixture can't be written without it>
       Suggested default: <concrete answer + one-line rationale from repo evidence>
   ```
   More than 8 real questions means the change is under-specified: say so, ask the 8 that
   unblock the most scenarios, and recommend sending the change back to team-spec-author.
5. **STOP — human answers.** Present the list and wait. Accept "default" per question. A
   question left unanswered stays open; never treat silence as the default.
6. **Record answers** in `openspec/changes/<change>/design.md` (create with a `# Design` heading
   if absent), appending one section per round:
   ```markdown
   ## Clarifications
   ### Round <k> — <YYYY-MM-DD>
   - Q1 [permission] <scenario>: <question> → **<answer>** (default accepted | human)
   ```
7. **Apply only answered items.** Either update the scenario/proposal text yourself to match
   the answer (minimal edit, cite `Clarifications Q<n>`), or — inside initiative-plan — hand the
   answers to team-spec-author as review feedback. Then re-run step 1; both commands must pass.

## Stop points (human)
- Step 5: always. This skill cannot finish without human answers.

## Failure handling
- `openspec validate` fails for structural reasons → fix structure first (author's job), then clarify.
- spec_lint missing or crashes → do step 2 by reading only; say "spec_lint=n/a" in the output.
- Answer contradicts another locked change or `openspec/specs/` → do not apply; raise it as a
  new question naming both sources.

## Outputs
- Question list (chat), `design.md` → `## Clarifications` round, updated spec text for answered
  items, and a one-line result:
  `CLARIFY <change>: asked=<n> answered=<n> open=<n> validate=<pass|fail> spec_lint=<pass|fail|n/a>`
