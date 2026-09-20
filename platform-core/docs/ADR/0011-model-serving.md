# ADR-0011 — Model serving (platform-modelserve)

**Status:** Accepted · **Date:** 2026-09-19 · **Relates to:** ADR-0004, ADR-0010

## Context

`team-ai` already defines the ML decoupling seam: `RAG_EMBED_BACKEND=model_server` delegates vectorization over HTTP (`POST {path} {"texts":[…]}` → `{"embeddings":[[…]]}`, OpenAI-shaped replies also accepted) and `app/modules/ai/rag/embeddings.py` validates that contract in pure, offline-tested code. No server implements it. The default backend is mock, so RAG runs on deterministic fake vectors.

Three consequences follow. (1) Retrieval is single-stage — top-k vector only, no reranker — so `RAG_DEFAULT_TOP_K` is one knob carrying two opposed goals (recall vs prompt cost/noise). (2) `ModelRouter` fails over only reactively, after its 4xx circuit breaker trips; there is no capacity-driven routing. (3) We have no home for GPU inference workloads at all.

Inference has a resource profile unlike every other service here: GPU/VRAM-bound rather than I/O-bound, throughput-bound rather than latency-bound, multi-GB images, 30s–5min cold starts, and a deploy cadence tied to model versions rather than sprints. Five differing properties across one boundary is the signal to split.

## Decision

A new `platform-modelserve` repo. By the §6 decision rule it is not a `team-*` service: it owns no business tables, defines no proto, and is not reachable from the browser. It is a platform capability, following the `platform-recsys` precedent — internal, called only by `team-ai`, never routed through `team-gateway`.

- **Adopt vendor runtimes; write no inference code.** Hugging Face text-embeddings-inference (TEI) serves embed and rerank; vLLM serves generation. Both implement continuous/dynamic batching, which is the dominant throughput lever — a hand-written one-request-one-forward-pass server forfeits most of the hardware. What we own is the router: contract, cache, admission control, routing, metrics.
- **The contract is the one `team-ai` already codes against.** The server adapts to the existing consumer, not the reverse. `_extract_vectors` is its conformance test.
- **One Deployment per model role.** Embed (small, high-QPS), rerank (medium), and generate (7B+, GPU-only) have different SLOs, replica counts, and node classes. Merging them couples a cheap model's scaling cycle to an expensive model's cold start.
- **Autoscale on queue depth, not CPU.** KEDA reads a Prometheus `pending_requests` / replica metric. CPU utilization is near-meaningless for GPU serving; only queue depth reflects waiting demand.
- **Never scale to zero on a serving path.** `minReplicas` ≥ 1; model weights on a PVC warmed by an initContainer, not pulled from Hugging Face per pod start. Scale up early, scale down late — the asymmetry is deliberate and bounds cold-start exposure.
- **Backpressure before collapse.** Past a queue threshold the router returns 429 + Retry-After rather than accumulating doomed requests. That same signal drives capacity overflow to a third-party API through `team-ai`'s existing `ModelRouter` — which extends it from failure-handling to capacity-handling.
- **Redis embedding cache, keyed hash(model_version + text).** Embedding is a pure function of its inputs, so it is fully cacheable; catalog re-indexing is highly repetitive. The cheapest scale lever, applied before adding replicas.
- **GPU scheduling is explicit.** `nvidia.com/gpu` limits, node selector + toleration for the GPU pool; a Helm sub-chart per role in `platform-gitops`.
- **Ports (free against §5 of AGENTS.md):** router `:8100`, TEI embed `:8101`, TEI rerank `:8102`, vLLM `:8103`.
- **Delivered in four independently shippable stages:** 0 compose + TEI CPU + router + Redis, flipping `team-ai` off mock; 1 per-role Deployments + HPA; 2 KEDA queue-depth scaling, PVC cache, admission control; 3 overflow routing + metrics federation.

## Alternatives rejected

- **Hand-written FastAPI + transformers server.** Trivial to start, then 10–50× worse throughput for lack of batching. The complexity we would own is exactly the complexity TEI/vLLM already solved.
- **`team-ai` calls TEI directly, no router.** Fewer hops, and defensible while the only need is embedding. Rejected because the shared cache, model-version indirection, admission control, and a single metrics surface each need a common point — four reasons, where one or two would not have justified the hop.
- **One Deployment serving all models.** Simpler manifests; couples unrelated scaling and cold-start behaviour, and wastes GPU on models that do not need it.
- **HPA on CPU%, and scale-to-zero.** Both are the defaults, and both are wrong here. Recorded explicitly because they are the likely accidental choice.
- **Triton Inference Server.** More capable and more general; heavier operationally than TEI+vLLM for three known model roles.
- **A separate cluster per component** (the pattern in the reference system that prompted this ADR). Three control planes, node pools, and bills to isolate workloads that namespaces plus node selectors already separate. Over-partitioned for our scale.

## Consequences

- One more repo and one more network hop on every RAG request; the hop is justified only while ≥2 of the router's four reasons hold — revisit if they stop holding.
- A GPU node pool is a standing cost, and `minReplicas` ≥ 1 means it is never free. Cold start is bounded, not eliminated.
- `team-ai` gains a rerank stage, which changes `KnowledgeRetrievalService.retrieve`: retrieve wide, rerank down. `RAG_DEFAULT_TOP_K` stops being overloaded.
- Cache entries are invalidated by model-version bump, so `model_version` must be part of the key and surfaced in responses — the `platform-recsys` `model_version.py` convention carries over.
- The mock backend stays: offline tests and CI must not require a model server.
- Serving ports are internal. ADR-0010's NetworkPolicy must be extended so only `team-ai` may reach `:8100`; the runtimes accept no auth of their own.
- No proto, no gateway forwarder, no change to `platform-core/packages/proto`.
