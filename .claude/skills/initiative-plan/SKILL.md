---
name: initiative-plan
description: Plan a multi-repo initiative the ASDLC v2 way (plan section 13, steps 1-3) — build or accept plans/<id>/00-spec.md, cut it into slices.yaml (one slice = one OpenSpec change, owner, depends_on, uses/provides), fan out one team-spec-author per slice in parallel, review every change (validate --strict, spec_lint, spec-quality-reviewer with the decide.py spec gate) plus cross_change_check.py, loop feedback max 3 rounds per slice, then STOP with a LOCK.md for the human to sign. Use when a goal spans several repos/capabilities and must be specified before anything is built; follow with initiative-build.
allowed-tools: Agent, Bash, Read, Edit, Write, Glob, Grep
license: MIT
metadata:
  author: asdlc-v2
  version: "1.0"
  plan: docs/asdlc/ASDLC-v2-PLAN.md W2-8, section 13 steps 1-3
---

# initiative-plan — spec an initiative, stop at the lock

The MAIN session orchestrates (subagents cannot spawn subagents). It writes only
`plans/<id>/**`; specs are written by `team-spec-author`, never by the orchestrator.
`PY=platform-e2e/.venv/bin/python`.

## Inputs
- `id` — initiative slug (kebab-case); the goal in the human's words, or an existing `plans/<id>/00-spec.md`.
- Profile from `$PY scripts/asdlc_config.py --get profile` (`solo`: teams = subagents;
  `team`: changes arrive as PRs from people, skip step 3 and review them in step 4).

## State and metrics
`plans/<id>/run/initiative-plan.md` — step reached, per slice: author status, review round,
validate / spec_lint / spec-quality / decide result, cross-check violations. Resume from it.
Append events to `metrics/runs/<id>.plan.jsonl` (`{"ts","initiative","event","slice","round","gate","result","wall_s"}`).

## Steps
1. **Initiative spec** — `plans/<id>/00-spec.md`.
   - Exists with a `## Locked` line and no `[UNRESOLVED` tag → accept it, go to step 2.
   - Otherwise follow `parallel-scrum` Steps 1, 2 and 2.5 (recon within read budget, SDD-lite
     spec, clarify until locked), starting from `plans/_template/00-spec.md`. Fill its
     `Capabilities → slices` table; every new RPC/event goes in `Contracts`.
   - **STOP (human):** the human confirms the spec ("chốt"); append `## Locked <date> — <summary>`.
2. **Slices** — write `plans/<id>/slices.yaml` (format and comments: `plans/_template/slices.yaml`).
   Rules:
   - One slice = one OpenSpec change = one user-observable capability, buildable and e2e-testable
     once its `depends_on` have landed. `owner` = the repo whose `FEATURES.yaml` owns the
     capability (frontend is the UI shell, not the owner of a backend capability).
   - Any proto change → a contract slice owned by `platform-core`, `depends_on: []`, with
     `provides:`; consumers list `uses:` (`Service.Rpc` or `<topic>/<Event>`).
   - No slice for work that changes no requirement (one-line bug fix, finishing a task of an
     existing change, pure infra values) — list it under `Out of initiative` in 00-spec.
   - Name the hot files two slices will both edit (gateway `server.go`, a shared FE client):
     note them in 00-spec; initiative-build serialises those slices.
3. **Team spec, in parallel** — ONE message with one Agent call per slice:
   `team-spec-author` with slice id, owner, initiative id, `plans/<id>/slices.yaml`, and the
   previous round's feedback (none in round 1). Collect each `STATUS / FILES / CHECKS / OPEN_ISSUES`.
4. **Spec review.**
   a. Per change (orchestrator, cheap first):
      ```bash
      openspec validate <slice> --strict
      $PY scripts/asdlc/spec_lint.py <slice>
      ```
      Then ONE message with one `spec-quality-reviewer` per change (change id, initiative id).
      It runs `decide.py --gate gates/spec.v1.yaml --mode advisory`; give it the units file
      `plans/<id>/run/<slice>.spec-units.json` if you built one (unit = one requirement block
      with its scenarios + proposal Why: `{id, parts:{requirement, why}, paths}`).
   b. Cross-change:
      ```bash
      $PY scripts/asdlc/cross_change_check.py --slices plans/<id>/slices.yaml
      ```
      Violations (`schema|dag|contract|ownership|conflict: …`) are attributed to the slice they name;
      keep the printed build order for LOCK.md.
   c. A slice fails if validate or spec_lint fails, the reviewer says `fail`, or a cross-check
      violation names it. Send its BLOCKING findings + violations back to step 3 for that slice
      only (one parallel message for all failing slices). Max **3 rounds per slice**; still failing →
      mark `escalated` and carry it to the lock with its findings.
   d. QUESTION findings and author OPEN_ISSUES that only a human can settle → run `spec-clarify`
      per affected change (human answers), then re-run 4a/4b for that change.
5. **Lock template** — write `plans/<id>/LOCK.md` (format below) and **STOP (human)**. Do not
   start initiative-build; do not edit specs after this point without reopening step 3.

## LOCK.md format
```markdown
# LOCK — <id>
status: pending            # human sets: approved | rejected
approved-by:               # human name
date:
spec: plans/<id>/00-spec.md (Locked <date>)
## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| <id> | <repo> | [..] | n | pass | pass | pass | PASS / WOULD_BLOCK / ENGINE_DOWN | <sha256 12> | [ ] |
## Build order (cross_change_check)
1. <contract slice> (sequential)  2. … — hot-file pairs that must not run in parallel: …
## Cross-change check
<pass | violations list>
## Open issues / NEEDS_HUMAN
- escalated slices, routed auth/payment/pii, assumptions locked in
```
Digest per change: `find openspec/changes/<slice> -type f | sort | xargs shasum -a 256 | shasum -a 256 | cut -c1-12`
(initiative-build refuses a slice whose digest changed after approval).

## Stop points (human)
Step 1 (00-spec lock), step 4d (spec-clarify answers), step 5 (LOCK.md sign-off).

## Failure handling
- `decide.py` returns `ENGINE_DOWN` or is missing → spec-quality-reviewer's own ISO 29148/EARS
  checklist is the verdict (LLM reviewer fallback); write `ENGINE_DOWN (LLM fallback)` in LOCK.md.
- `cross_change_check.py` crashes or is absent → do its five checks by reading, mark `n/a (manual)` in LOCK.md.
- A team-spec-author reports `blocked` on a missing interface → fix `uses`/`provides` in
  slices.yaml or the contract slice, then re-run step 3 for the affected slices (counts as a round).

## Outputs
`plans/<id>/00-spec.md` (Locked), `plans/<id>/slices.yaml`, `openspec/changes/<slice>/` per slice,
`plans/<id>/run/initiative-plan.md`, `plans/<id>/LOCK.md` (pending), metrics jsonl.
