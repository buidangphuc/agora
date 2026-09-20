# platform-modelserve

Internal Model Serving Router and vendor runtime integration capability (ADR-0011).

## Architecture

Following ADR-0011, `platform-modelserve` is an internal platform capability:
- **No business tables & no gRPC proto**: It serves purely HTTP/JSON internally to `team-ai` on `:8100`.
- **Vendor Runtimes**: Delegates GPU/batch inference to Hugging Face TEI (`:8101` embed, `:8102` rerank) and vLLM (`:8103` generation).
- **Owned Router**: Implements contract adaptation, Redis embedding cache keyed by `hash(model_version + text)`, queue-depth based admission control with 429 backpressure, and Prometheus metrics.

## Ports

- Router HTTP: `:8100`
- TEI Embed: `:8101`
- TEI Rerank: `:8102`
- vLLM: `:8103`

## Endpoints

- `POST /embed` / `POST /v1/embeddings`: Vector embeddings conforming to `team-ai`'s contract.
- `POST /rerank`: Cross-encoder ranking proxying to TEI reranker.
- `POST /v1/chat/completions` / `POST /generate`: Text generation proxying to vLLM.
- `GET /healthz`: Liveness probe.
- `GET /metrics`: Prometheus metrics (request counts, latency, cache hits/misses, in-flight queue depth).

## Running locally

```bash
# 1. Bring up platform-core shared infra (Redis)
cd ../platform-core/infra && docker compose up -d redis

# 2. Run router locally
cd platform-modelserve
make install
make test
make run
```
