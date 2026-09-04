"""Service configuration.

pydantic-settings, flat and per-capability (``settings.QDRANT_URL``, not
``settings.qdrant.url``) so call-sites stay short — mirrors the product repo
convention. ``extra="forbid"`` means an unknown key in ``.env`` is a hard error,
not a silent typo. Values are read from the environment / ``.env`` at process
start; use :func:`get_settings` (cached) everywhere rather than constructing
``Settings()`` yourself.
"""

from __future__ import annotations

from functools import lru_cache

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="forbid",
    )

    # ── Transport ────────────────────────────────────────────────────────────
    GRPC_PORT: int = 50052

    # ── Datastores (this service OWNS its own storage — Rule 3: never reach
    # into another service's DB; cross-service data comes via gRPC) ───────────
    # Optional until a Postgres-backed capability is wired up (alembic migrations).
    DATABASE_URL: str | None = None
    # Vector store for search/RAG (team-ai). Shared Qdrant collection.
    QDRANT_URL: str = "http://localhost:6333"
    # Optional cache / rate-limit backend.
    REDIS_URL: str | None = None

    # ── Auth (stub) ──────────────────────────────────────────────────────────
    # Static bearer token checked by the auth interceptor. TODO: replace with
    # JWT/cookie verification at the edge per ADR-0003; services then only ever
    # see a resolved Principal.
    AUTH_BEARER_TOKEN: str | None = None

    # ── Observability (OTel) ─────────────────────────────────────────────────
    OTEL_EXPORTER_OTLP_ENDPOINT: str = "http://localhost:4317"
    OTEL_SERVICE_NAME: str = "team-service"


@lru_cache
def get_settings() -> Settings:
    """Return the process-wide settings singleton (parsed once)."""
    return Settings()
