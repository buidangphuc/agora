"""Infrastructure connection settings: Redis."""

from __future__ import annotations

from pydantic import BaseModel, Field


class InfraSettingsMixin(BaseModel):
    REDIS_HOST: str
    REDIS_PORT: int = Field(default=6379, gt=0, lt=65_536)
    REDIS_PASSWORD: str = ""
    REDIS_DATABASE: int = Field(default=0, ge=0)
    REDIS_TIMEOUT_SECONDS: int = Field(default=5, gt=0)
