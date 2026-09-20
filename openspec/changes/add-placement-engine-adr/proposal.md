## Why

Recommendations across the marketplace require explicit placement-specific configurations (home feed, product similar items, cart cross-sell), a deterministic fallback ladder, cold-start handling, and latency budget management.

Following **ADR-0012**, this change formalizes the architecture for the Placement Engine and defines its specification.

## What Changes

- **platform-core** (`docs/ADR/0012-placement-engine.md`):
  - 3-layer configuration model (WHAT/HOW).
  - 4-tier fallback relaxation ladder.
  - 4-outcome cold-start taxonomy.
  - Call budget and latency SLAs.

## Non-goals

- Implementation code lands in `add-placement-engine` (P2-T5).
