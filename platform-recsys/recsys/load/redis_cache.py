"""Write the precomputed Top-N recommendation cache into Redis (:6379), one generation at a time.

A generation is one ``model_version``. Its keys are scoped to it and never overwrite another
generation's (recsys-generations):

- ``recs:v1:gen:<gen>:user:{user_key}``    → ranked ``[{listing_id, score}, ...]`` (JSON), capped at TOP_N
- ``recs:v1:gen:<gen>:item:{listing_id}``  → precomputed similar items (same shape)
- ``recs:v1:gen:<gen>:popular``            → global popularity fallback list (same shape)

Pointers (no TTL), moved together by ONE Lua script so readers never see them disagree:

- ``recs:v1:serving``        → the generation being served
- ``recs:v1:previous``       → the generation ``serving`` replaced (the rollback target)
- ``recs:v1:model_version``  → mirrors ``serving`` for readers that predate generations

Generation keys carry a TTL longer than the batch cadence; the job refreshes the TTL of the
serving and previous generations on every run so a rollback never lands on expired keys.
With ``RECS_WRITE_LEGACY_KEYS`` (default true) the unscoped ``recs:v1:{user,item,popular}`` keys
are written too, as a one-release shim for a reverted team-ai. Values hold listing ids + scores
only — no hydrated listing content (Rule 3).
"""

from __future__ import annotations

import json
import re

# KEYS: serving, previous, model_version. ARGV[1]: the new generation.
# Re-publishing the serving generation leaves previous alone.
_PROMOTE_LUA = """
local old = redis.call('GET', KEYS[1])
if old and old ~= ARGV[1] then
  redis.call('SET', KEYS[2], old)
end
redis.call('SET', KEYS[1], ARGV[1])
redis.call('SET', KEYS[3], ARGV[1])
return old or ''
"""

# KEYS: serving, previous, model_version. ARGV[1]: the serving generation the caller expects
# ('' when unset). Compare-and-set: swap serving and previous only if serving still equals it.
# Returns 0 when serving has moved on (the swap was already applied), -1 when there is no
# previous, else {old_serving, new_serving}.
_ROLLBACK_LUA = """
local s = redis.call('GET', KEYS[1])
if (s or '') ~= ARGV[1] then
  return 0
end
local p = redis.call('GET', KEYS[2])
if not p then
  return -1
end
redis.call('SET', KEYS[1], p)
if s then
  redis.call('SET', KEYS[2], s)
else
  redis.call('DEL', KEYS[2])
end
redis.call('SET', KEYS[3], p)
return {s or '', p}
"""

SWAP_STALE = "stale"
SWAP_NO_PREVIOUS = "no_previous"


def connect(settings):
    from redis import Redis  # noqa: PLC0415

    return Redis(
        host=settings.redis_host,
        port=settings.redis_port,
        password=settings.redis_password or None,
        db=settings.redis_db,
        decode_responses=True,
    )


def _encode(pairs) -> str:
    return json.dumps([{"listing_id": lid, "score": round(float(score), 6)} for lid, score in pairs])


def _text(value) -> str | None:
    if value is None:
        return None
    return value.decode("utf-8") if isinstance(value, bytes) else str(value)


def load_cache(
    settings,
    model_version: str,
    user_recs: dict[str, list[tuple[str, float]]],
    item_recs: dict[str, list[tuple[str, float]]],
    popular: list[tuple[str, float]],
    client=None,
) -> dict[str, int]:
    """Write the generation's per-user, per-item and popular keys. Returns counts.

    Does NOT move any pointer: the generation stays invisible until ``activate_generation``.
    """
    if client is None:
        client = connect(settings)

    ttl = settings.cache_ttl_seconds
    n_users = 0
    n_items = 0

    pipe = client.pipeline()
    for user_key, pairs in user_recs.items():
        body = _encode(pairs[: settings.top_n])
        pipe.set(settings.gen_user_key(model_version, user_key), body, ex=ttl)
        if settings.write_legacy_keys:
            pipe.set(settings.user_cache_key(user_key), body, ex=ttl)
        n_users += 1
    for listing_id, pairs in item_recs.items():
        body = _encode(pairs[: settings.top_n])
        pipe.set(settings.gen_item_key(model_version, listing_id), body, ex=ttl)
        if settings.write_legacy_keys:
            pipe.set(settings.item_cache_key(listing_id), body, ex=ttl)
        n_items += 1
    popular_body = _encode(popular[: settings.top_n])
    pipe.set(settings.gen_popular_key(model_version), popular_body, ex=ttl)
    if settings.write_legacy_keys:
        pipe.set(settings.popular_cache_key, popular_body, ex=ttl)
    pipe.execute()

    return {"users": n_users, "items": n_items}


def pointers(settings, client) -> tuple[str | None, str | None]:
    """(serving, previous) generations, None when unset."""
    return _text(client.get(settings.serving_key)), _text(client.get(settings.previous_key))


def activate_generation(settings, model_version: str, client=None) -> str | None:
    """Atomically make ``model_version`` the serving generation. Returns the one it replaced."""
    if client is None:
        client = connect(settings)
    old = client.eval(
        _PROMOTE_LUA,
        3,
        settings.serving_key,
        settings.previous_key,
        settings.model_version_cache_key,
        model_version,
    )
    return _text(old) or None


def swap_generations(settings, expected_serving: str | None, client=None):
    """Compare-and-set swap of serving and previous.

    Returns (old_serving, new_serving) when swapped, ``SWAP_STALE`` when serving no longer equals
    ``expected_serving`` (nothing changed), ``SWAP_NO_PREVIOUS`` when there is no previous."""
    if client is None:
        client = connect(settings)
    out = client.eval(
        _ROLLBACK_LUA,
        3,
        settings.serving_key,
        settings.previous_key,
        settings.model_version_cache_key,
        expected_serving or "",
    )
    if isinstance(out, (list, tuple)):
        return (_text(out[0]) or "", _text(out[1]) or "")
    return SWAP_STALE if int(out) == 0 else SWAP_NO_PREVIOUS


_GEN_KEY = re.compile(r"^(?P<gen>.+?):(?:user:|item:|popular$)")


def _scan_generation_keys(settings, client):
    """Yield (gen, key) for every generation-scoped key."""
    prefix = settings.gen_key_prefix
    for key in client.scan_iter(match=f"{prefix}*", count=1000):
        key = _text(key)
        m = _GEN_KEY.match(key[len(prefix) :])
        if m:
            yield m.group("gen"), key


def generation_present(settings, gen: str, client) -> bool:
    """True when the generation's keys still exist (its popular list is the sentinel)."""
    return bool(client.exists(settings.gen_popular_key(gen)))


def prune_generations(settings, keep: set[str], client=None) -> int:
    """Delete the keys of every generation not in ``keep``. Returns the number of keys deleted.

    An empty ``keep`` (no serving/previous readable) deletes nothing."""
    if not keep:
        return 0
    if client is None:
        client = connect(settings)
    doomed = [key for gen, key in _scan_generation_keys(settings, client) if gen not in keep]
    for i in range(0, len(doomed), 500):
        client.delete(*doomed[i : i + 500])
    return len(doomed)


def refresh_ttl(settings, gens: set[str], client=None) -> int:
    """EXPIRE every key of the given generations back to the full TTL. Returns keys touched."""
    if client is None:
        client = connect(settings)
    ttl = settings.cache_ttl_seconds
    n = 0
    for gen, key in _scan_generation_keys(settings, client):
        if gen in gens:
            client.expire(key, ttl)
            n += 1
    return n
