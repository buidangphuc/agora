## 1. Contract (serial, first)

- [x] 1.1 platform-core: `RecommendResponse.placement_id = 3`, `request_id = 4`; vendor into team-ai, team-gateway, team-frontend and regenerate; verify `buf lint`, `buf breaking` (additive), all three build

## 2. Code track (one agent per repo, worktree each)

- [x] 2.1 team-ai: fallback in the service (D1), cold start from the popular list (D2), placement/request id + `recs.served` log (D3), `RedisFeatureStore` and tie-break boost (D4), boot refusal + shared provider helper for both entrypoints (D5), `RECS_FEATURESTORE_REDIS_URL`; verify unit tests for each (cache error → popular, both down → empty OK, scroll removed, ids distinct, log fields, feature tie-break and error degrade, refusal, run_grpc registers Recommend), `make check` + `make test`
- [x] 2.2 team-frontend: recommendation rows use `request_id` / `placement_id` for beacons (D3); verify vitest and `npm run check`
- [x] 2.3 Root compose: `RECS_FEATURESTORE_REDIS_URL=redis://redis:6379/2` for team-ai; verify `docker compose config`

## 3. E2E track (platform-e2e; team-ai and team-frontend FEATURES.yaml)

- [x] 3.1 One FEATURES.yaml acceptance line per scenario, `planned` until merged; verify `features-check`
- [x] 3.2 `recommendations/serving_safeguards.feature`: Redis outage (destructive), popular cold start (destructive: publishes a known popular list), memory backend refusal (destructive boot guard), run_grpc entrypoint (destructive: a throwaway team-ai container plus a throwaway gateway), distinct request ids, feature tie-break (destructive: writes `fs:item_popularity` keys for a fixture); verify they pass
- [x] 3.3 `frontend/recommendation_attribution.feature`: homepage beacons carry the server request id; verify it passes

## 4. Review and verify

- [x] 4.1 `contract-boundary-reviewer` (additive proto, team-ai reads only featurestore keys and its own cache, frontend via gateway) and `auth-scope-reviewer` (request id is not an auth token; `recs.served` logs no PII beyond ids); verify no blocking finding
- [x] 4.2 Gate: `openspec validate recs-serving-safeguards --strict`, `features.py --strict`, `spec_sync.py recs-serving-safeguards --strict`, `repo_doctor`
- [x] 4.3 Full e2e suite green twice in the parallel lane plus the destructive lane

## Evidence (2026-10-09)

- Code:
  - proto 61d13534, vendoring caea7235 (`buf breaking` against 85b1c635 is clean);
  - team-ai 295a4422;
  - frontend efaf5214, d84a2925 (review fix: the fallback placement names are the served ones);
  - compose `RECS_FEATURESTORE_REDIS_URL`.
- Spec corrections:
  - A collection contract mismatch stays `UNAVAILABLE` (24c229c1). It is a deployment error, not an outage.
  - Recommendation beacons carry the served placement (`similar_items`, `home_feed`). The in-flight ui-phase-product-detail and ui-phase-discovery specs, the PDP scenario and FEATURES were renamed to match.
- e2e:
  - spec_sync 7/7.
  - rss scenarios 6/6 destructive and 1/1 non-destructive.
  - Full suite: 469/469 twice in the parallel lane (`-n 4`), then destructive lane 63/63.
  - After the frontend fallback fix, recommendations and funnel 15/15, attribution 1/1.
- e2e defects fixed (5653d523):
  - The memory-backend boot guard borrowed an env that tripped another guard first.
  - The image ships no `scripts/`, so run_grpc now runs as `python -m scripts.run_grpc` from the mounted repo scripts.
- Reviews: auth-scope found no blocking issue. Follow-ups:
  - The `RECS_FEATURESTORE_REDIS_URL` DB index is not validated in code.
  - Write access to featurestore Redis DB 2 is a ranking-integrity boundary; worth an ops note.
  - `recs.served` log volume scales with traffic.

## 5. Archive

- [ ] 5.1 `openspec archive recs-serving-safeguards`; retire `recommendations-end-to-end` if change 8 is also archived, else after it; verify `openspec list`
