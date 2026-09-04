---
name: spec-dispatch
description: Orchestrate parallel implementation of an approved OpenSpec change. Fans out the code track (per affected repo) and the e2e track (spec-to-e2e) concurrently, then runs the convergence gate before archiving. Use when a change is proposed and you want to build it with maximum parallelism across repos and the test platform.
allowed-tools: Task, Bash, Read, Edit, Write, Glob, Grep
license: MIT
compatibility: Requires openspec CLI, the local stack running, and platform-e2e set up.
metadata:
  author: platform-e2e
  version: "1.0"
---

# spec-dispatch — parallel build of one OpenSpec change

Turn one approved change into concurrent work streams, then converge on a gate.
This is where OpenSpec's isolation pays off: disjoint surfaces → no conflicts.

## Preconditions
- A proposed change exists: `openspec show <change-id>` returns proposal + spec + tasks.
- `tasks.md` is split into a `code` track (per repo) and an `e2e` track (see config rules).

## Orchestration
1. **Read the change**: `openspec show <change-id>`; list affected repos (from
   proposal `services:` / spec capability) and the e2e scenarios.
2. **Fan out (parallel subagents)** — one Task per disjoint unit, run in the same batch:
   - Code track: one subagent per affected repo. Each implements ONLY its repo's
     tasks, obeying root `AGENTS.md` (gateway-only edge, proto is source of truth,
     per-service DB, right broker). Use worktree isolation when repos are edited
     concurrently. Prefer `/opsx:apply` semantics inside each repo subagent.
     - Contract change? A proto edit is a platform-core PR that affects everyone —
       do it FIRST and serially, then fan out the consumers.
   - E2e track: one subagent running the `spec-to-e2e` skill for this change.
3. **Do NOT block e2e on code**: e2e is written from the spec; it may be red until the
   code lands. That's expected — the gate is the sync point, not per-task ordering.
4. **Converge — gate (all must pass)**:
   - `openspec validate <change-id> --strict`
   - `make -C platform-e2e features-check`
   - `cd platform-e2e && HEADLESS=true python -m pytest -q` (the change's scenarios green)
   - `ruff check platform-e2e && black --check platform-e2e`
5. **Archive**: when green, `openspec archive <change-id>` (folds the spec delta into
   `openspec/specs/`). The `<repo>/FEATURES.yaml` is already updated by the e2e track.

## Guardrails
- One subagent = one disjoint surface (a repo, or the e2e platform). Never let two
  subagents edit the same file.
- Report a live status table: per repo code done? e2e per scenario automated/blocked?
  gate results. Stop before archive if any gate is red and surface what blocks it.
- Blocked-by-backend scenarios stay `planned`/`manual` with a `notes: BLOCKED` line;
  they do not hold up archiving the rest of the change if the proposal scoped them out.
