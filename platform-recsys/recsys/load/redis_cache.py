"""Write the precomputed Top-N recommendation cache into Redis (:6379).

Key shapes match the consumer contract (serve-recommendations-teamai):

- ``recs:v1:user:{user_key}``     → ranked ``[{listing_id, score}, ...]`` (JSON), capped at TOP_N
- ``recs:v1:item:{listing_id}``   → precomputed similar items (same shape)
- ``recs:v1:popular``             → global popularity fallback list (same shape)
- ``recs:v1:model_version``       → the current model_version string (echoed by the RPC)

Every key carries a TTL longer than the batch cadence so a missed run degrades
gracefully rather than emptying the cache. Values hold listing ids + scores only
— no hydrated listing content (Rule 3). ``recs:v1:model_version`` is written last
so the version flips only once the generation is fully loaded.
"""

from __future__ import annotations

import json


def _encode(pairs) -> str:
    return json.dumps([{"listing_id": lid, "score": round(float(score), 6)} for lid, score in pairs])


def load_cache(
    settings,
    model_version: str,
    user_recs: dict[str, list[tuple[str, float]]],
    item_recs: dict[str, list[tuple[str, float]]],
    popular: list[tuple[str, float]],
    client=None,
) -> dict[str, int]:
    """Write per-user, per-item, popular, and model_version keys. Returns counts."""
    if client is None:
        from redis import Redis  # noqa: PLC0415

        client = Redis(
            host=settings.redis_host,
            port=settings.redis_port,
            password=settings.redis_password or None,
            db=settings.redis_db,
            decode_responses=True,
        )

    ttl = settings.cache_ttl_seconds
    n_users = 0
    n_items = 0

    pipe = client.pipeline()
    for user_key, pairs in user_recs.items():
        pipe.set(settings.user_cache_key(user_key), _encode(pairs[: settings.top_n]), ex=ttl)
        n_users += 1
    for listing_id, pairs in item_recs.items():
        pipe.set(settings.item_cache_key(listing_id), _encode(pairs[: settings.top_n]), ex=ttl)
        n_items += 1
    pipe.set(settings.popular_cache_key, _encode(popular[: settings.top_n]), ex=ttl)
    pipe.execute()

    # Flip the version only after the generation is fully written.
    client.set(settings.model_version_cache_key, model_version, ex=ttl)

    return {"users": n_users, "items": n_items}
