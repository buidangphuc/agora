"""Publish a promoted model as one generation, and restore the previous one (recsys-generations).

Publish order (a crash before step 3 leaves serving exactly as it was):
  1. write the generation's Redis keys                      (invisible: no pointer names it)
  2. write the generation's Qdrant collections              (invisible: the aliases do not name them)
  3. move the Qdrant aliases, then the Redis pointers       (the switch; the Redis one is a single Lua EVAL)
  4. delete every generation that is neither serving nor previous

The Redis pointer is the serving decision for BOTH stores: team-ai names its Qdrant collection from
``recs:v1:serving``. The aliases are a deprecated shim for older readers, so a crash between the alias
move and the pointer switch leaves current readers on one consistent generation.
"""

from __future__ import annotations

import logging

from .config import Settings
from .load import qdrant as qdrant_load
from .load import redis_cache
from .registry.registry import ModelRegistry

log = logging.getLogger("recsys.publish")

EXIT_REFUSED = 2


class RollbackRefused(Exception):
    """There is nothing safe to roll back to; nothing was changed."""


def publish_generation(
    settings: Settings,
    model_version: str,
    user_recs,
    item_recs,
    popular,
    item_rows,
    user_rows,
    redis_client=None,
    qdrant_client=None,
) -> dict:
    """Write ``model_version`` as a generation, switch serving to it, drop older generations."""
    if redis_client is None:
        redis_client = redis_cache.connect(settings)
    if qdrant_client is None:
        qdrant_client = qdrant_load._connect(settings)

    cache_counts = redis_cache.load_cache(
        settings, model_version, user_recs, item_recs, popular, client=redis_client
    )
    qdrant_counts = qdrant_load.load_vectors(
        settings, model_version, item_rows, user_rows, client=qdrant_client
    )

    qdrant_load.activate_aliases(settings, model_version, client=qdrant_client)
    replaced = redis_cache.activate_generation(settings, model_version, client=redis_client)

    serving, previous = redis_cache.pointers(settings, redis_client)
    keep = {g for g in (serving, previous) if g}
    dropped_keys = redis_cache.prune_generations(settings, keep, client=redis_client)
    dropped_collections = qdrant_load.prune_generations(settings, keep, client=qdrant_client)
    redis_cache.refresh_ttl(settings, keep, client=redis_client)
    log.info(
        "generation %s is serving (previous=%s, replaced=%s); dropped %d keys, collections=%s",
        model_version,
        previous,
        replaced,
        dropped_keys,
        dropped_collections,
    )
    return {
        "qdrant": qdrant_counts,
        "cache": cache_counts,
        "serving": serving,
        "previous": previous,
        "dropped_keys": dropped_keys,
        "dropped_collections": dropped_collections,
    }


def refresh_serving_ttl(settings: Settings, redis_client=None) -> int:
    """Refresh the TTL of the serving and previous generations (every run, promoted or not)."""
    if redis_client is None:
        redis_client = redis_cache.connect(settings)
    serving, previous = redis_cache.pointers(settings, redis_client)
    keep = {g for g in (serving, previous) if g}
    return redis_cache.refresh_ttl(settings, keep, client=redis_client) if keep else 0


def rollback(
    settings: Settings,
    registry: ModelRegistry,
    redis_client=None,
    qdrant_client=None,
) -> dict:
    """Make the previous generation the serving one. Raises RollbackRefused, changing nothing,
    when there is no previous generation or its keys or collections are gone."""
    if redis_client is None:
        redis_client = redis_cache.connect(settings)
    if qdrant_client is None:
        qdrant_client = qdrant_load._connect(settings)

    serving, previous = redis_cache.pointers(settings, redis_client)
    if not previous:
        raise RollbackRefused(f"no previous generation ({settings.previous_key} is not set)")
    if not redis_cache.generation_present(settings, previous, redis_client):
        raise RollbackRefused(f"previous generation {previous} has no keys left in Redis")
    if not qdrant_load.generation_present(settings, previous, client=qdrant_client):
        raise RollbackRefused(f"previous generation {previous} has no Qdrant collections left")

    # Crash after the pointer swap but before the registry follow-up: serving already names the
    # restored model while the registry still has the old champion. A restored model was a
    # champion once; a publish that crashed before its promotion leaves serving on a model still
    # marked "candidate".
    # Converge on serving; never swap back.
    champion = registry.get_champion_version()
    restored = registry.get_model(serving) if serving else None
    if serving and champion and champion != serving and restored and restored.status != "candidate":
        return _converge(settings, registry, serving, redis_client, qdrant_client)

    # Aliases first: moving them is idempotent, so a retry after a crash converges.
    qdrant_load.activate_aliases(settings, previous, client=qdrant_client)
    swapped = redis_cache.swap_generations(settings, serving, client=redis_client)
    if swapped == redis_cache.SWAP_NO_PREVIOUS:  # previous vanished between the check and the swap
        raise RollbackRefused("previous generation was cleared while rolling back")
    if swapped == redis_cache.SWAP_STALE:  # serving moved since we read it: already applied
        current, _ = redis_cache.pointers(settings, redis_client)
        return _converge(settings, registry, current, redis_client, qdrant_client)
    registry.restore_champion(previous)
    redis_cache.refresh_ttl(settings, {previous, serving} - {None, ""}, client=redis_client)
    log.info("rolled back: serving=%s previous=%s", previous, serving)
    return {"serving": previous, "previous": serving}


def _converge(settings, registry, serving, redis_client, qdrant_client) -> dict:
    """The swap already happened: make the aliases and the registry champion match ``serving``."""
    qdrant_load.activate_aliases(settings, serving, client=qdrant_client)
    registry.restore_champion(serving)
    _, previous = redis_cache.pointers(settings, redis_client)
    redis_cache.refresh_ttl(settings, {serving, previous} - {None, ""}, client=redis_client)
    log.info("rollback already applied: serving=%s previous=%s", serving, previous)
    return {"serving": serving, "previous": previous}
