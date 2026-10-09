# platform-modelserve

Internal ML serving router (ADR-0011). A small Python 3.12 / FastAPI service on `:8100` that sits
in front of upstream inference runtimes (Hugging Face Text Embeddings Inference and vLLM). It adds
an HTTP contract adapter, a Redis embedding cache and a global in-flight limit with 429 backpressure.

- **Bounded context:** platform (ML inference plumbing). It owns no business data, no database and no
  proto contract. The only state is the Redis embedding cache.
- **Status:** the router is **not** part of the root `docker-compose.services.yaml` (no modelserve,
  TEI or vLLM service there). It is opt-in, started from this repo's own `docker-compose.local.yaml`.
- **Runtimes are not provisioned here.** Only a TEI embed container is defined
  (`docker-compose.local.yaml`). `/rerank` and `/generate` need a TEI reranker and a vLLM server that
  you supply yourself.

## 1. Contract

HTTP/JSON only. **There is no authentication or scope check on any endpoint** (see Known gaps).

| Endpoint | Behaviour | Upstream |
|---|---|---|
| `POST /embed` | Body `{"texts": [...]}` or `{"input": str \| [str]}`. Returns `{"embeddings": [[...]]}`. Empty input returns `{"embeddings": []}`. | TEI embed |
| `POST /v1/embeddings` | Same logic, returns OpenAI shape `{"object":"list","data":[{"object":"embedding","index","embedding"}],"model"}`. | TEI embed |
| `POST /rerank` | Body `{"query", "texts", "top_k"?}`, forwarded as is. Not cached. | TEI rerank `/rerank` |
| `POST /generate`, `POST /v1/chat/completions` | Body forwarded unchanged to the same path on vLLM. Upstream status and JSON body are passed through. Not streamed (the response is read with `res.json()`). | vLLM |
| `GET /healthz` | `{"status":"ok"}`. Does not check Redis or upstreams. | none |
| `GET /metrics` | Prometheus text. | none |

Errors: `429` + `Retry-After: 2` when saturated; `503` when an upstream is unreachable; `502` when TEI
returns non-200, an unexpected shape or the wrong vector count (embed/rerank only).

Embed upstream details: cache misses are sent to `{TEI_EMBED_URL}/embed` as `{"inputs": [...]}`. If that
returns non-200, the router retries once at `/v1/embeddings` with `{"input": [...]}`. Accepted upstream
shapes: a bare list, `{"embeddings": ...}`, or OpenAI `{"data": [{"embedding": ...}]}`. The request
`model` field is not sent upstream; it only appears in the `/v1/embeddings` response.

**Known consumers** (both opt-in, neither is wired by default):

| Consumer | Setting | Notes |
|---|---|---|
| team-ai | `RAG_EMBED_BACKEND=model_server`, `RAG_EMBED_SERVER_URL=http://modelserve-router:8100`, `RAG_EMBED_DIM` | Defaults are `mock` and an empty URL. Calls `POST /embed` with `{"texts": [...]}`. Dim default 384 must match the TEI model (the local compose uses bge-small, 384). |
| team-search | `MODEL_SERVER_URL` (default `http://localhost:8100`), `ENABLE_RERANKER` (default `false`) | Calls `/embed` and, if enabled, `/rerank`, with 2 s timeouts. Embed errors fall back to lexical search. |

`platform-recsys` has no reference to modelserve.

## 2. Events

None. No Kafka or RabbitMQ producers or consumers.

## 3. Data

No database and no migrations. Redis (`REDIS_URL`) holds the embedding cache:

- Key: `modelserve:embed:{model_version}:{sha256(text)}` (`modelserve/model_version.py`).
- Value: JSON array of floats, written with `SET ... EX EMBED_CACHE_TTL_SECONDS` in a pipeline; reads use `MGET`.
- Bumping `MODEL_VERSION` makes all old entries unreachable (they expire by TTL).
- Redis failures are swallowed and logged (fail-open): the router just treats everything as a miss.

## 4. Configuration

Read by `modelserve/config.py` (pydantic-settings, also loads `.env`). `.env.example` lists all of them;
`tests/test_env_drift.py` fails if `.env.example` has a key that `Settings` does not know (it does not
check the reverse direction).

| Env var | Default | Purpose |
|---|---|---|
| `ROUTER_HOST` | `0.0.0.0` | Bind host |
| `ROUTER_PORT` | `8100` | Bind port |
| `TEI_EMBED_URL` | `http://tei-embed:8101` | Embed upstream |
| `TEI_RERANK_URL` | `http://tei-rerank:8102` | Rerank upstream (not provisioned here) |
| `VLLM_URL` | `http://vllm:8103` | Generate upstream (not provisioned here) |
| `REDIS_URL` | `redis://redis:6379/0` | Cache |
| `EMBED_CACHE_ENABLED` | `true` | Disable to skip Redis entirely |
| `EMBED_CACHE_TTL_SECONDS` | `86400` (1 day) | Cache TTL |
| `MODEL_VERSION` | `v1` | Cache key namespace |
| `MAX_QUEUE_DEPTH` | `100` | Max concurrent in-flight requests |
| `UPSTREAM_TIMEOUT_SECONDS` | `30.0` | httpx client timeout |

Admission control is one **global** in-flight counter shared by all endpoints, guarded by an asyncio
lock. Rejected requests increment `modelserve_admission_rejections_total{endpoint}`.

Metrics: `modelserve_in_flight_requests{endpoint}`, `modelserve_admission_rejections_total{endpoint}`,
`modelserve_request_duration_seconds{endpoint,status_code}`, `modelserve_cache_hits_total`,
`modelserve_cache_misses_total`. No tracing (no OpenTelemetry code or dependency).

## 5. Run locally

Root stack: not available (see Status).

Standalone, with the repo's compose (starts a TEI CPU embed container with `BAAI/bge-small-en-v1.5`
and the router, both on the **external** network `platform-core_default`, which must already exist; the
router expects a `redis` host on it, i.e. the platform-core Redis):

```bash
docker compose -f docker-compose.local.yaml up --build
```

The compose sets `MODEL_VERSION=bge-small-en-v1.5` and does not set the rerank or vLLM URLs. Without
the external network, run the router directly:

```bash
make install
make run          # python -m modelserve, http://localhost:8100/docs
```

Direct runs need a reachable Redis and TEI (override `REDIS_URL`, `TEI_EMBED_URL`), or set
`EMBED_CACHE_ENABLED=false` to skip Redis.

Docker image: `Dockerfile` (python:3.12-slim, `EXPOSE 8100`, `HEALTHCHECK` on `/healthz`, runs
`python -m modelserve`).

## 6. Build, test and lint

| Command | What |
|---|---|
| `make install` | `pip install -r requirements-dev.txt` and `pip install -e .` |
| `make lint` | `ruff check .` (line length 110, rules E, F, I, UP, B) |
| `make format` | `ruff format .` |
| `make test` | `pytest -v tests/` |

CI (`.github/workflows/ci.yaml`, path-filtered to `platform-modelserve/**`, Python 3.12) runs
`make lint` then `make test`. `pyproject.toml` declares `requires-python >=3.10`. Dependencies are pip
based; there is no `uv.lock`.

Tests: `test_admission`, `test_cache`, `test_config`, `test_contract_conformance` (native and OpenAI
shapes), `test_env_drift`, `test_router` (mock upstreams).

## 7. Spec and verification

- `FEATURES.yaml` is in this repo: one entry per scenario of `add-platform-modelserve` (8). The static NetworkPolicy
  scenario is `automated`; the other seven are `planned` until run on a stack with the overlay
  `platform-e2e/compose/modelserve.override.yaml` (router + a deterministic TEI fake, Redis DB 4; how to bring it up:
  `platform-e2e/compose/README.md`). The e2e features are `platform-e2e/tests/e2e/features/modelserve/`.
  The unit, contract and router tests above remain the fast check.
- Gates: `make -C platform-e2e features-check` and `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec (`openspec/changes/<id>`), per the root README's ASDLC. The original
  change is `openspec/changes/add-platform-modelserve`. Architecture: ADR-0011.

## 8. Gotchas

- Cache and Redis are fail-open: errors are logged, not returned. The Redis client is created lazily,
  so a wrong `REDIS_URL` does not fail startup.
- Vector dimension is not validated. Changing the TEI model without bumping `MODEL_VERSION` serves
  stale vectors from the cache, and consumers' dims (`RAG_EMBED_DIM`, team-search `EMBEDDING_DIM`)
  must match the model.
- `/rerank` and `/generate` return 503 until you point `TEI_RERANK_URL` / `VLLM_URL` at real servers.
- `/healthz` is liveness only; it does not report upstream or Redis health.
- Request latency is only recorded for successful `/embed` calls.
- The `modelserve_in_flight_requests` gauge is set to the global in-flight count under whichever
  endpoint label last changed it, so per-endpoint values are not meaningful.

## 9. Known gaps

- No authn/authz on any endpoint; it relies on network isolation.
- Not in the root compose, and not wired into team-ai or team-search by default.
- `/rerank` and `/generate` have no runtime anywhere in the repo.
- No streaming for `/generate` or chat completions.
- The vLLM proxy is e2e-tested against the TEI fake's stand-in, not a real vLLM; the rerank and chat paths have no real runtime.
- `.env.example` drift gate only checks one direction.
- Stale docs elsewhere: `team-search/README.md` documents `MODELSERVE_URL` (code reads
  `MODEL_SERVER_URL`); root `AGENTS.md` lists TEI/vLLM ports `:8101`-`:8103` as if provisioned.

## 10. Links

- Root `AGENTS.md` (rules, port table), root `README.md` (ASDLC).
- `platform-core/docs/ADR/0011-model-serving.md`.
- `openspec/changes/add-platform-modelserve`.
