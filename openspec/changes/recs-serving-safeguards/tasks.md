## 1. Contract (serial, first)

- [ ] 1.1 platform-core: `RecommendResponse.placement_id = 3`, `request_id = 4`; vendor into team-ai, team-gateway, team-frontend and regenerate; verify `buf lint`, `buf breaking` (additive), all three build

## 2. Code track (one agent per repo, worktree each)

- [ ] 2.1 team-ai: fallback in the service (D1), cold start from the popular list (D2), placement/request id + `recs.served` log (D3), `RedisFeatureStore` and tie-break boost (D4), boot refusal + shared provider helper for both entrypoints (D5), `RECS_FEATURESTORE_REDIS_URL`; verify unit tests for each (cache error → popular, both down → empty OK, scroll removed, ids distinct, log fields, feature tie-break and error degrade, refusal, run_grpc registers Recommend), `make check` + `make test`
- [ ] 2.2 team-frontend: recommendation rows use `request_id` / `placement_id` for beacons (D3); verify vitest and `npm run check`
- [ ] 2.3 Root compose: `RECS_FEATURESTORE_REDIS_URL=redis://redis:6379/2` for team-ai; verify `docker compose config`

## 3. E2E track (platform-e2e; team-ai and team-frontend FEATURES.yaml)

- [ ] 3.1 One FEATURES.yaml acceptance line per scenario, `planned` until merged; verify `features-check`
- [ ] 3.2 `recommendations/serving_safeguards.feature`: Redis outage (destructive), popular cold start (destructive: publishes a known popular list), memory backend refusal (destructive boot guard), run_grpc entrypoint (destructive: a throwaway team-ai container plus a throwaway gateway), distinct request ids, feature tie-break (destructive: writes `fs:item_popularity` keys for a fixture); verify they pass
- [ ] 3.3 `frontend/recommendation_attribution.feature`: homepage beacons carry the server request id; verify it passes

## 4. Review and verify

- [ ] 4.1 `contract-boundary-reviewer` (additive proto, team-ai reads only featurestore keys and its own cache, frontend via gateway) and `auth-scope-reviewer` (request id is not an auth token; `recs.served` logs no PII beyond ids); verify no blocking finding
- [ ] 4.2 Gate: `openspec validate recs-serving-safeguards --strict`, `features.py --strict`, `spec_sync.py recs-serving-safeguards --strict`, `repo_doctor`
- [ ] 4.3 Full e2e suite green twice in the parallel lane plus the destructive lane

## 5. Archive

- [ ] 5.1 `openspec archive recs-serving-safeguards`; retire `recommendations-end-to-end` if change 8 is also archived, else after it; verify `openspec list`
