---
name: proto-change
description: Make a change to the gRPC/proto contract the right way — edit it once in platform-core/packages/proto, lint + breaking-check, regenerate, then propagate to every consuming team-* service. Use whenever an RPC or message needs to be added or changed. Enforces AGENTS.md rule #4 (contract is the single source of truth; never fork it, never hand-edit generated code).
disable-model-invocation: true
---

# Change the proto contract

A proto change is **cross-cutting**: it is defined once in platform-core and every
consumer regenerates. Read `AGENTS.md` §3 rule #4 first. Never hand-edit generated
code and never fork/duplicate a message into a service to "fix it locally".

## 0. Scope the change
- Which package? `platform-core/packages/proto/platform/<domain>/v1/`.
- Is it **breaking** (removed/renumbered field, changed type, removed RPC)? Breaking
  changes hit every consumer and need a coordinated rollout — call it out explicitly.
- Which services **consume** this message/RPC? Grep the workspace:
  `grep -rl "platform/<domain>/v1" team-*/proto team-*/generated`.

## Steps → verify

1. **Edit the source of truth only.** Change the `.proto` under
   `platform-core/packages/proto/platform/<domain>/v1/`. Add new fields with **new
   field numbers** (never reuse/renumber). Keep it additive when you can.
   → verify: the edit is under `platform-core/packages/proto`, nowhere else.

2. **Lint + breaking-check + generate in platform-core.**
   ```bash
   make -C platform-core proto        # buf lint + buf generate (go, python, ts, jvm)
   make -C platform-core lint-proto   # lint only
   # breaking gate: make -C platform-core check
   ```
   → verify: lint passes; if `buf breaking` flags something, either make it additive
   or confirm the coordinated breaking rollout with the user.

3. **Propagate to each consuming service.** Each `team-*` service vendors the proto
   subset it needs in its own `proto/` and generates into `generated/` via its
   `buf.gen.yaml`. For every consumer found in step 0:
   ```bash
   # sync the updated .proto into the service's proto/ tree, then:
   cd team-<name> && buf generate      # regenerates ./generated
   ```
   (Check the repo for an existing sync script/Make target before copying by hand.)
   → verify: `git status` in each consumer shows only regenerated `generated/**` +
   the vendored `proto/**` — no hand edits inside `generated/`.

4. **Update call sites.** Adjust handlers/clients that use the new/changed field or
   RPC. New RPCs that expose or mutate protected data need a `RequireScopes` gate in
   the owning service (§4).
   → verify: `go build ./...` / `go test ./...` green in each touched service;
   `auth-scope-reviewer` subagent CLEAN if any RPC changed.

5. **Ship as a versioned platform-core PR.** The contract change is one PR to
   platform-core that affects everyone; consumer regenerations are their own PRs.
   → verify: `contract-boundary-reviewer` subagent reports CLEAN (no forked/duplicated
   contract, no hand-edited generated code).

## Guardrails
- The `guard-generated` hook will **block** direct edits to `**/generated/**` and
  `*.pb.go` — that's intentional. Regenerate instead.
- If this is part of an OpenSpec change, keep the delta spec and `FEATURES.yaml` in
  sync (`AGENTS.md` §9b).
