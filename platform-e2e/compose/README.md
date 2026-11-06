# Compose overlays for e2e

Each overlay is layered on the stack with an extra `-f` and keys services by name only (no
`container_name`), so it works under any compose project. Run `docker compose` from the
workspace root so relative build contexts resolve. The stack wrapper
(`dc-agora-ov.sh`, under `~/Library/Caches/ai-first-runs/agora-stack/`) is a plain
`docker compose -p agora -f docker-compose.yaml -f <isolation override> -f <overlays...> "$@"`; add
the overlays you need to its `-f` list.

| Overlay | Used by | Effect |
|---|---|---|
| `order-inventory.override.yaml` | order / inventory features | 20 s reservation TTL, 2 s sweep |
| `session-retention.override.yaml` | `auth/session_retention.feature` (`@retention_override`) | team-identity `SESSION_RETENTION=5s`, `LOGIN_HISTORY_RETENTION=5s`, `SESSION_SWEEP_INTERVAL=2s`; the `@retention_default` control runs without it |
| `payment-ledger.override.yaml` | payment ledger features | short ledger windows |
| `search-tombstones.override.yaml` | search read-model features | 30 s tombstone TTL |
| `llm-fake.override.yaml` | `ai/llm_*.feature` | adds the `llm-fake` provider, switches team-ai to `CHAT_BACKEND=llm_router` against it |
| `modelserve.override.yaml` | `modelserve/*.feature`, `search/hybrid_*.feature` | `tei-fake` and `modelserve-router` now live in the base compose (team-ai's RAG default needs them); this overlay publishes their host ports, names the images `agora-*`, sets `MAX_QUEUE_DEPTH=10` and points team-search and its indexer at the router (`MODEL_SERVER_URL`, `ENABLE_RERANKER=true`) |
| `llm-fake-breaker.override.yaml` | `ai/llm_breaker.feature` | `LLM_BREAKER_FAILURE_THRESHOLD=2` on team-ai, layered on `llm-fake`; applied by the scenario itself, restored in teardown |
| `rag-off.override.yaml` | `ai/assistant_grounding.feature` (demo-catalog scenario, `@destructive`) | team-ai with `RAG_ENABLED=false` and the listing indexer off; applied by the scenario itself, never part of the standing stack |
| `llm-fake-langfuse.override.yaml` | `ai/llm_tracing.feature` only | turns Langfuse on in team-ai, pointed at the fake's ingestion endpoint |

## `llm-fake.override.yaml` (change ai-path-resilience)

`platform-e2e/fakes/llm_fake/` is a stdlib-only, OpenAI-compatible fake provider that can be
told per request how to fail (`[[fake primary=429 fb1=ok fb2=ok]]`, modes `ok|429|500|400|hang|break2`),
records every request body (`GET /_requests?contains=`), and records Langfuse / OTLP ingestion
(`GET /_ingested?contains=`). The overlay builds and runs it as service `llm-fake`, publishes it
on `127.0.0.1:${LLM_FAKE_HOST_PORT:-18099}` and switches team-ai to:

| Setting | Value |
|---|---|
| `CHAT_BACKEND` / `CHAT_MODEL` / `CHAT_FALLBACK_MODELS` | `llm_router` / `openai:primary` / `openai:fb1,openai:fb2` |
| `OPENAI_BASE_URL` / `OPENAI_API_KEY` | `http://llm-fake:8099/v1` / `fake` |
| `CHAT_FIRST_TOKEN_TIMEOUT_SECONDS` / `LLM_MAX_RETRIES` | 2 / 1 (the default: the SDK's own retry; the chain walks each target once) |
| `LLM_BREAKER_FAILURE_THRESHOLD` / `LLM_BREAKER_COOLDOWN_SECONDS` | 1000 standing, 2 with `llm-fake-breaker.override.yaml` / 5 |
| `CHAT_SYSTEM_PROMPT` / `CHAT_HISTORY_MAX_TURNS` | a fixed e2e prompt / 4 |
| `QUOTA_CHAT_REPLIES_PER_WINDOW` | 15 (design says 30, see the file header); the quota is always on, counted in Redis |
| `RATE_LIMIT_PRINCIPAL_PER_MINUTE` | 200; the gRPC and per-principal HTTP rate limits are always on, in Redis |

Bring it up (stack already running, wrapper already carrying the overlay):

    DC=~/Library/Caches/ai-first-runs/agora-stack/dc-agora-ov.sh
    $DC up -d --build llm-fake
    $DC up -d --no-deps team-ai        # recreates team-ai with the new env
    curl -s localhost:18099/healthz    # {"status": "ok"}

The tests find the fake at `LLM_FAKE_URL` (default `http://localhost:18099`) and team-ai's logs in
the container named by `AI_CONTAINER` (default `agora-team-ai-svc`).

### Langfuse variant

Only the trace scenario needs it, and it applies it itself: `llm-fake-langfuse.override.yaml` is
passed as an extra `-f` to `up -d --no-deps team-ai`, and teardown recreates team-ai without it.
It is never part of the standing stack, which is why the scenario is `@destructive`.

### Unit tests for the fake

    platform-e2e/.venv/bin/pytest -p no:cacheprovider platform-e2e/unit/test_llm_fake.py -q

## `modelserve.override.yaml` (changes add-platform-modelserve, add-hybrid-retrieval-platform)

`platform-e2e/fakes/tei_fake/` is a stdlib-only, deterministic stand-in for Hugging Face TEI: `POST /embed`
(dim 384, a hashed bag of concepts, so overlapping texts are closer and `<stem>zalias` equals `<stem>`),
`POST /rerank` (token overlap), a canned vLLM (`/v1/chat/completions`, `/generate`) and a request log
(`GET /_requests?contains=&kind=`). `[[fake status=500]]`, `[[fake delay=3000]]`, `[[fake rerank=reverse]]`,
`[[fake rerank_status=500]]` in a text or query drive the failure scenarios. The base compose
(`docker-compose.services.yaml`) builds `tei-fake` and `modelserve-router` (Redis DB 4 is set there). The overlay
publishes them on host port `${TEI_FAKE_HOST_PORT:-18110}` and `${MODELSERVE_HOST_PORT:-18100}`, sets
`MAX_QUEUE_DEPTH=10` on the router and sets `MODEL_SERVER_URL` on team-search and team-search-indexer. Bring it up:

    DC=~/Library/Caches/ai-first-runs/agora-stack/dc-agora-ov.sh
    $DC -f platform-e2e/compose/modelserve.override.yaml up -d --build tei-fake modelserve-router
    $DC -f platform-e2e/compose/modelserve.override.yaml up -d --no-deps team-search team-search-indexer

Every later `$DC` call for these services needs the same `-f` (or add it to the wrapper's list).

## Environment per test group

Each row lists what a group needs on top of the base stack (infra plus the team-* services, the buyer
app on :3000 and the seller/admin apps on :3001/:3002). `DC` is the stack's compose wrapper
(`docker compose -p <project> -f docker-compose.yaml <overlays...>`, run from the workspace root);
`EXTRA` is the group's overlay list, given to every `DC` call and exported as part of `DC_WRAPPER`
so scenarios that recreate a service keep it. After recreating a service, `docker restart
<gateway container>` and wait ~10 s. Restore each recreated service without the overlay afterwards
(`DC up -d --no-build --no-deps --force-recreate <svc>`). Run each file one at a time:
`python scripts/asdlc/gate_digest.py run --cwd . -- "<e2e env> python -m pytest -q platform-e2e/tests/e2e/step_definitions/<file>"`,
where `<e2e env>` names the project's containers (`POSTGRES_CONTAINER`, `REDIS_CONTAINER`,
`TEAM_*`/`UIF_*`/`PLP_*` names, `STACK_NETWORK`, `ANALYTICS_VOLUME`, `FEATURESTORE_VOLUME`,
`KAFKA_BROKERS=localhost:19092`, `DC_WRAPPER`) and, on a project not named `agora`,
`UIF_COMPOSE_PROJECT=<project>`.

| Group | Test files | Services | Overlays | Env vars | Seed step | Run |
|---|---|---|---|---|---|---|
| GA4 purchase relay | `test_ga4_purchase_relay.py` | redpanda, `ga4-relay` (+ order, payment, promotion, sharing, engagement for the browser scenarios) | `ga4-relay-e2e.override.yaml` on `ga4-relay` | `GA4_MOCK_PORT=18120` | topic `analytics.ga4-relay.dlq` exists (redpanda-init); consumer group lag 0 | the file (~6.5 min) |
| LLM routing | `test_apr_breaker.py`, `test_apr_prompt.py`, `test_apr_quota.py`, `test_apr_routing.py`, `test_apr_tracing.py`, `test_ar2_stream_lifetime.py` | `llm-fake`, team-ai | `llm-fake.override.yaml` on `llm-fake` and team-ai, also in `DC_WRAPPER` | `LLM_FAKE_URL` (default `http://localhost:18099`) | - | each file |
| UI failure paths (uif) | `test_uif_account.py`, `test_uif_cart.py`, `test_uif_discovery.py`, `test_uif_foundation.py`, `test_uif_orders.py`, `test_uif_pdp.py`, `test_uif_seller.py` | base stack, seller app | - | `UIF_COMPOSE_PROJECT=<project>`, `UIF_<SERVICE>_CONTAINER` | - | each file (stops and restores real containers) |
| Payment ledger | `test_d2_seller_orders_money.py`, `test_pear_authz_payout.py`, `test_plp_payout_holdback.py`, `test_plp_refund_deduction.py`, `test_seller_payout.py` | base stack | `payment-ledger.override.yaml` on team-payment | `PLP_*_CONTAINER` | - | each file |
| GA4 export import | `test_ga4_export_import.py` | team-analytics | `ga4-fixture.override.yaml` on team-analytics | `TEAM_ANALYTICS_CONTAINER`, `WAREHOUSE_DB_PATH=/data/analytics.duckdb` | - | the file |
| Session retention | `test_session_retention.py` | team-identity | `session-retention.override.yaml` on team-identity, except `-k default_stack_keeps_rows`, which runs without it | - | - | the file with the overlay, then `-k default_stack_keeps_rows` without it |
| Search tombstones | `test_srm_deletes.py` | team-search-indexer | `search-tombstones.override.yaml` on team-search-indexer | - | - | the file |
| Metrics | `test_cockpit_metrics.py`, `test_session_revocation.py` | `prometheus`, `otel-collector` | - | `PROMETHEUS_URL` (default `http://localhost:9090`) | - | each file; the denylist expiry scenario can fail right after a gateway restart, rerun the file |
| Git-backed checks | `test_promotion_admin_vouchers.py` | base stack | - | - | - | run from a git checkout (the tests shell out to `git`) |
| Seller categories | `test_seller.py` | base stack, seller app | - | - | `docker exec -i <postgres container> psql -U listing_svc -d listing_db < platform-e2e/scripts/seed_base_categories.sql` (inserts the base categories, `ON CONFLICT DO NOTHING`, touches nothing else) | the file |
| UI catalogue | `test_ui_components.py` | base stack, buyer app under `next dev` (`/dev/ui` only exists there) | - | `BASE_URL=http://localhost:<dev port>` | `cd team-frontend && NEXT_PUBLIC_GATEWAY_URL=http://localhost:8080 GATEWAY_URL=http://localhost:8080 NEXT_PUBLIC_MEDIA_BASE_URL=http://localhost:9000/listing-images npm run dev -- -p <dev port>` | the file |
| Modelserve, hybrid search, RAG grounding | `test_modelserve.py`, `test_hybrid_retrieval.py`, `test_assistant_grounding.py` | qdrant, `tei-fake`, `modelserve-router`, team-search, team-search-indexer, team-ai with RAG on | `modelserve.override.yaml`; no `rag-off.override.yaml` in the standing stack | `MODELSERVE_URL` (default `http://localhost:18100`), `MS_ROUTER_CONTAINER` | - | each file |
| MLflow registry | `test_model_registry.py` | `postgres-mlflow`, `minio`, `mlflow` | `minio-bitnami.override.yaml` where `minio/minio` cannot be pulled | `MLFLOW_CONTAINER`, `MODELREGISTRY_COMPOSE_PROJECT=<project>` | `DC run --rm --no-deps minio-init` | the file |
| Recommendations and recsys/featurestore | `test_recommendations.py`, `test_tpr_trained_recs_local.py`, `test_ai_auth.py`, `test_buyer_full_funnel_journey.py`, `test_d1_discovery.py`, `test_d1_pdp.py`, `test_ple_placement_engine.py`, `test_rss_serving_safeguards.py`, `test_fia_item_attributes.py`, `test_fir_recsys_item_attributes.py`, `test_fsd_governed_datasets.py`, `test_fsm_materialization.py`, `test_gbl_ranker_serving.py`, `test_mla_nearline_ctr.py`, `test_mlr_ml_recsys.py`, `test_ofb.py`, `test_ple_home_feed.py`, `test_recsys_auto_train.py`, `test_rgp_generation_publish.py`, `test_rgt_gbdt_ranker.py`, `test_rss_recommendation_attribution.py`, `test_ssa_serving_switch_atomicity.py` | the modelserve row plus the MLflow row, redis published on the host, `platform-recsys-nearline` | `modelserve.override.yaml`, `minio-bitnami.override.yaml` | `FSM_REDIS_PORT=6379` (the stack's Redis host port), `RECSYS_IMAGE`, `FEATURESTORE_IMAGE`, `ANALYTICS_VOLUME`, `FEATURESTORE_VOLUME`, `FILE_TIMEOUT` >= 2400 s | one serving generation: `DC --profile featurestore run --rm --no-deps featurestore-job`, `DC --profile featurestore run --rm --no-deps featurestore-dataset`, `DC --profile jobs run --rm --no-deps platform-recsys`; for `test_recsys_auto_train.py` at least 24 distinct prices among viewed listings: `POSTGRES_CONTAINER=<postgres container> GATEWAY_URL=http://localhost:8080 python platform-e2e/scripts/seed_price_views.py`, then wait for the 300 s Parquet export | each file |
| Consent and context | `test_recsys_consent_context.py` | the recommendations row | the recommendations overlays plus `ga4-fixture.override.yaml` on team-analytics | as the recommendations row | as the recommendations row | the file |
