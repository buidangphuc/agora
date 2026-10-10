## Why

The hybrid search semantic floor `HYBRID_SEMANTIC_MIN_SCORE` was set to 0.6 based on bge-small-en-v1.5's
published similarity range, not on a measurement. A measurement on a GPU host shows that 0.6 is the 90th
percentile of unrelated query-title pairs, so about one unrelated neighbour in ten still passes.

## What Changes

- The default `HYBRID_SEMANTIC_MIN_SCORE` becomes 0.65. That is above every unrelated pair but one in the probe, and
  below every related pair.
- The probe script (`team-search/scripts/semantic_floor_probe.py`) is committed so the floor can be re-measured for
  another model or for real traffic.
- The e2e overlay keeps its own 0.3, because the fake TEI has a different cosine scale.

Repos: team-search.

## Capabilities

### Modified Capabilities
- `search-retrieval`: the semantic floor's default.

## Non-goals

- Changing the embedding model. multilingual-e5-small was measured too, and its margin is no wider.
- A per-query adaptive floor.
