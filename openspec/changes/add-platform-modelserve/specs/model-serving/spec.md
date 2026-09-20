## ADDED Requirements

### Requirement: Model server implements team-ai embedding contract

The system SHALL provide an HTTP router endpoint on `:8100` that accepts `POST` requests with body `{"texts": ["text1", "text2"]}` (or OpenAI-compatible payload `{"input": ["..."]}`) and returns vectors formatted as `{"embeddings": [[...], [...]]}` (or OpenAI-shaped `{"data": [{"embedding": [...]}]}`). The output SHALL conform strictly to `team-ai`'s `_extract_vectors` validation.

#### Scenario: Router serves embeddings conforming to team-ai parser

- **WHEN** `team-ai` sends `POST /embed` with `{"texts": ["apple", "banana"]}`
- **THEN** the router returns HTTP 200 with `{"embeddings": [[...], [...]]}` where each embedding has the expected dimension (e.g. 384 or 768) and passes `_extract_vectors` validation without error

#### Scenario: Router accepts OpenAI format embeddings request

- **WHEN** a client sends `POST /v1/embeddings` with `{"input": ["hello world"]}`
- **THEN** the router returns HTTP 200 with valid embedding vectors

### Requirement: Redis vector caching by model version and input hash

The model server router SHALL check Redis for cached vectors keyed by `hash(model_version + text)` before forwarding embedding requests to upstream inference runtimes. Cached vectors SHALL be returned immediately without calling the backend. Cache misses SHALL be batched, forwarded to the upstream TEI embed runtime, and stored in Redis with a configurable TTL.

#### Scenario: Embedding cache hit skips upstream inference

- **WHEN** an embedding request is made for a text whose vector is already cached in Redis under the active `model_version`
- **THEN** the router returns the cached vector immediately without contacting the TEI backend

#### Scenario: Multi-text embedding splits cache hits and misses

- **WHEN** an embedding request contains a mix of cached and uncached texts
- **THEN** the router fetches cached vectors from Redis, forwards only the uncached subset to TEI, populates Redis with newly computed vectors, and returns all vectors in the original request order

### Requirement: Upstream proxying for reranking and text generation

The model server router SHALL proxy rerank requests (`POST /rerank`) to Hugging Face TEI reranker (`:8102`) and text generation / chat completions (`POST /v1/chat/completions` or `POST /generate`) to vLLM (`:8103`).

#### Scenario: Router proxies rerank requests

- **WHEN** a client posts `{"query": "shoes", "texts": ["red sneakers", "blue jacket"]}` to `POST /rerank`
- **THEN** the router forwards the payload to the TEI rerank upstream and returns the ranked results

#### Scenario: Router proxies chat completions to vLLM

- **WHEN** a client posts an OpenAI-compatible chat completion payload to `POST /v1/chat/completions`
- **THEN** the router proxies the request to vLLM and returns the completion response

### Requirement: Admission control and queue depth backpressure

The model server router SHALL track pending in-flight requests and enforce a maximum queue depth threshold (`MAX_QUEUE_DEPTH`). When in-flight requests exceed this limit, the router SHALL return `429 Too Many Requests` with a `Retry-After` header.

#### Scenario: Backpressure on queue depth saturation

- **WHEN** the number of concurrent in-flight requests exceeds `MAX_QUEUE_DEPTH`
- **THEN** incoming requests receive HTTP status 429 with a `Retry-After` header to shed load and trigger downstream failover / retry

### Requirement: GitOps deployment and network isolation

The platform model serving capability SHALL be deployable via GitOps manifests in `platform-gitops` with separate Deployments per role (router, embed, rerank, vLLM), explicit resource limits / node affinities, and a Kubernetes NetworkPolicy ensuring only `team-ai` (and authorized platform services) can access the router port `:8100`.

#### Scenario: NetworkPolicy restricts access to team-ai

- **WHEN** network traffic targets port `:8100` of `platform-modelserve` in Kubernetes
- **THEN** only pods labeled `app: team-ai` (or internal prometheus scrapers) are permitted ingress
