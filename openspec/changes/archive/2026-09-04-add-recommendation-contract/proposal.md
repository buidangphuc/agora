## Why

The recommendation line now has a data foundation — `platform.analytics.v1.TrackingEvent`
(the just-shipped analytics contract) lands in the behavioral warehouse via the new
`team-analytics` warehouse writer (`team-analytics/internal/warehouse/*`), and a later offline
Spark job (`build-als-training-job`) turns that behavior into recommendation artifacts in Qdrant +
Redis. What is still missing is the **serving contract**: the online path (serving in `team-ai`,
routing in `team-gateway`, surfacing in `team-frontend` — the other half of this feature) has no
RPC to implement. This change adds that contract, and only the contract: a
`platform.recommendation.v1.RecommendationService` with a single `Recommend` RPC, defined once in
`platform-core/packages/proto` (Rule 4 — contract is the source of truth). It is purely additive
(a brand-new proto file/package), so it is non-breaking and touches no existing service.

## What Changes

- **platform-core** (`packages/proto`, **new proto file** — flagged cross-service contract change)
  — add `platform/recommendation/v1/recommendation.proto` defining:
  - `service RecommendationService` with `rpc Recommend(RecommendRequest) returns (RecommendResponse)`;
  - `RecommendRequest` = caller identity (`user_id` for an authenticated user OR `anonymous_id` for
    a cookie/device visitor), optional `seed_listing_id` (PDP "similar items" / cart anchor), a
    `RecommendationContext` enum (where the widget is shown, so the server can pick a strategy), and
    a `limit`;
  - `RecommendResponse` = a ranked `repeated RecommendedItem` (each `listing_id` + `score` + 1-based
    `rank`) plus a `model_version` string tying the ranking back to the offline artifact that
    produced it.
  - Modeled on the existing `search.proto`/`ai.proto` conventions: reuses `platform.common.v1`, uses
    the enum-value-prefix lint rule (`RECOMMENDATION_CONTEXT_*`), and returns **listing ids + scores**
    (not hydrated cards) so the recsys/serving side never owns listing content — the serving layer
    hydrates cards from `team-search`/`team-domain` (Rule 3, DB-per-service).
- **E2E (platform-e2e)** — no user-facing behavior ships in this change (contract only), so no
  `.feature`/journey is added here; the end-to-end assertion arrives with the serving change
  (`serve-recommendations-teamai`) that implements the RPC.

## Non-goals

- **No implementation.** No `team-ai` handler, no gateway forwarder, no frontend surface — those are
  the serving/surfacing half of the feature (`serve-recommendations-teamai` + the gateway/frontend
  change), drafted separately.
- **No consumer regen / re-vendor.** This change only writes the `.proto`; vendoring + `buf generate`
  into the consuming repos happens when the RPC is implemented.
- **No changes to any existing proto** (`search`, `ai`, `analytics`, `common`, …) — this is a new,
  isolated package. No renumbering/removal anywhere.
- **No offline training logic** — that is `build-als-training-job` (the other Wave 2 change).
- **No streaming/realtime recommendation RPC** — single-shot request/response only.
