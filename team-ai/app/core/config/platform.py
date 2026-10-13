"""Platform-layer settings: auth, rate limit, quota."""

from __future__ import annotations

from dataclasses import dataclass
from functools import lru_cache
from typing import Self

from pydantic import BaseModel, Field, model_validator


@dataclass(frozen=True)
class ApiKey:
    """One configured API key: who it authenticates as and the SHA-256 of the token."""

    subject: str
    scopes: tuple[str, ...]
    digest: bytes


@lru_cache(maxsize=8)
def parse_api_keys(raw: str) -> tuple[ApiKey, ...]:
    """Parse ``AUTH_API_KEYS``: comma-separated ``subject:scope|scope:sha256hex``.

    The subject is everything before the first ``:``, the digest everything after
    the last one, so scopes may themselves contain ``:`` (``tasks:read``).
    Cached by the raw string, so a ``model_copy`` with new keys never goes stale.
    """
    keys: list[ApiKey] = []
    for entry in (part.strip() for part in raw.split(",")):
        if not entry:
            continue
        fields = entry.split(":")
        subject, scopes_part, digest_hex = fields[0], ":".join(fields[1:-1]), fields[-1]
        try:
            digest = bytes.fromhex(digest_hex)
        except ValueError:
            digest = b""
        if len(fields) < 3 or not subject.strip() or len(digest) != 32:
            raise ValueError(
                "AUTH_API_KEYS entries must look like "
                "'subject:scope|scope:sha256hex' (64 hex chars; scopes may be empty)"
            )
        scopes = tuple(s.strip() for s in scopes_part.split("|") if s.strip())
        keys.append(ApiKey(subject=subject.strip(), scopes=scopes, digest=digest))
    return tuple(keys)


class PlatformSettingsMixin(BaseModel):
    # Auth / identity
    # Comma-separated ``subject:scope|scope:sha256hex`` entries (see parse_api_keys).
    # Several entries may share a subject (overlap rotation). Hash tokens with
    # ``python -m scripts.hash_api_key``.
    AUTH_API_KEYS: str = ""

    # Per-principal rate limit on HTTP and gRPC, always on, counted in Redis.
    RATE_LIMIT_WINDOW_SECONDS: int = Field(default=60, gt=0)
    RATE_LIMIT_REDIS_PREFIX: str = "rate-limit"
    RATE_LIMIT_PRINCIPAL_PER_MINUTE: int = Field(default=60, gt=0)

    # Chat reply quota, always on, counted in Redis under this key prefix.
    QUOTA_REDIS_PREFIX: str = Field(default="quota", min_length=1)

    @model_validator(mode="after")
    def _validate_api_keys(self) -> Self:
        parse_api_keys(self.AUTH_API_KEYS)  # a bad entry fails at boot
        return self

    @property
    def auth_api_keys(self) -> tuple[ApiKey, ...]:
        return parse_api_keys(self.AUTH_API_KEYS)
