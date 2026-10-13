"""Publish a promoted model as one generation, and restore the previous one (recsys-generations).

Publish order (a crash before step 3 leaves serving exactly as it was):
  1. write the generation's Redis keys                      (invisible: no pointer names it)
  2. write the generation's Qdrant collections (ALS, and two-tower when the stage ran) and its ranker artifact
                                                            (invisible: the aliases do not name them)
  3. move the Qdrant aliases, then the Redis pointers       (the switch; the Redis one is a single Lua EVAL)
  4. delete every generation that is neither serving nor previous

The Redis pointer is the serving decision for BOTH stores: team-ai names its Qdrant collection from
``recs:v1:serving``. The aliases are a deprecated shim for older readers, so a crash between the alias
move and the pointer switch leaves current readers on one consistent generation.
"""

from __future__ import annotations

import logging

from . import registry_client
from .config import Settings
from .load import qdrant as qdrant_load
from .load import redis_cache

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
    two_tower_vectors: dict[str, list[float]] | None = None,
    ranker_artifact: dict | None = None,
) -> dict:
    """Write ``model_version`` as a generation, switch serving to it, drop older generations.

    ``two_tower_vectors`` (the optional two-tower stage) is written with the ALS collections, before
    the switch, so a generation is complete or not visible at all. ``ranker_artifact`` (the promoted GBDT
    ranker) is written to ``recs:v1:gen:<generation>:ranker`` at the same point."""
    if redis_client is None:
        redis_client = redis_cache.connect(settings)
    if qdrant_client is None:
        qdrant_client = qdrant_load._connect(settings)

    cache_counts = redis_cache.load_cache(
        settings, model_version, user_recs, item_recs, popular, client=redis_client, legacy=False
    )
    qdrant_counts = qdrant_load.load_vectors(
        settings, model_version, item_rows, user_rows, client=qdrant_client
    )
    two_tower_items = None
    if two_tower_vectors is not None:
        two_tower_items = qdrant_load.load_two_tower_vectors(
            settings, model_version, two_tower_vectors, client=qdrant_client
        )

    ranker_key = None
    if ranker_artifact is not None:
        ranker_key = redis_cache.load_ranker(settings, model_version, ranker_artifact, client=redis_client)

    qdrant_load.activate_aliases(settings, model_version, client=qdrant_client)
    redis_cache.load_legacy_cache(settings, user_recs, item_recs, popular, client=redis_client)
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
        "two_tower_items": two_tower_items,
        "ranker_key": ranker_key,
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


def rollback(settings: Settings, redis_client=None, qdrant_client=None) -> dict:
    """Make the previous generation the serving one, registry first.

    Preconditions are checked before anything moves (a previous generation with keys and collections, a
    ``previous`` version with an artifact for every carried model, and serving and the registry in step);
    then ``champion`` and ``previous`` are swapped for each model both generations carry, ALS first, then
    GBDT, then two-tower; then the aliases and the pointer. Raises RollbackRefused, changing nothing, when
    a precondition fails; a registry failure part-way swaps the moved models back and re-raises."""
    if redis_client is None:
        redis_client = redis_cache.connect(settings)
    if qdrant_client is None:
        qdrant_client = qdrant_load._connect(settings)

    serving, previous = redis_cache.pointers(settings, redis_client)
    if not previous:
        raise RollbackRefused(f"no previous generation ({settings.previous_key} is not set)")
    if not serving:
        raise RollbackRefused(f"no serving generation ({settings.serving_key} is not set)")
    if not redis_cache.generation_present(settings, previous, redis_client):
        raise RollbackRefused(f"previous generation {previous} has no keys left in Redis")
    if not qdrant_load.generation_present(settings, previous, client=qdrant_client):
        raise RollbackRefused(f"previous generation {previous} has no Qdrant collections left")

    models = _carried_models(settings, serving, previous, redis_client, qdrant_client)
    swapped_already = _check_registry(models, serving, previous)

    # A crash before the pointer swap leaves serving as it was, and a retry finds the registry swapped.
    moved: list[tuple[str, int, int | None]] = []
    if not swapped_already:
        _swap_registry(models, moved)
    try:
        qdrant_load.activate_aliases(settings, previous, client=qdrant_client)
        swapped = redis_cache.swap_generations(settings, serving, client=redis_client)
    except Exception:
        _undo_registry(moved)
        raise
    if swapped in (redis_cache.SWAP_NO_PREVIOUS, redis_cache.SWAP_STALE):  # serving moved since it was read
        qdrant_load.activate_aliases(settings, serving, client=qdrant_client)
        _undo_registry(moved)
        raise RollbackRefused(f"serving changed while rolling back ({swapped}); nothing was changed")
    redis_cache.refresh_ttl(settings, {previous, serving}, client=redis_client)
    log.info("rolled back: serving=%s previous=%s models=%s", previous, serving, models)
    return {"serving": previous, "previous": serving}


def _carried_models(settings, serving, previous, redis_client, qdrant_client) -> list[str]:
    """The models both generations carry (ALS, then GBDT, then two-tower); logs the ones left out."""
    probes = (
        (registry_client.GBDT_MODEL, lambda g: redis_cache.ranker_present(settings, g, redis_client)),
        (
            registry_client.TWO_TOWER_MODEL,
            lambda g: qdrant_load.two_tower_present(settings, g, client=qdrant_client),
        ),
    )
    models = [registry_client.ALS_MODEL]
    for model, present in probes:
        in_serving, in_previous = present(serving), present(previous)
        if in_serving and in_previous:
            models.append(model)
        elif in_serving or in_previous:
            log.info(
                "%s is carried only by generation %s: not rolled back",
                model,
                serving if in_serving else previous,
            )
    return models


def _check_registry(models: list[str], serving: str, previous: str) -> bool:
    """Refuse (RollbackRefused) unless every carried model can be rolled back. Returns True when an
    earlier attempt already swapped the registry and crashed before the pointer switch."""
    states = []
    for model in models:
        champion = registry_client.alias_info(model, "champion")
        prev = registry_client.alias_info(model, "previous")
        champion_gen = champion["generation"] if champion else None
        previous_gen = prev["generation"] if prev else None
        if champion_gen == serving and prev is None:
            raise RollbackRefused(f"{model} has no previous version to roll back to")
        if champion_gen == serving and previous_gen == previous:
            if not prev["has_artifact"]:
                raise RollbackRefused(f"{model} previous v{prev['version']} has no artifact to roll back to")
            states.append("in step")
        elif champion_gen == previous and previous_gen == serving:
            states.append("swapped")
        else:
            raise RollbackRefused(
                f"{model} is out of step with serving {serving}: champion was built for {champion_gen}, "
                f"previous for {previous_gen}"
            )
    if len(set(states)) > 1:
        raise RollbackRefused(f"the registry is half rolled back ({dict(zip(models, states, strict=True))})")
    return states[0] == "swapped"


def _swap_registry(models: list[str], moved: list[tuple[str, int, int | None]]) -> None:
    """Swap champion and previous of every model; on a failure swap back the ones moved and re-raise."""
    for model in models:
        champion = registry_client.alias_info(model, "champion")
        prev = registry_client.alias_info(model, "previous")
        moved.append((model, champion["version"], prev["version"] if prev else None))
        try:
            registry_client.rollback_champion(model)
        except Exception as exc:  # the failing model may be half swapped: it is put back too
            _undo_registry(moved)
            if isinstance(exc, registry_client.RegistryUnavailable):
                raise
            raise registry_client.RegistryUnavailable(f"registry swap of {model} failed: {exc}") from exc


def _undo_registry(moved: list[tuple[str, int, int | None]]) -> None:
    """Best effort: write the original aliases back, latest swap first."""
    while moved:
        model, champion, previous = moved.pop()
        try:
            registry_client.restore_aliases(model, champion, previous)
        except Exception as exc:  # noqa: BLE001 - keep restoring the rest, report what is left
            log.error("could not restore %s (champion v%s, previous v%s): %s", model, champion, previous, exc)
