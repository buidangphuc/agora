"""AI capability settings: LLM router, Langfuse observability, RAG."""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, Field, model_validator

ChatBackend = Literal["mock", "llm_router"]


class AISettingsMixin(BaseModel):
    # Declared by the infra mixin; annotated here so the guard below can read it.
    REDIS_DATABASE: int

    # LLM router
    CHAT_MODEL: str = ""
    CHAT_FALLBACK_MODELS: str = ""
    JUDGE_CHAT_MODEL: str = ""
    # Every provider call is bounded: output cap, per-request timeout and the
    # SDK's own hidden retry budget. LLM_MAX_RETRIES is kept at 1 because the
    # breaker only sees a call after the SDK has exhausted its retries, so each
    # extra retry multiplies the latency before failover can engage.
    LLM_MAX_TOKENS: int = Field(default=2048, gt=0, le=200_000)
    LLM_TIMEOUT_SECONDS: float = Field(default=60.0, gt=0, le=600)
    LLM_MAX_RETRIES: int = Field(default=1, ge=0, le=5)
    # Overall budget for one ``LLMInstance.ainvoke`` across the whole fallback
    # chain; without it a 3-model chain can cost 3x LLM_TIMEOUT_SECONDS.
    LLM_CHAIN_DEADLINE_SECONDS: float = Field(default=90.0, gt=0, le=1800)
    # Primary-model circuit breaker: consecutive transient failures (429, 5xx,
    # timeouts, connection errors) before failover, and the cooldown before the
    # primary is probed again.
    LLM_BREAKER_FAILURE_THRESHOLD: int = Field(default=3, gt=0)
    LLM_BREAKER_COOLDOWN_SECONDS: float = Field(default=30.0, gt=0)

    @model_validator(mode="after")
    def _chain_deadline_covers_one_call(self) -> AISettingsMixin:
        if self.LLM_CHAIN_DEADLINE_SECONDS < self.LLM_TIMEOUT_SECONDS:
            raise ValueError(
                "LLM_CHAIN_DEADLINE_SECONDS must be >= LLM_TIMEOUT_SECONDS"
            )
        return self

    # Chat (gRPC StreamChat): ``mock`` echoes offline, ``llm_router`` streams
    # from the LLM chain. History is bounded per (principal, session); quota
    # counts one reply per call.
    CHAT_BACKEND: ChatBackend = "mock"
    CHAT_FIRST_TOKEN_TIMEOUT_SECONDS: float = Field(default=8.0, gt=0)
    CHAT_SYSTEM_PROMPT: str = ""
    CHAT_HISTORY_MAX_TURNS: int = Field(default=10, ge=0)
    CHAT_HISTORY_MAX_TOKENS: int = Field(default=1500, gt=0)
    CHAT_HISTORY_TTL_SECONDS: int = Field(default=1800, gt=0)
    QUOTA_CHAT_REPLIES_PER_WINDOW: int = Field(default=200, gt=0)
    QUOTA_CHAT_WINDOW_SECONDS: int = Field(default=86400, gt=0)

    # Langfuse observability: on exactly when both keys are set.
    LANGFUSE_PUBLIC_KEY: str = ""
    LANGFUSE_SECRET_KEY: str = ""
    LANGFUSE_BASE_URL: str = "https://cloud.langfuse.com"
    LANGFUSE_PROMPT_CACHE_TTL_SECONDS: int = Field(default=60, ge=0)

    @property
    def langfuse_enabled(self) -> bool:
        return bool(self.LANGFUSE_PUBLIC_KEY and self.LANGFUSE_SECRET_KEY)

    # RAG: on, it uses the Qdrant store and the model_server embeddings, and the
    # listing indexer feeds it from Kafka.
    RAG_ENABLED: bool = False
    RAG_CHUNK_SIZE: int = Field(default=512, gt=0)
    RAG_CHUNK_OVERLAP: int = Field(default=50, ge=0)
    RAG_DEFAULT_TOP_K: int = Field(default=5, gt=0)
    RAG_QDRANT_URL: str = "http://localhost:6333"
    RAG_QDRANT_COLLECTION: str = Field(default="rag_documents", min_length=1)
    RAG_EMBED_SERVER_URL: str = ""
    RAG_EMBED_SERVER_PATH: str = "/embed"
    RAG_EMBED_DIM: int = Field(default=384, gt=0)
    RAG_EMBED_TIMEOUT_SECONDS: float = Field(default=10.0, gt=0)
    RAG_RETRIEVE_TIMEOUT_SECONDS: float = Field(default=10.0, gt=0)
    ASSISTANT_RAG_MIN_SCORE: float = Field(default=0.0, ge=0)

    @model_validator(mode="after")
    def _rag_needs_its_backends(self) -> AISettingsMixin:
        if self.RAG_ENABLED and not self.RAG_QDRANT_URL:
            raise ValueError("RAG_ENABLED requires RAG_QDRANT_URL")
        if self.RAG_ENABLED and not self.RAG_EMBED_SERVER_URL:
            raise ValueError("RAG_ENABLED requires RAG_EMBED_SERVER_URL")
        return self

    # Curated tags live in Redis on their own database, never the shared
    # REDIS_DATABASE.
    TAXONOMY_REDIS_DATABASE: int = Field(default=5, ge=0)
    TAXONOMY_REDIS_PREFIX: str = Field(default="tagtax", min_length=1)

    @model_validator(mode="after")
    def _taxonomy_persistence_needs_its_own_redis_database(self) -> AISettingsMixin:
        if self.TAXONOMY_REDIS_DATABASE == self.REDIS_DATABASE:
            raise ValueError("TAXONOMY_REDIS_DATABASE must differ from REDIS_DATABASE")
        return self
