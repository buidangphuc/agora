---
name: feature-run
description: Build ONE locked OpenSpec change test-first with the ASDLC v2 worker/reviewer loop — preflight, e2e-author (red), then per task test-author → implementer → red_first.sh → task-reviewer (+ script-routed specialist reviewers), max 3 rounds per task, then e2e green, features-check, feature-reviewer and privacy-reviewer, and a human approval stop. Resumable from plans/<id>/run/<change>.md. Use when a change is locked and ready to build, or when initiative-build hands you a slice. Not for unlocked specs (use initiative-plan / spec-clarify first).
allowed-tools: Agent, Bash, Read, Edit, Write, Glob, Grep
license: MIT
metadata:
  author: asdlc-v2
  version: "1.0"
---

# feature-run — one change, test-first, reviewed

The MAIN session is the orchestrator (subagents cannot spawn subagents). It never writes
production code or tests itself; it runs scripts, calls subagents via the Agent tool, keeps the
state file and stops for the human. `ROOT` = main tree; `PY=$ROOT/platform-e2e/.venv/bin/python`
(absolute: worktrees have no `.venv`, and system python lacks PyYAML).

## Inputs
- `change` — id in `openspec/changes/`. Must be locked: listed `approved` in
  `plans/<id>/LOCK.md`, or the human says "locked" in chat for a standalone change.
- `initiative` — from the `initiative:` line in `proposal.md`; standalone → `adhoc`.
- Optional `base` ref (default `main`), `packet` (`plans/<id>/wave*-task*-<change>.md` from initiative-build).

## State and metrics (create on start, update after every step; this is how a run resumes)
`plans/<initiative>/run/<change>.md`:
```markdown
# feature-run <change>
initiative: <id> | base: <sha> | worktree: .claude/worktrees/<change> | branch: asdlc/<change> | started: <iso>
## Preflight   validate=<p/f> spec_lint=<p/f> spec_sync=<n uncovered>
## E2E         author=<done|blocked> run=<red as expected|red wrong reason|green> files=<n>
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
## Gates       e2e=… features-check=… spec_sync --strict=… feature-reviewer=… decide=… privacy=…
## Escalations (round-3 failures, blocked reports, human answers)
```
Task status: `todo → tests-red → implemented → reviewing → approved | escalated`.
Resume = read this file, skip `approved`, re-enter each task at its status.
Append one JSON line per event to `metrics/runs/<change>.jsonl` (for W5-1):
`{"ts","change","initiative","event","task","round","gate","result","wall_s"}` with events
`preflight | e2e_red | task_round | gate | escalation | task_done | run_done`; `run_done` also
carries `rounds_total`, `gate_red` (count per gate) and total `wall_s`.

## Steps
1. **Preflight** (main tree). All must pass, else stop and report:
   ```bash
   openspec validate <change> --strict
   $PY scripts/asdlc/spec_lint.py <change>
   $PY platform-e2e/scripts/spec_sync.py <change>          # report only; uncovered expected now
   ```
   Then the worktree, one per change: `git worktree add .claude/worktrees/<change> -b asdlc/<change> <base>`
   (skip if the state file already names it).
2. **E2E red.** Agent `e2e-author` with: change id, worktree path. Expect `STATUS: done` and
   `e2e run=red as expected`. `green` or `red wrong reason` = defect → re-run with the reason
   (counts as a round of the e2e step, max 3). Checkpoint commit (see Guardrails): `test(e2e): <change> red`.
3. **Per code-track task** in `tasks.md` order (tasks with `[e2e]` / `track: e2e` are done by step 2).
   For each task read its scenario ids and write-set from `tasks.md` (or the packet).
   a. `T0=$(git -C .claude/worktrees/<change> rev-parse HEAD)`; record it.
   b. Agent `test-author`: task id, scenario ids, interface/proto paths, worktree path. Need
      `red-first=red for right reason`. Checkpoint commit `test(<task>): red`.
   c. Agent `implementer`: task id, scenario ids, write-set, failing test command, worktree path.
      `test-defect` → send its OPEN_ISSUES to test-author (same round). Checkpoint commit `feat(<task>): green`.
   d. Script gates (in the worktree):
      ```bash
      (cd .claude/worktrees/<change> && scripts/asdlc/red_first.sh --base $T0 --head HEAD)
      git -C .claude/worktrees/<change> diff --name-only $T0..HEAD > <run>/<task>.paths
      $PY -c 'import sys;sys.path.insert(0,"scripts");from asdlc_config import route;print(" ".join(sorted(route(open(sys.argv[1]).read().split()))))' <run>/<task>.paths
      $PY scripts/validate_plan.py plans/<initiative> --audit <packet> --base $T0   # only if a packet exists
      ```
      No packet → compare the paths file against the task write-set yourself; any path outside = finding.
   e. Review, ONE message with parallel Agent calls: `task-reviewer` (task id, scenario ids,
      base `$T0`, write-set, test command) plus routed specialists — `proto` →
      `contract-boundary-reviewer`, `auth` → `auth-scope-reviewer`. `pii` is reviewed at step 5;
      `payment`/`auth`/`pii` also set `NEEDS_HUMAN` for step 6.
   f. Verdict: task `approved` only if red_first exit 0, write-set clean, task-reviewer `approve`,
      and no specialist BLOCKING finding. Otherwise send BLOCKING findings back (implementer for
      code, test-author for test findings), increment `rounds`, repeat c-f.
   g. **After round 3 still red → STOP** (human): mark `escalated`, list the open findings, ask
      the human to decide (fix spec via spec-clarify, accept, or drop the task). Do not continue
      to the next task if it depends on this one.
4. **Converge** (worktree), all must pass:
   ```bash
   (cd .claude/worktrees/<change>/platform-e2e && HEADLESS=true $PY -m pytest -q <change's .feature paths>)
   make -C .claude/worktrees/<change>/platform-e2e features-check VENV=$ROOT/platform-e2e/.venv/bin/
   (cd .claude/worktrees/<change> && $PY platform-e2e/scripts/spec_sync.py <change> --strict)
   ```
   A red here goes back to the task that owns the failing scenario (counts as a round of it).
   Then run the `docs-sync` skill on `<base>..HEAD` (READMEs match the landed code).
5. **Feature gate.** Agent `feature-reviewer` (change id, worktree path, optional units file
   `<run>/<change>.feature-units.json` for `decide.py --gate gates/feature.v1.yaml --mode advisory`;
   unit = one scenario `{id, parts:{scenario, test, assertion}, paths}`). Advisory: `WOULD_BLOCK`
   is recorded, not enforced. If any task routed `pii` (or the proposal declares personal data),
   run Agent `privacy-reviewer` on the whole change diff `<base>..HEAD` in the same message.
6. **STOP — human approval.** Present: task table (status, rounds), gate table, feature-reviewer
   table, privacy verdict, `NEEDS_HUMAN` tags, open issues, and the branch `asdlc/<change>`.
   The human decides merge/commit to the real branch, push, and `openspec archive`. Log `run_done`.

## Failure handling
- `decide.py` → `ENGINE_DOWN` (or missing): reviewer falls back to its own checklist (LLM
  reviewer); write `decide=ENGINE_DOWN (LLM fallback)` in the state file and the final table.
  `ESCALATE_LLM` questions are answered by that reviewer, max `escalation.max_rounds` (3), then human.
- Subagent `STATUS: blocked` → record its OPEN_ISSUES under Escalations; spec ambiguity → run
  `spec-clarify` on the change (human stop), then resume the task.
- Local stack down (e2e cannot run) → keep tasks going; step 4 stops with `e2e=not run (stack down)`.
- Session interrupted → restart this skill with the same change id; it resumes from the state file.

## Guardrails
- Checkpoint commits exist only on the local `asdlc/<change>` worktree branch, because
  `red_first.sh` needs refs. Never commit to the user's branch, never push, never archive, never
  `git checkout/stash/reset` in the main tree. Run the CI checks of the touched repos before
  presenting step 6.
- One implementer per worktree at a time; tasks of one change run in order.
- The orchestrator does not edit tests, code or specs; spec changes go through spec-clarify.

## Outputs
State file, `metrics/runs/<change>.jsonl`, branch `asdlc/<change>` with green tests, and the
step-6 approval table.
