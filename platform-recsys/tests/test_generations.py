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
from tests.fakes import FakeQdrantClient, FakeRedis

ALS = "recsys-als"
GBDT = "recsys-gbdt"
TWO_TOWER = "recsys-two-tower"

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


def _versions_of(reg, model, *generations, artifactless=()):
    """``model`` versions, one per generation, built in order: the last is ``champion`` and the one before
    it ``previous``. ``lineage.model_version`` names the generation. A generation in ``artifactless`` is a
    metrics-only version made by ``seed`` (only the first one can be: ``seed`` needs an empty model).

    INTERFACE ASSUMPTION: rollback finds the version a generation was built from through the lineage tag
    ``lineage.model_version`` (the pipeline writes it; see tests/pipeline_fixture.py).
    """
    ids = {}
    for g in generations:
        if g in artifactless:
            assert g == generations[0]
            from modelregistry import seed

            ids[g] = seed(model, {"ndcg@10": 0.1, "coverage@10": 0.5}, {"model_version": g}, "test").version
            continue
        ids[g] = reg.add_version(model, {"ndcg@10": 0.1, "coverage@10": 0.5}, alias=None)
        reg.client.set_model_version_tag(model, str(ids[g]), "lineage.model_version", g)
    reg.client.set_registered_model_alias(model, "champion", str(ids[generations[-1]]))
    if len(generations) > 1:
        reg.client.set_registered_model_alias(model, "previous", str(ids[generations[-2]]))
    return ids


def _registry_with(reg, *generations, artifactless=()):
    return _versions_of(reg, ALS, *generations, artifactless=artifactless)


def _with_ranker(redis_client, *generations):
    for g in generations:
        redis_client.set(SETTINGS.gen_ranker_key(g), "{}")


def _with_two_tower(qdrant, *generations):
    for g in generations:
        qdrant_load.load_two_tower_vectors(
            SETTINGS, g, {f"{g}-l0": [0.5] * SETTINGS.two_tower_dim}, client=qdrant
        )


def _snapshot(reg, redis_client, qdrant, *models):
    return (
        {m: reg.aliases(m) for m in models},
        {k: redis_client.get(k) for k in ("recs:v1:serving", "recs:v1:previous", "recs:v1:model_version")},
        _aliases(qdrant),
    )


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
    assert redis_cache.swap_generations(SETTINGS, "g2", client=r) == ("g2", "g1")
    assert (r.get("recs:v1:serving"), r.get("recs:v1:previous"), r.get("recs:v1:model_version")) == (
        "g1",
        "g2",
        "g1",
    )


# ── rollback ─────────────────────────────────────────────────────────────────────────────────────


def test_rollback_moves_registry_champion(reg, redis_client, qdrant):
    # RAT-31: serving g2 / previous g1 -> serving g1 / previous g2, and recsys-als@champion names g1's version
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    ids = _registry_with(reg, "g1", "g2")
    assert reg.aliases(ALS)["champion"] == ids["g2"]

    out = rollback(SETTINGS, redis_client=redis_client, qdrant_client=qdrant)

    assert out == {"serving": "g1", "previous": "g2"}
    assert redis_client.get("recs:v1:serving") == "g1"
    assert redis_client.get("recs:v1:previous") == "g2"
    assert redis_client.get("recs:v1:model_version") == "g1"
    assert _aliases(qdrant) == {
        "item_als_vectors": "item_als_vectors__g1",
        "user_als_vectors": "user_als_vectors__g1",
    }
    assert reg.aliases(ALS) == {"champion": ids["g1"], "previous": ids["g2"]}
    assert reg.tags(ALS, ids["g1"])["lineage.model_version"] == "g1"


def test_rollback_twice_goes_forward_again(reg, redis_client, qdrant):
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    ids = _registry_with(reg, "g1", "g2")
    rollback(SETTINGS, redis_client=redis_client, qdrant_client=qdrant)
    rollback(SETTINGS, redis_client=redis_client, qdrant_client=qdrant)

    assert redis_client.get("recs:v1:serving") == "g2"
    assert reg.aliases(ALS) == {"champion": ids["g2"], "previous": ids["g1"]}  # a swap: twice is a round trip


def test_rollback_without_previous_refused(reg, redis_client, qdrant, monkeypatch):
    # RAT-32: no previous generation -> refused (exit 2 at the command), no alias and no pointer changes
    _publish(redis_client, qdrant, "g1")
    ids = _registry_with(reg, "g1")
    before_aliases = _aliases(qdrant)
    registry_before = reg.aliases(ALS)

    with pytest.raises(RollbackRefused, match="no previous generation"):
        rollback(SETTINGS, redis_client=redis_client, qdrant_client=qdrant)

    assert redis_client.get("recs:v1:serving") == "g1"
    assert redis_client.get("recs:v1:model_version") == "g1"
    assert _aliases(qdrant) == before_aliases
    assert reg.aliases(ALS) == registry_before == {"champion": ids["g1"]}
    assert _main_with(monkeypatch, redis_client, qdrant, ["rollback"]) == 2
    assert redis_client.get("recs:v1:serving") == "g1"
    assert reg.aliases(ALS) == registry_before


def test_rollback_is_refused_when_the_previous_keys_expired(reg, redis_client, qdrant):
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    ids = _registry_with(reg, "g1", "g2")
    redis_client.delete(*_gen_keys(redis_client, "g1"))

    with pytest.raises(RollbackRefused, match="no keys left"):
        rollback(SETTINGS, redis_client=redis_client, qdrant_client=qdrant)
    assert redis_client.get("recs:v1:serving") == "g2"
    assert _aliases(qdrant)["item_als_vectors"] == "item_als_vectors__g2"
    assert reg.aliases(ALS)["champion"] == ids["g2"]


def test_rollback_is_refused_when_the_previous_collection_is_gone(reg, redis_client, qdrant):
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    ids = _registry_with(reg, "g1", "g2")
    qdrant.delete_collection(collection_name="user_als_vectors__g1")

    with pytest.raises(RollbackRefused, match="no Qdrant collections"):
        rollback(SETTINGS, redis_client=redis_client, qdrant_client=qdrant)
    assert redis_client.get("recs:v1:serving") == "g2"
    assert reg.aliases(ALS)["champion"] == ids["g2"]


def test_rollback_to_artifactless_refused(reg, redis_client, qdrant, monkeypatch):
    # RAT-54: the previous version of recsys-als is the metrics-only v1 made by seed -> exit 2, nothing moves
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    _registry_with(reg, "g1", "g2", artifactless=("g1",))
    before = _snapshot(reg, redis_client, qdrant, ALS)

    assert _main_with(monkeypatch, redis_client, qdrant, ["rollback"]) == 2

    assert _snapshot(reg, redis_client, qdrant, ALS) == before
    assert redis_client.get("recs:v1:serving") == "g2"


def test_partial_rollback_refused_before_any_change(reg, redis_client, qdrant, monkeypatch):
    # RAT-55: both generations carry a GBDT ranker but recsys-gbdt has no previous -> exit 2 before any
    # alias moves, recsys-als is not swapped
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    _with_ranker(redis_client, "g1", "g2")
    _registry_with(reg, "g1", "g2")
    _versions_of(reg, GBDT, "g2")
    before = _snapshot(reg, redis_client, qdrant, ALS, GBDT)

    assert _main_with(monkeypatch, redis_client, qdrant, ["rollback"]) == 2

    assert _snapshot(reg, redis_client, qdrant, ALS, GBDT) == before
    assert set(reg.aliases(GBDT)) == {"champion"}


def test_rollback_refused_when_serving_ahead_of_registry(reg, redis_client, qdrant, monkeypatch):
    # RAT-60: serving names g2 but recsys-als@champion still names g1's version (e.g. after RAT-46) -> exit 2
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    ids = _registry_with(reg, "g1", "g2")
    reg.client.set_registered_model_alias(ALS, "champion", str(ids["g1"]))
    reg.client.set_registered_model_alias(ALS, "previous", str(ids["g1"]))
    before = _snapshot(reg, redis_client, qdrant, ALS)

    assert _main_with(monkeypatch, redis_client, qdrant, ["rollback"]) == 2

    assert _snapshot(reg, redis_client, qdrant, ALS) == before
    assert redis_client.get("recs:v1:serving") == "g2"


def test_rollback_refused_when_a_carried_gbdt_is_out_of_step(reg, redis_client, qdrant, monkeypatch):
    # RAT-60 (second model): ALS is in step but recsys-gbdt@champion is not the version that built g2
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    _with_ranker(redis_client, "g1", "g2")
    _registry_with(reg, "g1", "g2")
    ids = _versions_of(reg, GBDT, "g1", "g2")
    reg.client.set_registered_model_alias(GBDT, "champion", str(ids["g1"]))
    reg.client.set_registered_model_alias(GBDT, "previous", str(ids["g2"]))
    before = _snapshot(reg, redis_client, qdrant, ALS, GBDT)

    assert _main_with(monkeypatch, redis_client, qdrant, ["rollback"]) == 2

    assert _snapshot(reg, redis_client, qdrant, ALS, GBDT) == before


def test_rollback_swaps_only_models_in_both_generations(reg, redis_client, qdrant, monkeypatch, caplog):
    # RAT-61: serving g2 has a GBDT ranker, previous g1 has none -> ALS swapped, GBDT untouched and the
    # omission logged, pointer swapped, exit 0
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    _with_ranker(redis_client, "g2")
    ids = _registry_with(reg, "g1", "g2")
    gbdt_ids = _versions_of(reg, GBDT, "g2")
    gbdt_before = reg.aliases(GBDT)

    with caplog.at_level("INFO"):
        assert _main_with(monkeypatch, redis_client, qdrant, ["rollback"]) == 0

    assert reg.aliases(ALS) == {"champion": ids["g1"], "previous": ids["g2"]}
    assert reg.aliases(GBDT) == gbdt_before == {"champion": gbdt_ids["g2"]}
    assert redis_client.get("recs:v1:serving") == "g1"
    assert redis_client.get("recs:v1:previous") == "g2"
    assert "recsys-gbdt" in caplog.text


def test_rollback_swaps_every_model_both_generations_carry(reg, redis_client, qdrant):
    # RAT-31 / RAT-61 (all three carried): ALS, GBDT (ranker key) and two-tower (collection) are swapped
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    _with_ranker(redis_client, "g1", "g2")
    _with_two_tower(qdrant, "g1", "g2")
    als = _registry_with(reg, "g1", "g2")
    gbdt = _versions_of(reg, GBDT, "g1", "g2")
    tt = _versions_of(reg, TWO_TOWER, "g1", "g2")

    out = rollback(SETTINGS, redis_client=redis_client, qdrant_client=qdrant)

    assert out == {"serving": "g1", "previous": "g2"}
    assert reg.aliases(ALS) == {"champion": als["g1"], "previous": als["g2"]}
    assert reg.aliases(GBDT) == {"champion": gbdt["g1"], "previous": gbdt["g2"]}
    assert reg.aliases(TWO_TOWER) == {"champion": tt["g1"], "previous": tt["g2"]}
    assert redis_client.get("recs:v1:serving") == "g1"


def test_registry_down_mid_rollback_restores(reg, redis_client, qdrant, monkeypatch):
    # RAT-56: MLflow goes away after recsys-als was swapped and before recsys-gbdt is -> ALS swapped back,
    # serving unchanged, exit 1
    import requests
    from mlflow import MlflowClient

    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    _with_ranker(redis_client, "g1", "g2")
    _registry_with(reg, "g1", "g2")
    _versions_of(reg, GBDT, "g1", "g2")
    before = _snapshot(reg, redis_client, qdrant, ALS, GBDT)

    real = MlflowClient.set_registered_model_alias
    als_writes = []

    def flaky(self, name, alias, version):
        if name == GBDT:
            raise requests.ConnectionError("connection refused")
        als_writes.append((alias, version))
        return real(self, name, alias, version)

    monkeypatch.setattr(MlflowClient, "set_registered_model_alias", flaky)

    assert _main_with(monkeypatch, redis_client, qdrant, ["rollback"]) == 1

    monkeypatch.setattr(MlflowClient, "set_registered_model_alias", real)
    assert len(als_writes) >= 2  # recsys-als really was swapped before the failure (and then swapped back)
    assert _snapshot(reg, redis_client, qdrant, ALS, GBDT) == before
    assert redis_client.get("recs:v1:serving") == "g2"


def test_rollback_registry_down_exit_1(redis_client, qdrant, monkeypatch):
    # RAT-57: MLflow is unreachable when rollback starts -> exit 1 within 20 s, nothing changes
    import socket
    import time

    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    monkeypatch.setenv("MLFLOW_TRACKING_URI", f"http://127.0.0.1:{port}")
    keys = ("recs:v1:serving", "recs:v1:previous", "recs:v1:model_version")
    before = ({k: redis_client.get(k) for k in keys}, _aliases(qdrant))

    t0 = time.monotonic()
    code = _main_with(monkeypatch, redis_client, qdrant, ["rollback"])

    assert code == 1
    assert time.monotonic() - t0 < 20
    assert ({k: redis_client.get(k) for k in keys}, _aliases(qdrant)) == before


# ── command ──────────────────────────────────────────────────────────────────────────────────────


def _main_with(monkeypatch, redis_client, qdrant, argv):
    from recsys import __main__ as entry
    from recsys.load import qdrant as qdrant_mod

    monkeypatch.setattr(redis_cache, "connect", lambda settings: redis_client)
    monkeypatch.setattr(qdrant_mod, "_connect", lambda settings: qdrant)
    monkeypatch.setattr(entry, "load_settings", lambda: SETTINGS)
    return entry.main(argv)


def test_command_rollback_exit_codes(reg, monkeypatch):
    redis_client, qdrant = FakeRedis(), FakeQdrantClient()
    _publish(redis_client, qdrant, "g1")
    _registry_with(reg, "g1")
    assert _main_with(monkeypatch, redis_client, qdrant, ["rollback"]) == 2
    assert redis_client.store["recs:v1:serving"] == "g1"

    _publish(redis_client, qdrant, "g2")
    _registry_with(reg, "g1", "g2")  # the registry is in step with serving: g2 built the champion
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


def test_swap_is_a_compare_and_set_on_serving(redis_client):
    redis_cache.activate_generation(SETTINGS, "g1", client=redis_client)
    redis_cache.activate_generation(SETTINGS, "g2", client=redis_client)

    assert redis_cache.swap_generations(SETTINGS, "g1", client=redis_client) == redis_cache.SWAP_STALE
    assert redis_cache.pointers(SETTINGS, redis_client) == ("g2", "g1")
    assert redis_cache.swap_generations(SETTINGS, "g2", client=redis_client) == ("g2", "g1")


class _CountingQdrant(FakeQdrantClient):
    def __init__(self):
        super().__init__()
        self.alias_calls: list[list] = []

    def update_collection_aliases(self, change_aliases_operations):
        self.alias_calls.append(list(change_aliases_operations))
        super().update_collection_aliases(change_aliases_operations)


def test_item_and_user_aliases_move_in_one_call():
    redis_client, qdrant = FakeRedis(), _CountingQdrant()
    _publish(redis_client, qdrant, "g1")
    qdrant.alias_calls.clear()

    _publish(redis_client, qdrant, "g2")

    assert len(qdrant.alias_calls) == 1
    ops = qdrant.alias_calls[0]
    assert sum(getattr(o, "delete_alias", None) is not None for o in ops) == 2
    assert sum(getattr(o, "create_alias", None) is not None for o in ops) == 2
    assert _aliases(qdrant) == {
        "item_als_vectors": "item_als_vectors__g2",
        "user_als_vectors": "user_als_vectors__g2",
    }


# ── serving-switch-atomicity: retention is pointer-safe ──────────────────────────────────────────


def _drop_aliases(qdrant):
    models = qdrant_load._get_models()
    qdrant.update_collection_aliases(
        change_aliases_operations=[
            models.DeleteAliasOperation(delete_alias=models.DeleteAlias(alias_name=name))
            for name in _aliases(qdrant)
        ]
    )


def test_retention_with_no_named_generation_deletes_nothing(redis_client, qdrant):
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    _drop_aliases(qdrant)  # nothing but the pointers (and now not even those) protects a generation
    before_collections = _collections(qdrant)
    before_keys = set(redis_client.scan_iter(match="recs:v1:gen:*"))

    assert qdrant_load.prune_generations(SETTINGS, set(), client=qdrant) == []
    assert redis_cache.prune_generations(SETTINGS, set(), client=redis_client) == 0

    assert _collections(qdrant) == before_collections
    assert set(redis_client.scan_iter(match="recs:v1:gen:*")) == before_keys


def test_publish_spares_serving_and_previous_collections_without_aliases(redis_client, qdrant):
    _publish(redis_client, qdrant, "g1")
    _publish(redis_client, qdrant, "g2")
    _drop_aliases(qdrant)
    _publish(redis_client, qdrant, "g3")

    names = _collections(qdrant)
    assert {"item_als_vectors__g3", "item_als_vectors__g2"} <= names
    assert not {n for n in names if n.endswith("__g1")}
    assert redis_client.get("recs:v1:serving") == "g3"
    assert redis_client.get("recs:v1:previous") == "g2"
