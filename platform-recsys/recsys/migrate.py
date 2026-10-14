"""``python -m recsys migrate-registry``: seed the model registry once from the old Redis champions.

The old registry kept one champion per model in Redis (a version key and a JSON meta per version). Each
becomes version 1 of the matching registered model through ``modelregistry.seed``: its metrics as stored,
``eval_protocol`` copied to ``lineage.eval_protocol``, no artifact. A model that already has a version is
left alone, and the Redis keys and the serving pointer are never touched.
"""

from __future__ import annotations

import json
import logging

import modelregistry
from modelregistry import GateError, RegistryUnavailable

from . import registry_client
from .load import redis_cache

log = logging.getLogger("recsys.migrate")


def _champions(settings) -> tuple[tuple[str, str], ...]:
    """(registered model, Redis key holding its champion version); two-tower never had a Redis champion."""
    base = f"{settings.cache_prefix}:model"
    return (
        (registry_client.ALS_MODEL, f"{base}:champion"),
        (registry_client.GBDT_MODEL, f"{base}:champion:gbdt"),
    )


def _text(value) -> str | None:
    if value is None:
        return None
    return value.decode("utf-8") if isinstance(value, bytes) else str(value)


def _numeric(value) -> bool:
    return isinstance(value, (int, float)) and not isinstance(value, bool)


def _safe_metrics(stored) -> dict[str, float]:
    """Numeric metrics whose name passes the registry_client allow-list; the rest are dropped."""
    out: dict[str, float] = {}
    for k, v in stored.items():
        if not _numeric(v):
            continue
        try:
            registry_client._check_metrics({k: v})
        except registry_client.PayloadRejected:
            log.warning("dropping a non-allow-listed metric name from the Redis meta")
            continue
        out[k] = float(v)
    return out


def _safe_protocol(protocol) -> bool:
    try:
        registry_client._check_reason(protocol)
    except registry_client.PayloadRejected:
        log.warning("dropping an invalid eval_protocol from the Redis meta")
        return False
    return True


def migrate(settings, redis_client=None) -> dict[str, str]:
    """Seed every model that has a Redis champion. Returns ``{model: "seeded" | "skipped: <why>"}``.

    Raises RegistryUnavailable (nothing seeded when it is down at the start)."""
    registry_client.preflight()
    if redis_client is None:
        redis_client = redis_cache.connect(settings)
    outcome: dict[str, str] = {}
    for model, champion_key in _champions(settings):
        version = _text(redis_client.get(champion_key))
        if not version:
            continue
        raw = _text(redis_client.get(f"{settings.cache_prefix}:model:meta:{version}"))
        if not raw:
            outcome[model] = f"skipped: no meta for Redis champion {version}"
            log.warning("%s: %s", model, outcome[model])
            continue
        try:
            stored = json.loads(raw).get("metrics") or {}
        except (ValueError, AttributeError):
            outcome[model] = f"skipped: the meta of Redis champion {version} is not valid JSON"
            log.warning("%s: %s", model, outcome[model])
            continue
        metrics = _safe_metrics(stored)
        protocol = stored.get("eval_protocol")
        lineage = {"eval_protocol": protocol} if protocol and _safe_protocol(protocol) else {}
        try:
            modelregistry.seed(model, metrics, lineage, f"migrated from the Redis champion {version}")
        except GateError as exc:  # the model already has versions: leave it alone
            outcome[model] = f"skipped: {exc}"
            log.info("%s left alone: %s", model, exc)
            continue
        except RegistryUnavailable:
            raise
        except Exception as exc:  # any other registry failure: nothing more is seeded
            raise RegistryUnavailable(f"seeding {model} failed: {exc}") from exc
        outcome[model] = "seeded"
        log.info("%s seeded as v1 from the Redis champion %s", model, version)
    return outcome
