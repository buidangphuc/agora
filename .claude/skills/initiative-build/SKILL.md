---
name: initiative-build
description: Build a locked ASDLC v2 initiative (plan section 13, steps 4-5) — verify plans/<id>/LOCK.md is approved and unchanged, build slices in DAG order (contract slices first and sequential, then waves of slices whose write-sets are disjoint per validate_plan.py), each slice through feature-run, then converge with features-check, the full e2e suite and feature-reviewer per change, and stop for the human before any merge or archive. Use after initiative-plan once the human has signed LOCK.md.
allowed-tools: Agent, Bash, Read, Edit, Write, Glob, Grep
license: MIT
metadata:
  author: asdlc-v2
  version: "1.0"
  plan: ASDLC-v2-PLAN.md W2-8, section 13 steps 4-5
---

# initiative-build — slices in DAG order, then converge

The MAIN session orchestrates and runs `feature-run` for every slice (subagents cannot spawn
subagents). It writes only `plans/<id>/**` and `metrics/runs/**`; code/tests come from the
feature-run workers. `ROOT` = main tree, `PY=$ROOT/platform-e2e/.venv/bin/python`.

## Inputs
- `id` — initiative with `plans/<id>/{00-spec.md, slices.yaml, LOCK.md}`.

## Preconditions (refuse to start otherwise)
1. `LOCK.md` has `status: approved` and a non-empty `approved-by:`; every slice row is ticked `[x]`
   (unticked slices are skipped and reported).
2. Each slice's current digest equals the LOCK.md digest (command in initiative-plan). A changed
   spec → STOP: the human re-locks via initiative-plan.
3. `$PY scripts/asdlc/cross_change_check.py --slices plans/<id>/slices.yaml` exits 0.

## State and metrics
`plans/<id>/run/initiative-build.md` — wave table (wave, slices, base branch, status), plus a
pointer to each slice's `plans/<id>/run/<slice>.md` (written by feature-run). Resume from it.
Events go to `metrics/runs/<id>.build.jsonl`; per-slice metrics stay in `metrics/runs/<slice>.jsonl`.

## Steps
1. **Waves from the DAG.** Start from the LOCK.md build order.
   - Wave 0: slices owned by `platform-core` (contracts), one at a time, in order.
   - Next waves: DAG levels. For each slice write a packet
     `plans/<id>/wave<N>-task<M>-<slice>.md` with `## Write-set` (union of the write-sets in its
     `tasks.md`, e2e files included) and `## Verify` (its test + e2e commands), then:
     ```bash
     $PY scripts/validate_plan.py plans/<id>
     ```
     A same-wave overlap error (the hot files noted in 00-spec) → move the later slice to the
     next wave and re-run until clean. Write the final wave table to the state file.
2. **Branches.** One local integration branch in its own worktree:
   `git worktree add .claude/worktrees/<id> -b asdlc/<id> main`. Each slice's feature-run
   worktree branches from `asdlc/<id>` as it stands when the slice's wave starts, so it sees all
   its dependencies. Never touch the user's branch.
3. **Build each wave.** Run `feature-run` for every slice of the wave (base = `asdlc/<id>`,
   `packet` = its packet). Slices of one wave run side by side: put the same step of all slices
   into ONE message of parallel Agent calls (all e2e-authors, then all test-authors, …), each with
   its own worktree. feature-run's step 6 (human approval) is deferred to step 5 below; its
   round-3 escalations still STOP the affected slice, and its dependents wait.
   After a slice reaches feature-run step 5 with pass:
   ```bash
   $PY scripts/validate_plan.py plans/<id> --audit plans/<id>/wave<N>-task<M>-<slice>.md --base <slice base sha>
   git -C .claude/worktrees/<id> merge --no-ff asdlc/<slice>     # local integration branch only
   ```
   Audit fail (files outside the write-set) → back to that slice's implementer, counts as a round.
   A merge conflict means the write-set analysis was wrong → STOP (human).
4. **Converge** in `.claude/worktrees/<id>` (branch `asdlc/<id>`), all must pass:
   ```bash
   make -C platform-e2e features-check VENV=$ROOT/platform-e2e/.venv/bin/
   (cd platform-e2e && HEADLESS=true $PY -m pytest -q)                 # full suite, not only the new scenarios
   for s in <slices>; do $PY platform-e2e/scripts/spec_sync.py $s --strict; openspec validate $s --strict; done
   $PY scripts/asdlc/cross_change_check.py --slices plans/<id>/slices.yaml
   ```
   Plus the CI checks of every touched repo (read `.github/workflows/*` and the repo Makefiles).
   Then ONE message: `feature-reviewer` per change (decide.py feature gate, advisory) and
   `privacy-reviewer` for each change routed `pii`. A regression in an older scenario → the slice
   whose diff caused it (bisect by merge commit) goes back into feature-run.
5. **STOP — human.** Present: wave table, per-slice task/round summary, converge gate table,
   feature-reviewer tables, privacy verdicts, `NEEDS_HUMAN` tags (auth/payment/pii/proto routing),
   and the archive order (= build order). The human decides merge of `asdlc/<id>`, push, and
   `openspec archive <slice>` per slice. Never do these yourself. Log `run_done`.

## Failure handling
- `decide.py` `ENGINE_DOWN` / missing → reviewers use their own checklists (LLM fallback); the
  converge table shows `ENGINE_DOWN (LLM fallback)` per affected gate.
- A slice escalates (3 rounds) → finish independent slices of the wave, hold its dependents,
  report at step 5 or earlier if nothing else can progress.
- Local stack down → steps 1-3 continue up to unit level; step 4 stops with `e2e=not run (stack down)`.
- Spec defect found while building → do not patch the spec here; run `spec-clarify` (human),
  update LOCK.md digest only after the human re-approves that slice.

## Guardrails
- Contract slices strictly first and alone; a consumer never starts before its contract merged into `asdlc/<id>`.
- Parallel only where `validate_plan.py` says write-sets are disjoint; WIP limit =
  `$PY scripts/asdlc_config.py --get wip_limit` slices in flight.
- No commit to the user's branch, no push, no archive, no `git checkout/stash/reset` in the main tree.

## Outputs
Packets + state in `plans/<id>/`, branch `asdlc/<id>` with all slices merged and green, metrics
jsonl, and the step-5 approval table.
