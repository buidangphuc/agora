# ADR-0001 — Proto distribution (pin + local-generate)

**Status:** Accepted · **Date:** 2026-08-30

## Context

The platform is a polyrepo with `packages/proto` as the single source of truth for
all gRPC contracts. A naive setup makes `platform-core`'s `buf.gen.yaml` write
generated code directly into sibling repos (`out: ../../team-*/generated`). That
only works if every repo is checked out side-by-side in one fixed on-disk layout —
i.e. a **monorepo on disk pretending to be a polyrepo**. It breaks when an agent
clones a single repo, and it lets a `make proto` in one repo silently mutate others.

We are local-only (no cloud), so a hosted schema registry (buf BSR) is out.

## Decision

- `platform-core` **owns** the proto module and versions it with **git tags** (`proto/vX.Y.Z`).
- `platform-core`'s `buf generate` writes **only into its own `./gen`** (verification;
  gitignored). It never writes outside its own tree.
- Each consumer repo **vendors the proto module** (git submodule/subtree) **pinned at a
  tag** and runs `buf generate` **in its own repo**, committing the result there.
- A contract change = a new proto tag. Consumers **bump the pin deliberately**; nothing
  is overwritten behind their back.

## Consequences

- Repos are independent; an agent can clone one and generate without the others present.
- Contract upgrades are explicit and reviewable (pin bump in a PR).
- Cost: consumers must run `buf generate` and re-pin on upgrade (a documented step).
- Revisit if/when we go remote: buf BSR could replace vendoring without changing this
  model (pin a BSR version instead of a git tag).
