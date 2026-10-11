## Why

`assistant-rag-grounding` made `ShoppingAssistant` answer from real listings, but only when team-ai runs the RAG store:
in the base compose team-ai still starts with `RAG_ENABLED=false` and answers from the hard-coded demo catalog. The
real behaviour is reachable only by layering `platform-e2e/compose/modelserve.override.yaml`. A plain
`docker compose up` therefore shows the demo products (ids that are not listings) and `SearchListings` answers `UNAVAILABLE`.

## What Changes

- **Compose (base)**: `docker-compose.services.yaml` gains `tei-fake` (deterministic embedding stand-in) and
  `modelserve-router`, and team-ai gets the RAG environment (Qdrant store, `model_server` embeddings, listing indexer
  on `listing.events`), the `"recs ai kafka"` build extras and `depends_on` for the router, Qdrant and Redpanda.
- **E2E overlays**: `modelserve.override.yaml` shrinks to test-only settings (published host ports, `agora-*` image
  names, `MAX_QUEUE_DEPTH=10`, team-search wiring). New `rag-off.override.yaml` disables RAG and the indexer in team-ai.
- **E2E**: new `@destructive` scenario in `ai/assistant_grounding.feature` recreates team-ai with `rag-off.override.yaml`
  and checks the demo catalog answers (the scenario was `not-testable` before).
- No code default changes: `RAG_ENABLED` stays `false` in `app/core/config/ai.py` (unit tests, standalone runs).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `ai-assistant`: ADDED requirement "The local stack runs the assistant grounded by default".

## Impact

- Compose: `docker-compose.services.yaml`, `platform-e2e/compose/{modelserve,rag-off}.override.yaml`, `platform-e2e/compose/README.md`.
- Docs: `team-ai/README.md` (compose run section, env table, gotchas). Build/boot: team-ai image gains the `ai` and
  `kafka` extras (larger image); a plain `up` now also builds and starts two small services.
- E2E: `ai/assistant_grounding.feature`, `agr_steps.py`; `team-ai/FEATURES.yaml`.

## Non-goals

- No change to the code default, the proto, or the assistant logic.
- No real TEI/GPU in the base stack; team-search's hybrid wiring (`MODEL_SERVER_URL`) stays in the e2e overlay.
- No production/k8s change (`platform-gitops` is untouched).
