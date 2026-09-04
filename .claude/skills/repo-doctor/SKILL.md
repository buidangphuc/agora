---
name: repo-doctor
description: Detect when the layers of this polyrepo have drifted out of sync — a service directory missing from docker-compose or the AGENTS.md port table, a port collision, a proto contract changed in platform-core but not regenerated in consumers, generated code hand-edited, or an OpenSpec change whose scenarios lack e2e coverage. Use before opening a PR, before archiving an OpenSpec change, after adding/removing a service, or whenever asked "is everything in sync / consistent / có bị lệch không". Composes the existing gates (spec-check, buf breaking, make check) with static cross-repo checks and the boundary reviewers into one drift report.
---

# repo-doctor — keep the layers from drifting

This polyrepo has many layers that must agree: **service dirs ↔ docker-compose ↔
AGENTS.md port table ↔ gitops**, **proto contract ↔ generated code ↔ each
consumer**, **OpenSpec scenarios ↔ FEATURES.yaml ↔ e2e**, **plan/INDEX status ↔
reality**. Each has its own tool; nothing checks them *together*. This skill runs
one coherence sweep and reports every drift with a fix pointer. It does not fix —
it reports; you (or the right skill) fix.

## When to run
- Before opening a PR that touched more than one repo.
- Before `openspec archive <id>` (drift is cheapest to catch pre-archive).
- Right after `new-go-service` / `gitops-scaffold-service` (registration is the #1 drift).
- After any `proto-change` (propagation to consumers is the #2 drift).
- Any time the user asks whether things are consistent / "có bị lệch không".

## Run order → what each layer catches

1. **Static structural drift (always; no toolchain needed).**
   ```bash
   python scripts/repo_doctor.py --root .
   ```
   Catches: a `team-*` dir absent from compose (won't run) or from the AGENTS.md §2
   port table (doc drift), port collisions, a Go service with `buf.gen.yaml` but no
   `generated/`/`proto-vendor/` (codegen not run), missing `FEATURES.yaml`.
   → ❌ = runtime-breaking, block. ⚠️ = doc/wiring drift, fix or justify.

2. **Contract propagation drift (when proto changed).** A `proto-change` must reach
   every consumer. Find who consumes the changed package and confirm each regenerated:
   ```bash
   pkg=platform/<domain>/v1
   grep -rl "$pkg" team-*/proto team-*/proto-vendor team-*/generated 2>/dev/null
   ```
   For each consumer, its `generated/` must be newer than the contract change and
   must NOT be hand-edited. The `guard-generated` hook blocks hand-edits live; here
   confirm none slipped in via `git -C <consumer> diff --stat -- generated/` (should
   be codegen-shaped only). Behind consumers → run the `proto-change` skill's
   propagation step.
   → drift = a consumer still on the old contract.

3. **Spec↔e2e drift (when an OpenSpec change is in flight).** Delegate to the owned gate:
   ```bash
   make -C platform-e2e features-check
   make -C platform-e2e spec-check CHANGE=<id>   # every #### Scenario has a green e2e
   ```
   → red = a scenario without automated coverage (violates AGENTS.md §9b shift-left).

4. **Boundary + auth drift (when service/gateway/proto/auth changed).** Run the
   reviewers over the diff as the semantic gate:
   - `contract-boundary-reviewer` — frontend→gateway only, DB-per-service, right
     broker, no forked contract.
   - `auth-scope-reviewer` — every new/changed RPC has a `RequireScopes` gate; no
     verifier outside the gateway; principal not spoofable.
   → any BLOCKING finding = a boundary drifted from AGENTS.md §3/§4.

5. **Plan drift (when a `plan/` exists).**
   ```bash
   python scripts/validate_plan.py plan/
   ```
   → write-set overlap, unlocked spec, or `[UNRESOLVED]` left behind.

## Output — one report
Summarize as a table: **Layer | Status (✅/⚠️/❌) | Drift | Fix**. Lead with any ❌
(runtime-breaking / block the PR), then ⚠️ (doc/coverage drift), then ✅. For each
drift name the exact file and the skill/command that fixes it — don't fix silently.
End with a one-line verdict: **safe to merge/archive** or **N blockers**.

## Scope discipline
Report only. Skip a layer that doesn't apply (no proto change → skip step 2) and say
so, rather than running everything blindly. Steps 3–4 need Docker/venv and a change
id — if unavailable, print the command for the user instead of failing the sweep.
