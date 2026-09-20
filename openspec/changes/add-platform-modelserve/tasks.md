# Tasks

## 1. Code — platform-modelserve (router + cache + runtime integration)
- [x] Initialize `platform-modelserve/` repo layout: `pyproject.toml`, `requirements.txt`, `requirements-dev.txt`, `Makefile`, `Dockerfile`, `.gitignore`, `.env.example`, `FEATURES.yaml`, `README.md`.
- [x] Implement `modelserve.config`: environment variables with defaults (`ROUTER_PORT`, `TEI_EMBED_URL`, `TEI_RERANK_URL`, `VLLM_URL`, `REDIS_URL`, `EMBED_CACHE_TTL_SECONDS`, `MAX_QUEUE_DEPTH`, `MODEL_VERSION`).
- [x] Implement `modelserve.model_version`: key hashing `hash(model_version + text)` and version tracking.
- [x] Implement `modelserve.cache`: async Redis embedding cache with multi-key get/set support.
- [x] Implement `modelserve.admission`: queue depth tracking and 429 backpressure.
- [x] Implement `modelserve.router`: FastAPI application supporting `/embed`, `/v1/embeddings`, `/rerank`, `/v1/chat/completions`, `/generate`, `/healthz`, `/metrics`.
- [x] Write unit & integration tests in `platform-modelserve/tests/`:
  - `test_config.py` & `test_env_drift.py`
  - `test_cache.py`
  - `test_admission.py`
  - `test_router.py`
  - `test_contract_conformance.py` (matching `team-ai.app.modules.ai.rag.embeddings._extract_vectors`)
- [x] Provide `platform-modelserve/docker-compose.local.yaml` for local development.

## 2. Infrastructure & GitOps — platform-core & platform-gitops
- [x] Document ADR-0011 in `platform-core/docs/ADR/0011-model-serving.md`.
- [x] Update `AGENTS.md` and `openspec/config.yaml` to include `platform-modelserve`.
- [x] Create `platform-gitops/platform/infra/modelserve.yaml` (Deployments, Services, PVC, and NetworkPolicy).
- [x] Create `platform-gitops/argocd/apps/platform-modelserve.yaml` (ArgoCD Application manifest).

## 3. Verification & Gate
- [x] Run `openspec validate add-platform-modelserve --strict` to ensure specification passes.
- [x] Run `pytest` test suite in `platform-modelserve/` to verify 100% green tests.
- [x] Verify Docker Compose configuration for `platform-modelserve`.
