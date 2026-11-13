from __future__ import annotations

import datetime as dt
import json
import sys

import redis

from forecast.config import Settings
from forecast.extract import ConfigError
from forecast.pipeline import run_forecast_job

EXIT_OK = 0
EXIT_CONFIG = 2
EXIT_REDIS = 3


def _parse_as_of(value: str) -> dt.datetime:
    if not value:
        return dt.datetime.now(dt.timezone.utc)
    try:
        parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as exc:
        raise ConfigError(f"invalid AS_OF: {value}") from exc
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=dt.timezone.utc)
    return parsed


def main(argv: list[str] | None = None) -> int:
    args = sys.argv[1:] if argv is None else argv
    if args[:1] != ["run"]:
        print(json.dumps({"status": "error", "error": "usage: forecast run"}), file=sys.stderr)
        return EXIT_CONFIG
    try:
        settings = Settings()
        as_of = _parse_as_of(settings.as_of)
        client = redis.Redis.from_url(settings.redis_url, socket_timeout=10, protocol=2)
        summary = run_forecast_job(settings, as_of, client)
    except ConfigError as exc:
        print(json.dumps({"status": "config_error", "error": str(exc)}), file=sys.stderr)
        return EXIT_CONFIG
    except (redis.RedisError, OSError) as exc:
        print(json.dumps({"status": "redis_error", "error": type(exc).__name__}), file=sys.stderr)
        return EXIT_REDIS
    print(json.dumps(summary))
    return EXIT_OK
