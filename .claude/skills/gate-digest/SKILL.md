---
name: gate-digest
description: Run unit tests, coverage (spec_sync), features-check, red_first, the write-set audit, tsc and biome through scripts/asdlc/gate_digest.py, which prints a compact JSON verdict and writes the full log to a file. Use for every test/coverage/gate run in feature-run, initiative-build, converge and Workflow gate agents, so agents read the verdict instead of raw logs. Not for writing tests or code.
allowed-tools: Bash, Read
license: MIT
metadata:
  author: asdlc-v2
  version: "1.0"
---

# gate-digest — mechanical checks, minimal reading

The check runs as code; the agent reads ~15 lines of JSON. `PY=<main tree>/platform-e2e/.venv/bin/python`,
`G="$PY <main tree>/scripts/asdlc/gate_digest.py"`. Exit 0 pass, 1 fail, 2 tooling error.

| need | command |
|---|---|
| Go unit tests (+vet) | `$G go <wt>/<repo> --vet [-- ./internal/x/...]` |
| pytest (explicit paths only) | `$G pytest <cwd> -- <paths> [-m "not destructive"]` |
| vitest / tsc | `$G vitest <wt>/team-frontend [-- <files>]` · `$G tsc <wt>/team-frontend` |
| biome vs base | `$G biome <wt>/team-frontend --base-count <errors at slice base>` |
| spec↔e2e coverage | `$G spec-sync <wt> <change>` |
| FEATURES manifests | `$G features-check <wt>` |
| write-set audit | `$G audit <wt> <initiative> <packet> <T0>` |
| red-first | `$G red-first <wt> <T0> [--script <local-plugins copy>]` |
| whole task gate | `$G task-gate <wt> <initiative> <packet> <T0> -- "<task test command>"` |
| anything else | `$G run --cwd <dir> -- "<command>"` |

## Rules
1. Decide from the JSON fields (`verdict`, counts, `new_errors`, `uncovered`, `failed_tests`), not by
   reading logs. Open `log` only for a failure, and only around the line it names (`grep -n`, `sed -n`).
2. Report the JSON (or its key fields) as your result. Do not paste raw output.
3. `skipped_total > 0` in Go means Postgres/integration tests did not run: say "db-tests: skipped", or run them
   on a throwaway `postgres:16-alpine --rm` container and re-run. A skip is never a pass.
4. Encoded non-defects (do not escalate them): packet edits made by `chore(plans): widen` commits or undone by a
   later `chore(<task>): revert plan edit`; paths that came in by merging the integration branch;
   biome errors at or below the base count.
5. `red-first` `green_at_base` on a verification-only task (proto copies, codegen checks, wire gates) →
   empty commit with `Red-First-Exempt: <exact path> -- verification-only: delivered by <task>`; `verdict=error`
   with buf warnings → tooling, reproduce by hand (worktree README gen.sh) instead of sending code back.
6. e2e against the local stack needs the stack env (KAFKA_BROKERS=localhost:19092, container names): run it as
   `$G run --cwd <wt> -- "bash <e2e.sh> <wt> <paths>"`, not `$G pytest` (which sets no env; Kafka readers then
   silently see nothing). Run e2e files one at a time: files with @destructive/timer scenarios disturb each other.
7. In a Workflow, gate/converge agents that only run checks use `effort: 'low'` and this skill.

## Outputs
One JSON object per check on stdout; full logs under `$GATE_LOG_DIR` (default `$TMPDIR/gate-digest`).
