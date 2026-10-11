"""Serving-generation pointer: scoped keys, 5 s memo, fail-open fallback."""

from __future__ import annotations

from app.modules.business.recommend.cache import PrecomputedCache


class _Redis:
    def __init__(self, values=None, *, fail_keys=()):
        self.values = values or {}
        self.fail_keys = set(fail_keys)
        self.gets: list[str] = []

    async def get(self, key):
        self.gets.append(key)
        if key in self.fail_keys:
            raise ConnectionError("down")
        return self.values.get(key)


class _Clock:
    def __init__(self):
        self.now = 1000.0

    def __call__(self):
        return self.now


def _cache(redis, clock=None):
    return PrecomputedCache(
        redis, prefix="recs", schema_version="v1", clock=clock or _Clock()
    )


async def test_pointer_set_scopes_user_and_popular_keys():
    redis = _Redis(
        {
            "recs:v1:serving": "g2",
            "recs:v1:gen:g2:user:u1": '["a"]',
            "recs:v1:gen:g2:popular": '["p"]',
            "recs:v1:user:u1": '["legacy"]',
        }
    )
    cache = _cache(redis)
    assert [c.listing_id for c in await cache.get_user_candidates("u1")] == ["a"]
    assert [c.listing_id for c in await cache.get_popular_candidates()] == ["p"]
    assert cache.item_key("x", "g2") == "recs:v1:gen:g2:item:x"


async def test_pointer_absent_uses_unscoped_keys_and_model_version():
    redis = _Redis({"recs:v1:user:u1": '["legacy"]', "recs:v1:model_version": b"v0"})
    cache = _cache(redis)
    assert [c.listing_id for c in await cache.get_user_candidates("u1")] == ["legacy"]
    assert await cache.get_model_version() == "v0"


async def test_model_version_is_pointer_when_set():
    redis = _Redis({"recs:v1:serving": b"g2", "recs:v1:model_version": "old"})
    assert await _cache(redis).get_model_version() == "g2"


async def test_pointer_memoised_for_5s_then_refreshed():
    clock = _Clock()
    redis = _Redis({"recs:v1:serving": "g1"})
    cache = _cache(redis, clock)
    assert await cache.get_model_version() == "g1"
    redis.values["recs:v1:serving"] = "g2"
    clock.now += 4.9
    assert await cache.get_model_version() == "g1"
    clock.now += 0.2
    assert await cache.get_model_version() == "g2"
    assert redis.gets.count("recs:v1:serving") == 2


async def test_pointer_read_error_counts_as_absent():
    redis = _Redis(
        {"recs:v1:user:u1": '["legacy"]', "recs:v1:model_version": "v0"},
        fail_keys={"recs:v1:serving"},
    )
    cache = _cache(redis)
    assert [c.listing_id for c in await cache.get_user_candidates("u1")] == ["legacy"]
    assert await cache.get_model_version() == "v0"
