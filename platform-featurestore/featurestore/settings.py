"""Environment-driven settings (names fixed by the featurestore-materialization proposal)."""

from __future__ import annotations

import os
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path


class ConfigError(Exception):
    """Invalid configuration or input (exit code 2)."""


def parse_as_of(raw: str | None, now: datetime | None = None) -> datetime:
    """AS_OF as a naive UTC datetime. Empty or whitespace means now (compose passes AS_OF="" when unset)."""
    if raw is None or not raw.strip():
        n = now or datetime.now(timezone.utc)
        return n.astimezone(timezone.utc).replace(tzinfo=None, microsecond=0)
    text = raw.strip()
    if text.endswith(("Z", "z")):
        text = text[:-1] + "+00:00"
    try:
        dt = datetime.fromisoformat(text)
    except ValueError as exc:
        raise ConfigError(f"AS_OF must be RFC 3339, got {raw!r}") from exc
    if dt.tzinfo is None:
        raise ConfigError(f"AS_OF must carry a UTC offset (RFC 3339), got {raw!r}")
    return dt.astimezone(timezone.utc).replace(tzinfo=None)


@dataclass(frozen=True)
class Settings:
    input_dir: Path
    offline_dir: Path
    redis_url: str
    ttl_seconds: int
    parity_sample: int
    as_of: datetime
    registry_dir: Path
    dataset_window_days: int = 30

    @classmethod
    def from_env(cls, env: dict[str, str] | None = None) -> Settings:
        e = os.environ if env is None else env
        try:
            ttl = int(e.get("FEATURESTORE_ONLINE_TTL_SECONDS") or 172800)
            sample = int(e.get("FEATURESTORE_PARITY_SAMPLE") or 200)
            window = int(e.get("DATASET_WINDOW_DAYS") or 30)
        except ValueError as exc:
            raise ConfigError(str(exc)) from exc
        if window <= 0:
            raise ConfigError(f"DATASET_WINDOW_DAYS must be positive, got {window}")
        return cls(
            input_dir=Path(e.get("FEATURESTORE_INPUT_DIR") or "/data"),
            offline_dir=Path(e.get("FEATURESTORE_OFFLINE_DIR") or "/features"),
            redis_url=e.get("FEATURESTORE_REDIS_URL") or "",
            ttl_seconds=ttl,
            parity_sample=sample,
            as_of=parse_as_of(e.get("AS_OF")),
            registry_dir=Path(__file__).resolve().parent.parent / "registry",
            dataset_window_days=window,
        )
