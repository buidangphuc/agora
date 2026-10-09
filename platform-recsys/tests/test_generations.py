"""Generation publish and rollback against in-memory Redis/Qdrant.

Every scenario runs against the hand-written fakes; the pointer scripts are also run as real Lua
against fakeredis (needs ``fakeredis`` + ``lupa``), and the Qdrant side also against qdrant-client's
local in-memory mode, so the alias semantics are the library's and not only our fake's.
"""

from __future__ import annotations

import pytest

from recsys.config import Settings
from recsys.load import qdrant as qdrant_load
from recsys.load import redis_cache
from recsys.publish import RollbackRefused, publish_generation, rollback
from recsys.registry.metadata import ModelMetadata
from recsys.registry.registry import ModelRegistry
from tests.fakes import FakeQdrantClient, FakeRedis

DIM = 4
SETTINGS = Settings(als_rank=DIM, top_n=3, redis_host="redis.invalid")


def _redis_backends():
    yield "fake"
    try:
        import fakeredis  # noqa: F401
        import lupa  # noqa: F401

        yield "fakeredis"
    except ImportError:
        yield pytest.param("fakeredis", marks=pytest.mark.skip(reason="fakeredis/lupa not installed"))


def _qdrant_backends():
    yield "fake"
    try:
        import qdrant_client  # noqa: F401

        yield "local"
    except ImportError:
        yield pytest.param("local", marks=pytest.mark.skip(reason="qdrant-client not installed"))


@pytest.fixture(params=list(_redis_backends()))
def redis_client(request):
    if request.param == "fake":
        return FakeRedis()
    import fakeredis

    return fakeredis.FakeRedis(decode_responses=True)


@pytest.fixture(params=list(_qdrant_backends()))
def qdrant(request):
    if request.param == "fake":
        return FakeQdrantClient()
    from qdrant_client import QdrantClient

    return QdrantClient(":memory:")


def _recs(tag: str):
    users = {f"u{i}": [(f"{tag}-l{i}", 0.9), (f"{tag}-l{i + 1}", 0.5)] for i in range(2)}
    items = {f"{tag}-l{i}": [(f"{tag}-l{i + 1}", 0.7)] for i in range(2)}
    popular = [(f"{tag}-l0", 5.0)]
    return users, items, popular


def _rows(tag: str):
    vec = [1.0, 0.0, 0.0, 0.0]
    return [(f"{tag}-l{i}", vec) for i in range(3)], [(f"u{i}", vec) for i in range(2)]


def _publish(redis_client, qdrant, gen, settings=SETTINGS):
    users, items, popular = _recs(gen)
    item_rows, user_rows = _rows(gen)
    return publish_generation(
        settings,
        gen,
        users,
        items,
        popular,
        item_rows,
        user_rows,
        redis_client=redis_client,
        qdrant_client=qdrant,
    )


def _aliases(qdrant):
    return {a.alias_name: a.collection_name for a in qdrant.get_aliases().aliases}


def _collections(qdrant):
    return {c.name for c in qdrant.get_collections().collections}


def _gen_keys(redis_client, gen):
    return {k for k in redis_client.scan_iter(match=f"recs:v1:gen:{gen}:*")}


def _registry_with(redis_client, *versions):
    reg = ModelRegistry(redis_client=redis_client)
    for v in versions:
        reg.register_model(ModelMetadata(model_name="als", model_version=v, model_type="als", metrics={}))
        reg.evaluate_and_promote(v, force=True)
    return reg


# ── publish ──────────────────────────────────────────────────────────────────────────────────────


def test_generation_is_invisible_until_activated(redis_client, qdrant):
    users, items, popular = _recs("g1")
    redis_cache.load_cache(SETTINGS, "g1", users, items, popular, client=redis_client)
    item_rows, user_rows = _rows("g1")
    qdrant_load.load_vectors(SETTINGS, "g1", item_rows, user_rows, client=qdrant)

    assert redis_client.get("recs:v1:serving") is None
    assert redis_client.get("recs:v1:model_version") is None
    assert _aliases(qdrant) == {}
    assert "item_als_vectors__g1" in _collections(qdrant)
    assert "recs:v1:gen:g1:user:u0" in _gen_keys(redis_client, "g1")


def test_publish_sets_pointers_alias_and_mirror(redis_client, qdrant):
    out = _publish(redis_client, qdrant, "g1")

    assert out["serving"] == "g1" and out["previous"] is None
    assert redis_client.get("recs:v1:serving") == "g1"
    assert redis_client.get("recs:v1:model_version") == "g1"
    assert redis_client.get("recs:v1:previous") is None
    assert _aliases(qdrant) == {
        "item_als_vectors": "item_als_vectors__g1",
        "user_als_vectors": "user_als_vectors__g1",
    }


def test_second_promotion_keeps_the_first_as_previous(redis_client, qdrant):
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")

    assert redis_client.get("recs:v1:serving") == "g2"
    assert redis_client.get("recs:v1:previous") == "g1"
    assert redis_client.get("recs:v1:model_version") == "g2"
    assert _aliases(qdrant)["item_als_vectors"] == "item_als_vectors__g2"
    assert {"item_als_vectors__g1", "item_als_vectors__g2"} <= _collections(qdrant)
    assert _gen_keys(redis_client, "g1") and _gen_keys(redis_client, "g2")


def test_third_promotion_drops_the_oldest_generation(redis_client, qdrant):
    for gen in ("g1", "g2", "g3"):
        _publish(redis_client, qdrant, gen)

    assert redis_client.get("recs:v1:serving") == "g3"
    assert redis_client.get("recs:v1:previous") == "g2"
    assert _gen_keys(redis_client, "g1") == set()
    assert _gen_keys(redis_client, "g2") and _gen_keys(redis_client, "g3")
    names = _collections(qdrant)
    assert not {n for n in names if n.endswith("__g1")}
    assert {
        "item_als_vectors__g2",
        "item_als_vectors__g3",
        "user_als_vectors__g2",
        "user_als_vectors__g3",
    } <= names


def test_republishing_the_serving_generation_keeps_previous(redis_client, qdrant):
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    _publish(redis_client, qdrant, "g2")

    assert redis_client.get("recs:v1:serving") == "g2"
    assert redis_client.get("recs:v1:previous") == "g1"
    assert _aliases(qdrant)["item_als_vectors"] == "item_als_vectors__g2"


def test_legacy_keys_shim_is_on_by_default_and_can_be_turned_off(redis_client, qdrant):
    _publish(redis_client, qdrant, "g1")
    assert redis_client.get("recs:v1:user:u0") is not None
    assert redis_client.get("recs:v1:item:g1-l0") is not None
    assert redis_client.get("recs:v1:popular") is not None

    off = Settings(als_rank=DIM, top_n=3, write_legacy_keys=False)
    _publish(redis_client, qdrant, "g2", settings=off)
    assert redis_client.get("recs:v1:gen:g2:popular") is not None
    # The unscoped keys still hold g1: g2 did not write them.
    assert "g1-l0" in redis_client.get("recs:v1:user:u0")


def test_first_publish_migrates_a_plain_collection_behind_the_alias(redis_client, qdrant):
    from qdrant_client import models

    for name in ("item_als_vectors", "user_als_vectors"):
        qdrant.create_collection(
            collection_name=name,
            vectors_config=models.VectorParams(size=DIM, distance=models.Distance.COSINE),
        )
        qdrant.upsert(
            collection_name=name,
            points=[
                models.PointStruct(id=qdrant_load.point_id(f"old-{i}"), vector=[1.0] * DIM, payload={"n": i})
                for i in range(300)
            ],
        )

    out = _publish(redis_client, qdrant, "g1")

    assert out["qdrant"]["legacy_migrated"] == 2
    assert _aliases(qdrant) == {
        "item_als_vectors": "item_als_vectors__g1",
        "user_als_vectors": "user_als_vectors__g1",
    }
    # The plain collection is gone (the alias owns the name) and so is the temporary copy.
    assert _collections(qdrant) == {"item_als_vectors__g1", "user_als_vectors__g1"}


def test_migration_copies_every_point_and_serves_until_the_switch(qdrant):
    from qdrant_client import models

    qdrant.create_collection(
        collection_name="item_als_vectors",
        vectors_config=models.VectorParams(size=DIM, distance=models.Distance.COSINE),
    )
    qdrant.upsert(
        collection_name="item_als_vectors",
        points=[
            models.PointStruct(id=qdrant_load.point_id(f"old-{i}"), vector=[1.0] * DIM, payload={"n": i})
            for i in range(300)
        ],
    )
    item_rows, user_rows = _rows("g1")
    qdrant_load.load_vectors(SETTINGS, "g1", item_rows, user_rows, client=qdrant)

    # Before the switch the alias still serves the old data, now from the copy.
    assert _aliases(qdrant)["item_als_vectors"] == "item_als_vectors__legacy"
    assert qdrant.count(collection_name="item_als_vectors").count == 300


def test_ttl_of_serving_and_previous_is_refreshed_and_pointers_never_expire(redis_client, qdrant):
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    _publish(redis_client, qdrant, "g3")
    keys = _gen_keys(redis_client, "g2") | _gen_keys(redis_client, "g3")
    for key in keys:
        redis_client.expire(key, 5)

    from recsys.publish import refresh_serving_ttl

    assert refresh_serving_ttl(SETTINGS, redis_client) == len(keys)

    for key in keys:
        assert 5 < redis_client.ttl(key) <= SETTINGS.cache_ttl_seconds
    for pointer in ("recs:v1:serving", "recs:v1:previous", "recs:v1:model_version"):
        assert redis_client.ttl(pointer) == -1


def test_pointer_switch_is_one_script_call():
    redis_client = FakeRedis()
    redis_cache.activate_generation(SETTINGS, "g1", client=redis_client)
    redis_cache.activate_generation(SETTINGS, "g2", client=redis_client)
    assert redis_client.evals == 2  # one EVAL per switch: serving, previous and the mirror together


def test_lua_switch_moves_all_three_keys_together():
    fakeredis = pytest.importorskip("fakeredis")
    pytest.importorskip("lupa")
    r = fakeredis.FakeRedis(decode_responses=True)
    assert redis_cache.activate_generation(SETTINGS, "g1", client=r) is None
    assert redis_cache.activate_generation(SETTINGS, "g2", client=r) == "g1"
    assert (r.get("recs:v1:serving"), r.get("recs:v1:previous"), r.get("recs:v1:model_version")) == (
        "g2",
        "g1",
        "g2",
    )
    assert redis_cache.swap_generations(SETTINGS, client=r) == ("g2", "g1")
    assert (r.get("recs:v1:serving"), r.get("recs:v1:previous"), r.get("recs:v1:model_version")) == (
        "g1",
        "g2",
        "g1",
    )


# ── rollback ─────────────────────────────────────────────────────────────────────────────────────


def test_rollback_restores_the_previous_generation(redis_client, qdrant):
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    registry = _registry_with(redis_client, "g1", "g2")
    assert registry.get_champion_version() == "g2"

    out = rollback(SETTINGS, registry, redis_client=redis_client, qdrant_client=qdrant)

    assert out == {"serving": "g1", "previous": "g2"}
    assert redis_client.get("recs:v1:serving") == "g1"
    assert redis_client.get("recs:v1:previous") == "g2"
    assert redis_client.get("recs:v1:model_version") == "g1"
    assert _aliases(qdrant) == {
        "item_als_vectors": "item_als_vectors__g1",
        "user_als_vectors": "user_als_vectors__g1",
    }
    assert registry.get_champion_version() == "g1"
    assert registry.get_model("g1").status == "champion"
    assert registry.get_model("g2").status == "archived"


def test_rollback_twice_goes_forward_again(redis_client, qdrant):
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    registry = _registry_with(redis_client, "g1", "g2")
    rollback(SETTINGS, registry, redis_client=redis_client, qdrant_client=qdrant)
    rollback(SETTINGS, registry, redis_client=redis_client, qdrant_client=qdrant)

    assert redis_client.get("recs:v1:serving") == "g2"
    assert registry.get_champion_version() == "g2"


def test_rollback_without_a_previous_generation_is_refused_and_changes_nothing(redis_client, qdrant):
    _publish(redis_client, qdrant, "g1")
    registry = _registry_with(redis_client, "g1")
    before_aliases = _aliases(qdrant)

    with pytest.raises(RollbackRefused, match="no previous generation"):
        rollback(SETTINGS, registry, redis_client=redis_client, qdrant_client=qdrant)

    assert redis_client.get("recs:v1:serving") == "g1"
    assert redis_client.get("recs:v1:model_version") == "g1"
    assert _aliases(qdrant) == before_aliases
    assert registry.get_champion_version() == "g1"


def test_rollback_is_refused_when_the_previous_keys_expired(redis_client, qdrant):
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    registry = _registry_with(redis_client, "g1", "g2")
    redis_client.delete(*_gen_keys(redis_client, "g1"))

    with pytest.raises(RollbackRefused, match="no keys left"):
        rollback(SETTINGS, registry, redis_client=redis_client, qdrant_client=qdrant)
    assert redis_client.get("recs:v1:serving") == "g2"
    assert _aliases(qdrant)["item_als_vectors"] == "item_als_vectors__g2"


def test_rollback_is_refused_when_the_previous_collection_is_gone(redis_client, qdrant):
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    registry = _registry_with(redis_client, "g1", "g2")
    qdrant.delete_collection(collection_name="user_als_vectors__g1")

    with pytest.raises(RollbackRefused, match="no Qdrant collections"):
        rollback(SETTINGS, registry, redis_client=redis_client, qdrant_client=qdrant)
    assert redis_client.get("recs:v1:serving") == "g2"


# ── command ──────────────────────────────────────────────────────────────────────────────────────


def _main_with(monkeypatch, redis_client, qdrant, argv):
    from recsys import __main__ as entry
    from recsys.load import qdrant as qdrant_mod

    monkeypatch.setattr(redis_cache, "connect", lambda settings: redis_client)
    monkeypatch.setattr(qdrant_mod, "_connect", lambda settings: qdrant)
    monkeypatch.setattr(entry, "load_settings", lambda: SETTINGS)
    return entry.main(argv)


def test_command_rollback_exit_codes(monkeypatch):
    redis_client, qdrant = FakeRedis(), FakeQdrantClient()
    _publish(redis_client, qdrant, "g1")
    assert _main_with(monkeypatch, redis_client, qdrant, ["rollback"]) == 2
    assert redis_client.store["recs:v1:serving"] == "g1"

    _publish(redis_client, qdrant, "g2")
    assert _main_with(monkeypatch, redis_client, qdrant, ["rollback"]) == 0
    assert redis_client.store["recs:v1:serving"] == "g1"


def test_command_rejects_unknown_subcommands(monkeypatch):
    assert _main_with(monkeypatch, FakeRedis(), FakeQdrantClient(), ["frobnicate"]) == 2



def test_legacy_migration_keeps_the_old_vector_size(redis_client, qdrant):
    """A legacy collection trained at another ALS rank is copied at ITS size, not the new one.

    The old code sized the copy for the NEW rank, which a real Qdrant refuses ("Vector
    dimension error"); the publish must now succeed.
    """
    from qdrant_client import models

    old_dim = DIM + 4
    qdrant.create_collection(
        collection_name="item_als_vectors",
        vectors_config=models.VectorParams(size=old_dim, distance=models.Distance.COSINE),
    )
    qdrant.upsert(
        collection_name="item_als_vectors",
        points=[models.PointStruct(id=qdrant_load.point_id("old"), vector=[1.0] * old_dim, payload={})],
    )

    out = _publish(redis_client, qdrant, "g1")

    assert out["qdrant"]["legacy_migrated"] == 1
    assert qdrant.get_collection(collection_name="item_als_vectors__g1").config.params.vectors.size == DIM
