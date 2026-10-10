## 1. Code — team-analytics

- [x] 1.1 Image prepares a nonroot-owned `/data`; compose `DUCKDB_PATH=/data/analytics.duckdb`; verify a container writes to a fresh volume
- [x] 1.2 Periodic atomic Parquet export (`PARQUET_EXPORT_PATH`, `PARQUET_EXPORT_INTERVAL_SECONDS`, default off) wired in main, plus env-example sync; verify unit tests (atomic replace, disabled by default, failure keeps running) and `go test ./...`

## 2. Code — team-ai

- [x] 2.1 Add the `qdrant-client` dependency. Map seed listing ids to uuid5 point ids, with a pin test against the platform-recsys namespace. Return the payload `listing_id`. Verify unit tests with a fake client, and run the repo test suite

## 3. Compose and rollout

- [x] 3.1 `platform-recsys` job service (profile `jobs`), team-ai `RECS_ENABLED=true` and `RECS_BACKEND=qdrant`; verify the training job completes against the agora stack and Redis/Qdrant hold the artifacts
- [x] 3.2 Move the collected events from `/tmp` to the volume once; verify the row count is unchanged

## 4. E2E — platform-e2e

- [x] 4.1 Remove the strict xfail on "Home page shows AI recommendations with viewable impressions"; verify green against the agora stack (3 runs), FEATURES entries
- [x] 4.2 Run `openspec validate serve-trained-recs-locally --strict`; verify it is valid

## 5. Leakage-free evaluation — platform-recsys

- [x] 5.1 Per-user leave-last-new-item-out holdout, a separately trained evaluation model, seen-item exclusion, `eval_protocol` stamp, and a gate that does not compare across protocols; verify a Spark test that the evaluation training set holds no target pair, the holdout unit tests, the registry test, and a run on the agora stack

## Notes (found while applying)

- team-ai also never registered RecommendationService: the proto was not vendored, and the servicer used a guessed contract (`recommendations` field, string context, unknown scope). Fixed as part of 2.1 and gated on `listing.read`.
- A failed startup collection check stuck forever. It is now re-checked every 30 s.
- Qdrant 1.9 → 1.19, to match qdrant-client 1.19 (query API).
- platform-recsys: bitnami/spark is gone (rebuilt on python-slim + JRE). Evaluation always ran on an empty set (TIMESTAMP_NTZ cast). PROMOTION_FORCE was added for reseeding the stores.
- Out of scope: the 6 offline-pipeline scenarios in recommendations.feature need pandas and platform-recsys in the e2e venv (the runner ignores that module).

## Evidence (2026-10-10)

- Code and unit tests: each repo's `make check` / test suite was green at merge (see the commit bodies).
- e2e after rebuilding team-ai, team-search (server and indexer), gateway, frontend and the recsys image, with
  platform-recsys-nearline and the modelserve overlay (fake TEI + router) running:
  - ML scenarios: 23/23, twice;
  - modelserve, hybrid and taxonomy: 27/27, three times;
  - placement and serve-trained scenarios: green three times.
- Scenarios that cannot be produced end to end carry a VERIFIED BY line in the spec and a not-testable FEATURES
  entry.
- spec_sync --strict reports e2e-ready.

- Final gate (2026-10-10): parallel lane 775/775 (w10-par) and 775/776 (w9-par; its one failure was the gateway-wide denylist gauge scenario, moved to the serial lane in e8373e00). Destructive lane 100/101 (w9-dfull); its one failure, backpressure, was fixed in 7f4ae454 and 4d325f2f and then passed twice in the outage-then-backpressure order.
