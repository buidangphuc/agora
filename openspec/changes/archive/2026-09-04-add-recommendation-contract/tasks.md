# Tasks

> Planning only — every box is unchecked. buf is NOT installed on the host and generated proto is
> gitignored, so `buf lint`/`buf breaking`/`buf generate` run in Docker/CI. This change writes the
> `.proto` only; no consumer re-vendor/regen (that ships with the serving change).

## 1. Code — platform-core (proto contract, additive)

- [x] Add `platform/recommendation/v1/recommendation.proto`:
      `syntax = "proto3"; package platform.recommendation.v1;`; a leading comment naming the owner
      (online serving lands in `serve-recommendations-teamai`) and the offline artifact producer
      (`build-als-training-job`).
      NOTE: the `platform/common/v1/common.proto` import was dropped — the message shapes below use
      only plain scalars, and buf STANDARD lint (`IMPORT_USED`) fails an unused import. Neighbors
      import common only when they reference a common type (e.g. `search.proto` → `PageRequest`).
- [x] `service RecommendationService { rpc Recommend(RecommendRequest) returns (RecommendResponse); }`.
- [x] `enum RecommendationContext` with enum-value-prefixed members:
      `RECOMMENDATION_CONTEXT_UNSPECIFIED = 0`, `_HOMEPAGE = 1` (for-you feed),
      `_SIMILAR_ITEMS = 2` (PDP), `_CART = 3`, `_SEARCH = 4`.
- [x] `message RecommendRequest { string user_id = 1; string anonymous_id = 2;
      string seed_listing_id = 3; RecommendationContext context = 4; int32 limit = 5; }`
      (documented: `user_id` xor `anonymous_id`; `limit` 0 = server default).
- [x] `message RecommendedItem { string listing_id = 1; float score = 2; int32 rank = 3; }` and
      `message RecommendResponse { repeated RecommendedItem items = 1; string model_version = 2; }`.
- [x] Field-doc every field; keep the response to ids+scores (no card hydration — Rule 3).
- [x] `buf lint` + `buf breaking` green: STANDARD lint passes (`ENUM_VALUE_PREFIX` satisfied),
      `buf breaking --against '.git#branch=main,subdir=packages/proto'` passes (new-package addition
      is non-breaking). Run locally via `npx @bufbuild/buf` from `packages/proto` (offline from cache).
- [x] `buf format` clean; `buf format --diff` reports no changes. File style matches
      `search.proto`/`ai.proto`.

## E2E (platform-e2e)

- [ ] None in this change. It is contract-only with **no user-facing behavior**; there is no
      `.feature`/journey and no `FEATURES.yaml` entry to add here. The end-to-end scenario (a buyer
      seeing recommendations) is authored with the serving change `serve-recommendations-teamai`,
      which implements this RPC and owns the recommendations capability's user-facing acceptance.

## Archive

- [ ] Confirm `buf lint`/`buf breaking` green in CI and the new package is importable by the
      serving repos before they vendor it. (Deferred: verified locally via `npx @bufbuild/buf`;
      Docker/CI confirmation + consumer `buf generate`/re-vendor ship with `serve-recommendations-teamai`.)
- [ ] `openspec archive add-recommendation-contract` — after the proto is merged and the serving
      change is ready to consume it. (Deferred to post-merge.)
