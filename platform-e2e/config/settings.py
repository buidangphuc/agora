"""Environment + runtime configuration.

Single source of truth for config (mirrors bds `environments.ts` but collapsed
into one module — no vestigial second config file). Loads the default `.env`
first, then the env-specific `env/.env.<ENV>` on top, then reads everything into
a typed `Settings` object.
"""

from __future__ import annotations

import os
from functools import lru_cache
from pathlib import Path

from dotenv import load_dotenv
from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict

ROOT_DIR = Path(__file__).resolve().parent.parent
ENV_DIR = ROOT_DIR / "env"


def load_environment() -> str:
    """Load dotenv files and return the active env name.

    Order (later wins): `.env` (defaults) -> `env/.env.<ENV>`.
    `ENV` may be provided by the shell or by the default `.env` file.
    """
    load_dotenv(ROOT_DIR / ".env", override=False)
    env = os.getenv("ENV", "local").strip()
    specific = ENV_DIR / f".env.{env}"
    if specific.exists():
        load_dotenv(specific, override=False)
    return env


class Settings(BaseSettings):
    """Typed runtime settings resolved from environment variables."""

    model_config = SettingsConfigDict(extra="ignore", case_sensitive=False)

    env: str = Field(default="local")
    base_url: str = Field(default="http://localhost:3000")
    gateway_url: str = Field(default="http://localhost:8080")
    headless: bool = Field(default=True)
    debug: bool = Field(default=False)
    device: str | None = Field(default=None)  # e.g. "iPhone 12", "Pixel 7"
    seed_password: str = Field(default="pass123")

    # Analytics tracking (emit-tracking-events): where the gateway produces
    # TrackingEvent envelopes so the e2e can consume + assert them.
    kafka_brokers: str = Field(default="localhost:9092")
    kafka_analytics_topic: str = Field(default="analytics.events")
    # Feature flags (wire-openfeature): Flipt REST/HTTP endpoint used to toggle
    # `checkout-enabled` per scenario (the JS provider uses :8080, not gRPC :9000).
    flipt_url: str = Field(default="http://localhost:8080")

    # Timeouts (ms) — keep close to Playwright defaults; do not hardcode long sleeps.
    action_timeout_ms: int = Field(default=10_000)
    navigation_timeout_ms: int = Field(default=30_000)
    expect_timeout_ms: int = Field(default=10_000)


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    """Load env files once and build the cached Settings singleton."""
    load_environment()
    return Settings(
        env=os.getenv("ENV", "local"),
        base_url=os.getenv("BASE_URL", "http://localhost:3000"),
        gateway_url=os.getenv("GATEWAY_URL", "http://localhost:8080"),
        headless=_as_bool(os.getenv("HEADLESS"), default=True),
        debug=_as_bool(os.getenv("DEBUG"), default=False),
        device=os.getenv("DEVICE") or None,
        seed_password=os.getenv("SEED_PASSWORD", "pass123"),
        kafka_brokers=os.getenv("KAFKA_BROKERS", "localhost:9092"),
        kafka_analytics_topic=os.getenv("KAFKA_ANALYTICS_TOPIC", "analytics.events"),
        flipt_url=os.getenv("FLIPT_URL", "http://localhost:8080"),
    )


def _as_bool(value: str | None, *, default: bool) -> bool:
    if value is None:
        return default
    return value.strip().lower() in {"1", "true", "yes", "on"}
