# Tasks

Depends on **serve-recommendations-teamai** (team-ai answers `Recommend`) and, transitively,
**add-recommendation-contract** (generated Connect stubs in gateway + frontend). The code below
is written against the already-generated `RecommendationService` stubs; it must not regenerate or
hand-edit them (Rule 4).

## 1. Code — team-gateway
- [x] `internal/upstream/clients.go`: add `recommendationv1` import, `Recommendation
      recommendationv1.RecommendationServiceClient` field, a `recAddr` param to `Dial(...)`, and
      `dial("recommendation", recAddr)` + `NewRecommendationServiceClient`.
- [x] `internal/edge/recommendation.go` (new): `RecommendationForwarder` implementing
      `recommendationv1connect.RecommendationServiceHandler` — `Recommend` via `edge.callRead`,
      `edge.outgoing(ctx, req.Header())`, `toConnectErr`. Direct copy of `internal/edge/ai.go`
      (read-only, no Kafka emit, no business logic).
- [x] `internal/edge/server.go`: register
      `recommendationv1connect.NewRecommendationServiceHandler(NewRecommendationForwarder(clients.Recommendation, e), opts)`
      in `NewMux`, and add `recommendationv1connect.RecommendationServiceName` to the reflector.
- [x] `internal/config/config.go`: add `RecAddr string ` + `UPSTREAM_RECOMMENDATION_ADDR`
      (default `localhost:50060`, team-ai gRPC); thread into `Dial(...)` (also `cmd/gateway/main.go`
      + `.env.example` for the bidirectional env drift gate `TestEnvExampleInSync`).
- [ ] `docker-compose.services.yaml`: set `UPSTREAM_RECOMMENDATION_ADDR=team-ai-svc:<team-ai grpc
      port>`. DEFERRED — `docker-compose.services.yaml` is reserved/out of scope for this session.
      The gateway default (`localhost:50060`, team-ai's gRPC) already matches `UPSTREAM_AI_ADDR`;
      set the compose override alongside `UPSTREAM_AI_ADDR` when wiring the stack.
- [x] Generated symbol names verified against `recommendationv1connect`. Vendored the contract into
      `team-gateway/proto/platform/recommendation/v1/recommendation.proto` (the frontend/gateway
      consume-copy, like every other contract) and ran `buf generate`: the forwarder + wiring
      compile-match the real stubs (`RecommendationServiceHandler`, `UnimplementedRecommendationServiceHandler`,
      `NewRecommendationServiceHandler`, `RecommendationServiceName`, `recommendationv1.RecommendationServiceClient`,
      `Recommend(ctx, *RecommendRequest) (*RecommendResponse, error)`). `go build ./...` itself
      DEFERRED to CI Docker build (Go toolchain not on host; `generated/` is gitignored).

## 2. Code — team-frontend
- [x] `src/lib/gateway/client.ts`: import `RecommendationService`, add
      `recommendation: createPromiseClient(RecommendationService, transport)` to `makeClients()`.
- [x] `src/lib/gateway/recommendations.ts` (new): `getRecommendations({ seedListingId?, context?,
      limit? })` wrapping `gateway().recommendation.recommend(...)`. The RPC returns listing
      ids + scores only (no card hydration, Rule 3), so it hydrates each id into a `ViewListing`
      via the existing `getListing(...)` listing client — the SAME path `searchListings` uses to
      resolve search hits (least-invasive hydration, reuses `ListingCard`). Preserves best-first
      rank order, drops ids that no longer resolve, and swallows `UNAVAILABLE` into an empty list
      so callers can hide the row.
- [x] `src/features/recommendations/actions.ts` (new): `getRecommendationsAction` Server Action
      wrapping the data layer with `ConnectError` handling (mirror
      `src/features/assistant/actions.ts`); always resolves with an empty list on failure.
- [x] `src/features/recommendations/RecommendationsRow.tsx` (new): async Server Component "Gợi ý
      cho bạn" row reusing `ListingGrid`/`ListingCard`; renders nothing when the list is empty.
- [x] `src/app/page.tsx`: render `<RecommendationsRow />` (home "for you", context `HOMEPAGE`,
      no seed).
- [x] `src/app/listing/[id]/page.tsx`: render `<RecommendationsRow seedListingId={listing.id} />`
      (PDP "similar", context `SIMILAR_ITEMS`), seeded with the current listing id.
- [x] `tsc --noEmit` green. Also `npm run check` (biome + tsc + vitest) and `npm test` GREEN —
      131 tests pass, incl. new `recommendations.test.ts` (5) + `recommendations/actions.test.ts` (3).
      Note: TS Connect stubs are a gitignored `buf generate` artifact; vendored the contract into
      `team-frontend/proto/...` and generated `src/generated/platform/recommendation/v1/*` locally
      (CI-equivalent) so the client registration + wrapper type-check against the real descriptor.

## E2E (platform-e2e)
- [x] `team-frontend/FEATURES.yaml`: add the `recommendations.for-you-row` feature — `persona:
      buyer`, `entry_route: /` + `related_routes: [/listing/{id}]`, `api.exercised:
      [recommendation.v1.Recommend]`, `services: [team-ai, team-gateway, team-frontend]`,
      `preconditions.login: buyer`, `ui.key_elements.recs_row: "text:Gợi ý cho bạn"`, acceptance
      mapping 1:1 to the two spec scenarios (populated row + graceful UNAVAILABLE). `status:
      planned` (schema-valid; id ASCII-normalized per the features schema pattern).
- [x] `platform-e2e`: added `tests/e2e/features/recommendations/recommendations.feature` (4
      scenarios mapping to the two spec scenarios + the gateway-forwarder scenario: buyer sees
      the row on home, seeded row on PDP, browser-never-calls-team-ai, graceful-empty
      degradation), `tests/e2e/step_definitions/recommendations_steps.py` + binder
      `test_recommendations.py` (reuses `buyer_steps`/`common_steps` — `a buyer is logged in`,
      `a listing has been seeded via the API`, `the buyer opens the seeded listing`, `I navigate
      to the "home" page`), a `RecommendationsRowComponent` page object (`src/pages/components/`,
      assertion-free — heading + `/listing/<id>` cards scoped to the `<section>`, wired onto
      `HomePage` + `ListingDetailPage`), and a `RecommendationService` gateway client
      (`src/api/services/recommendation_service.py` + barrel + `ServiceFactory.recommendation` +
      `RECOMMENDATION_RECOMMEND` endpoint) that exercises the real
      `RecommendationService/Recommend` path to prove the row is not mocked. Registered the
      `recommendations` gherkin marker in `pyproject.toml`. Runnable-without-stack checks GREEN:
      `pytest --collect-only` (55 tests, 0 errors; +4), gherkin parse, ruff, black,
      `python scripts/features.py` ("All manifests valid").
- [ ] Run against the local stack (`:3000` via gateway `:8080`, team-ai `RECS_ENABLED=true` with
      the training-job Qdrant/Redis data) — assert the row is populated from the real gateway path
      (team-ai logs receive `recommendation.recommend` with the gateway request-id), not a mock.
      DEFERRED (CI/stack+data-gated) — the green run needs the full stack (Go build of the gateway
      forwarder, not possible on host) PLUS a live team-ai `RECS_ENABLED=true` with populated
      Qdrant/Redis; the scenarios + wiring are authored and collect clean, but `pytest` green is
      blocked on that stack. Needs serve-recommendations-teamai live.
- [ ] Flip the shared recommendations coverage to `status: automated` (this closes out the
      `planned` entry left by serve-recommendations-teamai). DEFERRED — the platform-e2e scenario
      has landed (authored + collects clean) but is NOT yet green (CI/stack+data-gated). Left
      `recommendations.for-you-row` at `status: planned` with NO `covered_by`: the features-check
      gate errors on a `planned` entry that sets `covered_by`, and `automated` requires a
      resolving `covered_by`. Flip to `automated` + add
      `covered_by: recommendations/recommendations.feature::Logged-in buyer sees a recommendations
      row sourced from team-ai` once the scenario is green against team-ai `RECS_ENABLED=true`.

## Archive
- [ ] End-to-end verified: logged-in buyer loads the page → "Gợi ý cho bạn" row populated via
      gateway → team-ai; gateway `go build` clean; `tsc --noEmit` green. PARTIAL — `tsc --noEmit`
      (+ full `npm run check`/`npm test`) GREEN; gateway `go build` + the live end-to-end run are
      deferred to CI/the live stack.
- [ ] `make -C platform-e2e features-check` green and the recommendations scenario `automated`.
      PARTIAL — `python scripts/features.py` ("All manifests valid; every covered_by resolves ✓")
      and `pytest --collect-only` (0 errors) are GREEN now; flipping the scenario to `automated`
      is DEFERRED to the green stack run (CI/stack+data-gated).
- [ ] Archive after serve-recommendations-teamai is live (this change is the surface half and the
      one that brings the recommendations capability to `automated`).
