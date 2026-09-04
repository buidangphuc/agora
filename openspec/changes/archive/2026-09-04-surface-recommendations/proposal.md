## Why

**serve-recommendations-teamai** makes `team-ai` answer
`platform.recommendation.v1.RecommendationService/Recommend` over gRPC, but the capability is not
reachable from the UI: the gateway has no recommendation forwarder and the frontend has no
recommendation client, data layer, or row. This change wires the existing serving RPC through
the standard path (frontend → gateway → team-ai gRPC) and puts a **"Gợi ý cho bạn"**
recommendations row on the home page and/or product-detail page, exactly mirroring how
**wire-ai-assistant** surfaced `AIService` (gateway `ai.go` forwarder → `makeClients().ai` →
`lib/gateway/ai.ts` → Server Action → UI). No proto change (the contract already exists) and no
business logic in the gateway — it is a read-only forwarder that verifies auth once and forwards
`x-principal-*`.

## What Changes

- **team-gateway** — add a `RecommendationForwarder`
  (`internal/edge/recommendation.go`, new) implementing the Connect
  `RecommendationServiceHandler`, forwarding `Recommend` to team-ai over gRPC via
  `edge.callRead` + `edge.outgoing` (read-shaped, no Kafka emit, no business logic) — a direct
  copy of `internal/edge/ai.go`. Add the `Recommendation` client to the upstream routing table
  (`internal/upstream/clients.go`: import, struct field, `recAddr` dial param) and register the
  handler + reflector entry in `internal/edge/server.go`.
- **team-gateway** — add `UPSTREAM_RECOMMENDATION_ADDR` (`internal/config/config.go`, default
  team-ai's gRPC addr `localhost:50060` like `UPSTREAM_AI_ADDR`) and thread it into `Dial(...)`;
  set it in `docker-compose.services.yaml` to team-ai's gRPC endpoint.
- **team-frontend** — register the `recommendation` client in `makeClients()`
  (`src/lib/gateway/client.ts`), add a data layer
  (`src/lib/gateway/recommendations.ts`, new) mapping the proto response to plain view models
  (bigint → number) like `lib/gateway/ai.ts`, and a Server Action
  (`src/features/recommendations/actions.ts`, new).
- **team-frontend** — add a **"Gợi ý cho bạn"** row component
  (`src/features/recommendations/RecommendationsRow.tsx`, new) reusing the existing product-card
  presentation, placed on the home page (`src/app/page.tsx`) and/or the PDP
  (`src/app/listing/[id]/page.tsx`, seeded with the current listing id). Server-only — the
  browser never calls team-ai directly (Rule 1).
- **E2E (platform-e2e)** — add the user-facing scenario: a logged-in buyer sees the "Gợi ý cho
  bạn" row populated from team-ai via the gateway (not mocked). Update `team-frontend/FEATURES.yaml`
  with the capability and flip the shared recommendations coverage to `automated`.

**No proto change.** `RecommendationService` is defined by **add-recommendation-contract**; this
change consumes the already-generated Connect stubs in gateway and frontend (Rule 4). It does not
re-vendor or regenerate.

## Non-goals

- **New proto** — the contract already exists (add-recommendation-contract); no re-vendor.
- **The serving logic** — retrieval/ranking/cache lives in team-ai (serve-recommendations-teamai);
  the gateway forwards and the frontend renders.
- **Client-side ML** — no ranking or embedding in the browser; the frontend only displays what the
  RPC returns.
- **Personalized ranking UI controls** — no user-facing knobs to tune/reorder recommendations.
- **Gateway business logic** — the forwarder is a pure read-only pass-through (Rule 2).
