"""``python -m recsys backfill-attributes``: add the candidate attributes to what is already published.

Patches the serving and previous generations in place, from the latest ``item_attributes@v2`` snapshot:

- Qdrant: ``set_payload`` on the item collections (ALS, and two-tower when present) of those generations,
  at most 256 points per call. Vectors, user collections and other generations are untouched.
- Redis: rewrites the user, item and popular entries of those generations and the legacy ``recs:v1:*``
  entries, keeping each key's remaining TTL (a persistent key stays persistent, a key that expired meanwhile
  is skipped). Pointers, ``recs:v1:model_version`` and the ranker artifact are untouched.

Exit 0 on success (including no serving generation: nothing to do), 2 when the snapshot is missing or
corrupt (nothing written), 1 when Redis or Qdrant fails. Logs carry counts, never ids.
"""

from __future__ import annotations

import json
import logging

from .load import attributes as attributes_load
from .load import qdrant as qdrant_load
from .load import redis_cache

log = logging.getLogger("recsys.backfill")

EXIT_FAILED = 1
EXIT_REFUSED = 2
BATCH = 256


def _patch_collection(client, collection: str, attributes) -> int:
    groups: dict[str, tuple[dict, list]] = {}
    offset = None
    while True:
        points, offset = client.scroll(
            collection_name=collection, limit=BATCH, offset=offset, with_vectors=False, with_payload=True
        )
        for point in points:
            listing_id = (point.payload or {}).get("listing_id")
            attrs = attributes.get(listing_id) if listing_id else None
            if attrs:
                groups.setdefault(json.dumps(attrs, sort_keys=True), (attrs, []))[1].append(point.id)
        if offset is None:
            break
    patched = 0
    for attrs, ids in groups.values():
        for i in range(0, len(ids), BATCH):
            batch = ids[i : i + BATCH]
            client.set_payload(collection_name=collection, payload=attrs, points=batch)
            patched += len(batch)
    return patched


def _patch_qdrant(settings, client, generations, attributes) -> int:
    existing = qdrant_load._real_collections(client)
    patched = 0
    for gen in generations:
        for base in (settings.qdrant_item_collection, settings.qdrant_two_tower_collection):
            name = qdrant_load.generation_collection(base, gen)
            if name in existing:
                patched += _patch_collection(client, name, attributes)
    return patched


def _rewrite(raw, attributes) -> str | None:
    try:
        entries = json.loads(raw)
    except (TypeError, ValueError):
        return None
    if not isinstance(entries, list) or not all(isinstance(e, dict) and "listing_id" in e for e in entries):
        return None
    out = []
    for entry in entries:
        base = {k: v for k, v in entry.items() if k not in attributes_load.ATTRIBUTE_KEYS}
        base.update(attributes.get(entry["listing_id"], {}))
        out.append(base)
    return json.dumps(out)


def _patterns(settings, generations) -> list[str]:
    prefix = f"{settings.cache_prefix}:{settings.cache_schema_version}"
    patterns = [f"{prefix}:user:*", f"{prefix}:item:*", f"{prefix}:popular"]
    for gen in generations:
        base = f"{settings.gen_key_prefix}{gen}"
        patterns += [f"{base}:user:*", f"{base}:item:*", f"{base}:popular"]
    return patterns


def _patch_redis(settings, client, generations, attributes) -> int:
    rewritten = 0
    for pattern in _patterns(settings, generations):
        for key in client.scan_iter(match=pattern, count=1000):
            pttl = client.pttl(key)
            if pttl == -2:
                continue
            raw = client.get(key)
            body = _rewrite(raw, attributes) if raw is not None else None
            if body is None:
                continue
            if pttl == -1:
                client.set(key, body, xx=True)
            else:
                client.set(key, body, px=max(pttl, 1), xx=True)
            rewritten += 1
    return rewritten


def backfill(settings) -> int:
    attributes = attributes_load.load_attributes(settings)
    if attributes is None:
        log.error("backfill refused: no readable item_attributes v2 snapshot, nothing was written")
        return EXIT_REFUSED
    try:
        redis_client = redis_cache.connect(settings)
        serving, previous = redis_cache.pointers(settings, redis_client)
        generations = [g for g in dict.fromkeys((serving, previous)) if g]
        if not generations:
            log.info("backfill: no serving generation, nothing to do")
            return 0
        points = _patch_qdrant(settings, qdrant_load._connect(settings), generations, attributes)
        keys = _patch_redis(settings, redis_client, generations, attributes)
    except Exception as exc:  # noqa: BLE001
        log.error("backfill failed (%s); it is safe to re-run", type(exc).__name__)
        return EXIT_FAILED
    log.info("backfill done: generations=%d points=%d keys=%d", len(generations), points, keys)
    return 0
