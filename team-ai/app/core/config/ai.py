"""AI capability settings: LLM router, Langfuse observability, RAG."""

from __future__ import annotations

from pydantic import BaseModel, Field


class AISettingsMixin(BaseModel):
    # LLM router
    CHAT_MODEL: str = ""
    CHAT_FALLBACK_MODELS: str = ""
    JUDGE_CHAT_MODEL: str = ""
    # Chat streaming backend for the gRPC ChatService (ML/LLM decoupling seam):
    # "mock" streams deterministic tokens offline; "llm_router" streams from the
    # external LLM via the existing router (team-ai embeds no model itself).
    CHAT_BACKEND: str = "mock"  # "mock" | "llm_router"

    # Chat path resilience (CHAT_BACKEND=llm_router). Each attempt is cancelled when
    # no chunk arrives within the first-token timeout; LLM_MAX_ATTEMPTS bounds all
    # pre-first-chunk attempts across the fallback chain. Each target has its own
    # breaker: LLM_BREAKER_THRESHOLD consecutive transient failures (429/5xx/timeout/
    # connection) open it, and after the cooldown one probe request is let through.
    LLM_FIRST_TOKEN_TIMEOUT_SECONDS: float = Field(default=8.0, gt=0)
    LLM_MAX_ATTEMPTS: int = Field(default=3, ge=1)
    LLM_BREAKER_THRESHOLD: int = Field(default=3, gt=0)
    LLM_BREAKER_COOLDOWN_SECONDS: float = Field(default=30.0, ge=0)

    # Prompt and session history. The system prompt comes from Langfuse when it is
    # enabled and reachable, else CHAT_SYSTEM_PROMPT, else a built-in static prompt.
    CHAT_SYSTEM_PROMPT: str = ""
    CHAT_HISTORY_MAX_TURNS: int = Field(default=10, ge=0)
    CHAT_HISTORY_MAX_TOKENS: int = Field(default=1500, gt=0)
    CHAT_HISTORY_TTL_SECONDS: int = Field(default=1800, gt=0)

    # What user text may appear in logs and traces: "redacted" (PII masked, default),
    # "off" (no user text) or "full" (raw; refused outside dev/local/test). Text sent
    # to the model is always redacted, whatever this says.
    LLM_TRACE_CONTENT: str = "redacted"

    # Chat quota: one unit per reply (resource ``chat.reply``), enforced when
    # QUOTA_ENABLED and CHAT_BACKEND=llm_router.
    QUOTA_CHAT_REPLIES_PER_WINDOW: int = Field(default=200, gt=0)
    QUOTA_CHAT_WINDOW_SECONDS: int = Field(default=86_400, gt=0)

    # Langfuse observability
    LANGFUSE_ENABLED: bool = False
    LANGFUSE_PUBLIC_KEY: str = ""
    LANGFUSE_SECRET_KEY: str = ""
    LANGFUSE_BASE_URL: str = "https://cloud.langfuse.com"
    LANGFUSE_PROMPT_CACHE_TTL_SECONDS: int = Field(default=60, ge=0)

    # RAG
    RAG_ENABLED: bool = False
    # Vector store backend: "memory" (in-process, dev) | "qdrant" (real infra).
    RAG_BACKEND: str = "memory"
    RAG_CHUNK_SIZE: int = Field(default=512, gt=0)
    RAG_CHUNK_OVERLAP: int = Field(default=50, ge=0)
    RAG_DEFAULT_TOP_K: int = Field(default=5, gt=0)
    RAG_EMBED_MODEL: str = ""
    RAG_MOCK_EMBED_DIM: int = Field(default=16, gt=0)
    RAG_RETRIEVE_TIMEOUT_SECONDS: float = Field(default=10.0, gt=0)
    # ShoppingAssistant: drop RAG hits scoring below this (0 = keep the nearest k; scores
    # are model specific, so tune per deployment).
    ASSISTANT_RAG_MIN_SCORE: float = Field(default=0.0, ge=0)

    # Embedding backend — the ML/LLM decoupling seam. team-ai runs NO embedding
    # model in-process; "model_server" delegates vectorization to a remote ML
    # serving layer over HTTP. "mock" keeps deterministic dev vectors offline.
    RAG_EMBED_BACKEND: str = "mock"  # "mock" | "model_server"
    RAG_EMBED_SERVER_URL: str = ""  # base URL of the embedding/ML server
    RAG_EMBED_SERVER_PATH: str = "/embed"  # POST {"texts": [...]} -> vectors
    RAG_EMBED_DIM: int = Field(default=384, gt=0)  # vector dim from the server
    RAG_EMBED_TIMEOUT_SECONDS: float = Field(default=10.0, gt=0)

    # Tag taxonomy registry persistence (tag-taxonomy-persistence): canonical tags and
    # exploring candidates in team-ai's Redis, own database (0 serving/chat, 2 featurestore,
    # 3-4 and 10-13 e2e are taken) and prefix. Needs REDIS_ENABLED.
    TAXONOMY_PERSISTENCE_ENABLED: bool = False
    TAXONOMY_REDIS_DATABASE: int = Field(default=5, ge=0)
    TAXONOMY_REDIS_PREFIX: str = "tagtax"

    # Qdrant vector store (used when RAG_BACKEND=qdrant).
    RAG_QDRANT_URL: str = "http://localhost:6333"
    RAG_QDRANT_COLLECTION: str = "rag_documents"
