---
name: docs-sync
description: Keep READMEs and docs in step with the code after any change — map every changed proto RPC, migration, env var, Kafka topic/event, port, route, Make target, compose override, ADR, skill or Go version to the doc section that lists it, update that section with path:line evidence, and move fixed or newly found mismatches in and out of README "Known gaps". Use before every commit/PR that touches code or config, in feature-run step 4 and initiative-build step 4 (converge), and when asked to "cập nhật docs", "sửa readme", "docs sync". Not for writing specs (OpenSpec) or new design docs.
allowed-tools: Bash, Read, Edit, Grep, Glob
license: MIT
metadata:
  author: asdlc-v2
  version: "1.0"
---

# docs-sync — the README states what the code does now

Docs describe landed code only. Run it on the diff of the change (`<base>..HEAD`), in the worktree
that holds the change, before the pre-commit-ci skill.

## Steps
1. **Changed files.** `git diff --name-only <base>..HEAD`, grouped by top-level repo dir.
2. **Map each change to its doc home** and check that section:

   | change in the diff | doc section to update |
   |---|---|
   | `rpc` added/removed/renamed in a `.proto` | owner `<repo>/README.md` RPC table; `<repo>/proto/README.md` owned services; `platform-core/README.md` contract table; `team-gateway/README.md` if the gateway forwards or gates it (`internal/edge/server.go`, `policy.go`) |
   | `migrations/NNNN_*.up.sql` | owner README Data/Migrations table |
   | env var read in config (`config.go` env tags, `settings`/`config/*.py`) | owner README Config table **and** `<repo>/.env.example` (CI `check-env`) |
   | Kafka topic, event type produced/consumed, DLQ | owner README Events; `platform-core/docs/DATA_ARCHITECTURE.md` topic catalog |
   | gRPC/HTTP/host port, new service | `AGENTS.md` §2 ports + service table; `docker-compose*.yaml` is the source of truth |
   | gateway route, admin procedure | `team-gateway/README.md` route/gate tables |
   | frontend page or API route | `team-frontend/README.md` pages/surface tables |
   | Make target, script, flag | the README that lists targets/scripts |
   | `platform-e2e/compose/*.override.yaml` | `platform-e2e/compose/README.md` overlay table |
   | new ADR / status change | `platform-core/README.md` ADR table |
   | new skill, agent, hook | `AGENTS.md` lists |
   | `go` line in `go.mod`, Node/Python version | README Build section |
   | `FEATURES.yaml` entries | README feature count only if the README states one |

   Counts (features, files, RPCs) are computed by a command and the command is put in the evidence, never estimated.
3. **Edit minimally.** Fix wrong facts, add the missing row to an existing list. English only. No rewording
   of correct text, no new doc files. ADRs: only paths/links, never decisions.
4. **Contract-only items.** An RPC that exists in proto but whose handler is the embedded
   `Unimplemented*Server` is documented as "Contract only (<change id>, not built)". Never describe planned
   behaviour as current.
5. **Known gaps.** A change that fixes a gap listed under README "Known gaps" removes that line.
   A mismatch you find but cannot fix in this change (code vs proto, code vs FEATURES.yaml, generated doc)
   goes under the owner README "Known gaps" with `path:line`, not only into chat or memory.
6. **Generated docs** (`docs/runbooks/team-*.md`, `docs/asdlc/baseline.md`): fix the generator input and
   regenerate; never hand-edit.

## Output
A table `file | section | change | evidence (path:line or command)`, or
`docs-sync: no doc change needed — <reason>` (e.g. internal refactor, tests only).
Done when every changed item in step 2 has a row or an explicit "no doc lists it".
