"""Configuration settings for modelserve router."""

from __future__ import annotations

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )

    # Router Server
    router_host: str = Field(default="0.0.0.0", alias="ROUTER_HOST")
    router_port: int = Field(default=8100, alias="ROUTER_PORT")

    # Upstream Inference Runtimes
    tei_embed_url: str = Field(default="http://tei-embed:8101", alias="TEI_EMBED_URL")
    tei_rerank_url: str = Field(default="http://tei-rerank:8102", alias="TEI_RERANK_URL")
    vllm_url: str = Field(default="http://vllm:8103", alias="VLLM_URL")

    # Embedding Cache
    redis_url: str = Field(default="redis://redis:6379/0", alias="REDIS_URL")
    embed_cache_enabled: bool = Field(default=True, alias="EMBED_CACHE_ENABLED")
    embed_cache_ttl_seconds: int = Field(default=86400, alias="EMBED_CACHE_TTL_SECONDS")
    model_version: str = Field(default="v1", alias="MODEL_VERSION")

    # Admission Control & Resilience
    max_queue_depth: int = Field(default=100, alias="MAX_QUEUE_DEPTH")
    upstream_timeout_seconds: float = Field(default=30.0, alias="UPSTREAM_TIMEOUT_SECONDS")


settings = Settings()
