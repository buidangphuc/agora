"""Composed application settings.

``Settings`` is intentionally flat (``settings.POSTGRES_HOST``, not
``settings.infra.postgres.host``) so call-sites stay short. The mixins below
just group the field declarations across files for readability; pydantic
flattens them back into one schema.

To add a new section: create a new ``BaseModel`` mixin and add it to the
``Settings`` inheritance list. To add a new adapter / backend for an existing
section: just register it in that module's ``factory.py`` — the config layer
does not maintain an allowlist (factory raises ``ValueError`` on unknown).

Numeric range validation is expressed via ``Field(gt=, ge=, le=)`` so the
mixins don't need explicit ``@field_validator`` for the common cases. Add a
validator only for cross-field or semantic checks that ``Field`` can't express.
"""

from __future__ import annotations

from functools import lru_cache

from pydantic import model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

from app.core.config.ai import AISettingsMixin
from app.core.config.http import HttpSettingsMixin
from app.core.config.infra import InfraSettingsMixin
from app.core.config.messaging import MessagingSettingsMixin
from app.core.config.mongo import MongoSettingsMixin
from app.core.config.platform import PlatformSettingsMixin
from app.core.config.recommendations import RecommendationSettingsMixin
from app.core.config.runtime import (
    WEAK_AUTH_TOKENS,
    Environment,
    RuntimeSettingsMixin,
)
from app.core.config.transport import TransportSettingsMixin

__all__ = [
    "WEAK_AUTH_TOKENS",
    "Environment",
    "Settings",
    "get_settings",
]

_SECRET_MARKERS = ("PASSWORD", "SECRET", "TOKEN", "KEY", "PEPPER")


class Settings(
    BaseSettings,
    RuntimeSettingsMixin,
    HttpSettingsMixin,
    InfraSettingsMixin,
    MongoSettingsMixin,
    AISettingsMixin,
    MessagingSettingsMixin,
    PlatformSettingsMixin,
    RecommendationSettingsMixin,
    TransportSettingsMixin,
):
    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="forbid",
    )

    @model_validator(mode="after")
    def validate_runtime_safety(self) -> Settings:
        if not self.ENVIRONMENT.is_local and not self.AUTH_BEARER_TOKEN:
            raise ValueError("AUTH_BEARER_TOKEN is required outside dev/local/test")
        if self.AUTH_ADMIN_BEARER_TOKEN and not self.ENVIRONMENT.is_local:
            if (
                self.AUTH_ADMIN_BEARER_TOKEN in WEAK_AUTH_TOKENS
                or len(self.AUTH_ADMIN_BEARER_TOKEN) < 24
            ):
                raise ValueError(
                    "AUTH_ADMIN_BEARER_TOKEN is too weak outside dev/local/test"
                )
            if self.AUTH_ADMIN_BEARER_TOKEN == self.AUTH_BEARER_TOKEN:
                raise ValueError(
                    "AUTH_ADMIN_BEARER_TOKEN must differ from AUTH_BEARER_TOKEN"
                )
        if self.TAXONOMY_PERSISTENCE_ENABLED:
            if not self.REDIS_ENABLED:
                raise ValueError(
                    "TAXONOMY_PERSISTENCE_ENABLED requires REDIS_ENABLED=true"
                )
            if self.TAXONOMY_REDIS_DATABASE == self.REDIS_DATABASE:
                raise ValueError(
                    "TAXONOMY_REDIS_DATABASE must differ from REDIS_DATABASE"
                )
        if self.GRPC_BEARER_FALLBACK_ENABLED and not self.ENVIRONMENT.is_local:
            raise ValueError(
                "GRPC_BEARER_FALLBACK_ENABLED must be false outside dev/local/test"
            )
        if (
            self.GRPC_RATE_LIMIT_ENABLED
            and self.RATE_LIMIT_BACKEND == "redis"
            and not self.REDIS_ENABLED
        ):
            raise ValueError("RATE_LIMIT_BACKEND=redis requires REDIS_ENABLED=true")
        if (
            self.GRPC_RATE_LIMIT_ENABLED
            and self.RATE_LIMIT_BACKEND == "memory"
            and not self.ENVIRONMENT.is_local
        ):
            # A per-process limiter multiplies the limit by the replica count.
            raise ValueError(
                "RATE_LIMIT_BACKEND=memory is refused with GRPC_RATE_LIMIT_ENABLED "
                "outside dev/local/test (use RATE_LIMIT_BACKEND=redis)"
            )
        if (
            self.CHAT_BACKEND == "llm_router"
            and not self.ENVIRONMENT.is_local
            and not (self.GRPC_RATE_LIMIT_ENABLED and self.QUOTA_ENABLED)
        ):
            # The real-LLM path costs money per call and anonymous chat is allowed
            # (ai:use is not enforced), so it must be rate limited and metered.
            raise ValueError(
                "CHAT_BACKEND=llm_router outside dev/local/test requires "
                "GRPC_RATE_LIMIT_ENABLED=true and QUOTA_ENABLED=true"
            )
        if (
            self.RECS_ENABLED
            and self.RECS_BACKEND == "memory"
            and not self.ENVIRONMENT.is_local
        ):
            # The memory backend serves a fake in-process catalogue.
            raise ValueError(
                "RECS_BACKEND=memory is refused with RECS_ENABLED outside "
                "dev/local/test (use RECS_BACKEND=qdrant)"
            )
        if self.LLM_TRACE_CONTENT not in {"off", "redacted", "full"}:
            raise ValueError("LLM_TRACE_CONTENT must be one of off, redacted, full")
        if self.LLM_TRACE_CONTENT == "full" and not self.ENVIRONMENT.is_local:
            raise ValueError(
                "LLM_TRACE_CONTENT=full is refused outside dev/local/test "
                "(raw user text must not reach logs or traces)"
            )
        if not self.ENVIRONMENT.is_production:
            return self

        if self.DOCS_ENABLED:
            raise ValueError("DOCS_ENABLED must be false in production")
        for field_name, parsed in (
            ("CORS_ALLOW_ORIGINS", self.cors_allow_origins),
            ("TRUSTED_HOSTS", self.trusted_hosts),
        ):
            if "*" in parsed:
                raise ValueError(f"{field_name} must not contain '*' in production")
        if (
            self.AUTH_BEARER_TOKEN in WEAK_AUTH_TOKENS
            or len(self.AUTH_BEARER_TOKEN) < 24
        ):
            raise ValueError("AUTH_BEARER_TOKEN is too weak for production")
        return self

    def redacted_summary(self) -> dict[str, object]:
        values = self.model_dump(mode="json")
        return {key: self._redacted_value(key, value) for key, value in values.items()}

    def _redacted_value(self, key: str, value: object) -> object:
        if not value:
            return value
        key_upper = key.upper()
        if any(marker in key_upper for marker in _SECRET_MARKERS):
            return "***"
        if (
            key_upper.endswith("_URL")
            and isinstance(value, str)
            and "://" in value
            and "@" in value
        ):
            return "***"
        return value


@lru_cache
def get_settings() -> Settings:
    return Settings()  # pyright: ignore[reportCallIssue]
