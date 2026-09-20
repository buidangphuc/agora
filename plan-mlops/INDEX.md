# Plan Index — MLOps / LRS track

Nguồn sự thật cho track recsys-ML. Cập nhật cột Status sau mỗi task. Status: `todo | running | review | done | failed`.

## Nền tảng đã có (audit 2026-09-20, verified trong code)

| Có | Ở đâu |
|---|---|
| Warehouse layer | `team-analytics` Kafka→warehouse, có test |
| Training job tái lập được | `platform-recsys` Spark ALS, `_FIELDS` drift gate |
| Model versioning + atomic swap | `model_version.py`, prune theo generation |
| Scheduled retraining | `platform-gitops/platform/recsys/cronjob.yaml` |
| Graceful degradation | TTL 48h > nhịp nightly, `recs:v1:popular` floor |
| Eval harness (LLM) | `team-ai/app/modules/ai/evals/` |
| LLM observability | Langfuse: trace, prompt versioning, score |
| Progressive delivery | Argo Rollouts + `analysistemplate.yaml` |
| Behavior stream realtime | `analytics.events` — giàu (position, search_query) |

| Đã hoàn thành | Ở đâu |
|---|---|
| CI (chỉ 1 workflow toàn workspace) | P1-T3 (`add-ml-ci-workflows`) |
| Attribution per-placement | P1-T1 (`add-placement-attribution-contract`) |
| Offline eval cho recsys | P1-T2 (`add-recsys-offline-eval`) |
| Model registry / promotion gate | P1-T4 (`add-model-registry-promotion`) |
| Model serving | P2-T2 (`add-platform-modelserve`) |
| Kafka consumer trong team-ai (index RAG trống) | P2-T3 (`add-ai-content-indexer`) |
| Speed layer (stream chảy vào batch sink) | P2-T4 (`add-recsys-nearline-signals`) |
| Placement engine ADR & implementation | P2-T1 & P2-T5 (`add-placement-engine`) |
| Ranker (GBDT point-wise/pairwise) | P3-T1 (`add-gbdt-ranker`) |
| Feature store (online + offline + parity) | P3-T2 (`add-platform-featurestore`) |
| Two-tower retrieval (cold-start recovery) | P3-T3 (`add-two-tower-retrieval`) |
| Drift monitoring (PSI + Prometheus alerts) | P3-T4 (`add-recsys-drift-monitoring`) |

**Định vị**: MLOps Level 2 (end-to-end automated pipelines, feature store, serving router, CI/CD validation gates, and drift monitoring).

---

## Phases & Tasks

| Phase | Task file | OpenSpec Change ID | Repo (writes) | Verify | Status |
|---|---|---|---|---|---|
| 1 | `phase1-task1-attribution-contract` | `add-placement-attribution-contract` | platform-core proto + team-gateway | buf lint/breaking; e2e tracking | **done** |
| 1 | `phase1-task2-recsys-eval-harness` | `add-recsys-offline-eval` | platform-recsys/recsys/evals | make eval | **done** |
| 1 | `phase1-task3-ci-ml-repos` | `add-ml-ci-workflows` | platform-recsys/.github, team-ai/.github | PR nháp phải đỏ | **done** |
| 1 | `phase1-task4-model-registry-promotion` | `add-model-registry-promotion` | platform-recsys + gitops + ADR-0014 | dry-run promotion | **done** |
| 2 | `phase2-task1-adr-placement-engine` | `add-placement-engine-adr` | platform-core/docs/ADR (doc) | đọc chéo AGENTS.md §3 | **done** |
| 2 | `phase2-task2-modelserve-stage0` | `add-platform-modelserve` | platform-modelserve (new) | pytest + compose smoke | **done** |
| 2 | `phase2-task3-content-indexer` | `add-ai-content-indexer` | team-ai/modules/messaging/indexer | publish→Qdrant < 10s | **done** |
| 2 | `phase2-task4-nearline-signal-layer` | `add-recsys-nearline-signals` | platform-recsys/recsys/nearline | event→Redis < 10s | **done** |
| 2 | `phase2-task5-placement-engine` | `add-placement-engine` | team-ai/modules/business/recommend | pytest 2 placement | **done** |
| 3 | `phase3-task1-gbdt-ranker` | `add-gbdt-ranker` | platform-recsys/ranker + team-ai/ranking.py | NDCG@10 > baseline | **done** |
| 3 | `phase3-task2-feature-store` | `add-platform-featurestore` | platform-featurestore (new) | parity test offline↔online | **done** |
| 3 | `phase3-task3-two-tower-retrieval` | `add-two-tower-retrieval` | platform-recsys/two_tower | make eval vs ALS | **done** |
| 3 | `phase3-task4-drift-monitoring` | `add-recsys-drift-monitoring` | platform-recsys/monitoring + gitops | alert nổ đúng | **done** |

---

## Dependency Notes

- **Phase 1 là điều kiện cần của mọi thứ sau**: Không có attribution (T1) + eval (T2) thì "cải thiện" là mệnh đề không kiểm chứng được. T4 cần T2 xong trước.
- **Phase 1 chạy song song được** trừ T4 (chờ T2). Write-set rời nhau.
- **Phase 2**: T1 (ADR) trước T5. T2 trước T3 (/embed phải sống — `platform-modelserve` đã hoàn thành). T3 ∥ T4 (khác repo). T5 cần T1+T3+T4.
- **Phase 3**: T1 độc lập, làm trước — ranker là nút thắt, không phải retrieval. T2 chặn T3 (item tower cần feature nhất quán). T4 sau cùng.
- **ALS không bị xoá** cho tới P3-T3 thắng trên metric. Nó là baseline duy nhất; xoá trước khi đo được là tự bịt mắt. Khi thắng → xoá một dòng YAML.
