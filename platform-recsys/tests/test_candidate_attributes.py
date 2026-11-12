from __future__ import annotations

import copy
import importlib
import json
import logging
from datetime import datetime, timedelta, timezone

import fakeredis
import pandas as pd
import pytest
from redis.exceptions import AuthenticationError

from recsys.config import Settings, env_names, load_settings
from recsys.load import qdrant as qdrant_load
from recsys.load import redis_cache
from recsys.publish import publish_generation
from tests.fakes import FakeQdrantClient, FakeRedis

NOW = datetime(2026, 11, 6, 12, 0, 0, tzinfo=timezone.utc)
FRESH = "20261106T000000Z"
PUBLISHED = "LISTING_STATUS_PUBLISHED"
BANNED = "LISTING_STATUS_BANNED"
VEC = [1.0, 0.0, 0.0, 0.0]
BASE_PAYLOAD = {"listing_id", "model_version", "updated_at"}
ATTR_KEYS = {"seller_id", "category_id", "visible"}
COLUMNS = ["listing_id", "seller_id", "category_id", "price", "status"]
SELLER_128 = "s" * 128
SELLER_129 = "s" * 129

ROWS = [
    ["L1", "S1", "C1", 100, PUBLISHED],
    ["L2", "S2", "C2", 200, BANNED],
    ["L3", "S3", None, 300, None],
    ["L5", SELLER_129, "C5", 500, PUBLISHED],
    ["L6", SELLER_128, "C6", 600, PUBLISHED],
    ["L7", "S7", "C7", 700, "LISTING_STATUS_UNSPECIFIED"],
]
LISTINGS = ["L1", "L2", "L3", "L4", "L5", "L6", "L7"]
EXPECTED = {
    "L1": {"seller_id": "S1", "category_id": "C1", "visible": True},
    "L2": {"seller_id": "S2", "category_id": "C2", "visible": False},
    "L3": {"seller_id": "S3"},
    "L4": {},
    "L5": {"category_id": "C5", "visible": True},
    "L6": {"seller_id": SELLER_128, "category_id": "C6", "visible": True},
    "L7": {"seller_id": "S7", "category_id": "C7"},
}


@pytest.fixture
def attributes(monkeypatch):
    module = importlib.import_module("recsys.load.attributes")
    monkeypatch.setattr(module, "utcnow", lambda: NOW)
    return module


def _settings(tmp_path, **kw) -> Settings:
    values = {
        "als_rank": 4,
        "two_tower_dim": 4,
        "top_n": 10,
        "redis_host": "redis.invalid",
        "item_attributes_v2_dir": str(tmp_path / "attr2"),
        **kw,
    }
    return Settings(**values)


def _snapshot(tmp_path, rows=ROWS, stamp=FRESH, extra_columns=None):
    directory = tmp_path / "attr2"
    directory.mkdir(parents=True, exist_ok=True)
    frame = pd.DataFrame(rows, columns=COLUMNS)
    for name, value in (extra_columns or {}).items():
        frame[name] = value
    path = directory / f"as_of={stamp}.parquet"
    frame.to_parquet(path, index=False)
    return path


def _corrupt_snapshot(tmp_path, stamp=FRESH):
    directory = tmp_path / "attr2"
    directory.mkdir(parents=True, exist_ok=True)
    path = directory / f"as_of={stamp}.parquet"
    path.write_bytes(b"this is not parquet")
    return path


def _recs(listings=LISTINGS):
    users = {"u0": [(lid, 0.9 - 0.05 * i) for i, lid in enumerate(listings)]}
    items = {
        lid: [(other, 0.7 - 0.01 * j) for j, other in enumerate(listings) if other != lid] for lid in listings
    }
    popular = [(lid, 5.0 - i) for i, lid in enumerate(listings)]
    return users, items, popular


def _publish(settings, gen, redis_client, qdrant, listings=LISTINGS, two_tower=True):
    users, items, popular = _recs(listings)
    item_rows = [(lid, list(VEC)) for lid in listings]
    user_rows = [("u0", list(VEC))]
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
        two_tower_vectors={lid: list(VEC) for lid in listings} if two_tower else None,
    )


def _item_collections(settings, gen):
    return [
        qdrant_load.generation_collection(settings.qdrant_item_collection, gen),
        qdrant_load.generation_collection(settings.qdrant_two_tower_collection, gen),
    ]


def _payload(qdrant, collection, listing):
    return qdrant.collections[collection][qdrant_load.point_id(listing)]["payload"]


def _entries(redis_client, key):
    return json.loads(redis_client.get(key))


def _generation_keys(settings, gen):
    return [
        settings.gen_user_key(gen, "u0"),
        *[settings.gen_item_key(gen, lid) for lid in LISTINGS],
        settings.gen_popular_key(gen),
    ]


def _legacy_keys(settings):
    return [
        settings.user_cache_key("u0"),
        *[settings.item_cache_key(lid) for lid in LISTINGS],
        settings.popular_cache_key,
    ]


def _attrs_of(entry):
    return {k: v for k, v in entry.items() if k not in ("listing_id", "score")}


def _assert_entries_carry(redis_client, keys, expected=EXPECTED):
    for key in keys:
        entries = _entries(redis_client, key)
        assert entries, key
        for entry in entries:
            assert _attrs_of(entry) == expected[entry["listing_id"]], (key, entry)


def _core(redis_client, key):
    return [(e["listing_id"], e["score"]) for e in _entries(redis_client, key)]


def _warnings(caplog, *needles):
    return [
        r
        for r in caplog.records
        if r.levelno == logging.WARNING and any(n in r.getMessage() for n in needles)
    ]


def test_config_item_attributes_v2_defaults():
    s = load_settings({})
    assert s.item_attributes_v2_dir == "/features/item_attributes/v2"
    assert s.item_attributes_v2_path == ""
    assert s.item_attributes_dir == "/features/item_attributes/v1"


def test_config_item_attributes_v2_env_override_and_names():
    s = load_settings({"ITEM_ATTRIBUTES_V2_DIR": "/x/v2", "ITEM_ATTRIBUTES_V2_PATH": "/x/f.parquet"})
    assert s.item_attributes_v2_dir == "/x/v2"
    assert s.item_attributes_v2_path == "/x/f.parquet"
    assert {"ITEM_ATTRIBUTES_V2_DIR", "ITEM_ATTRIBUTES_V2_PATH"} <= set(env_names())


def test_config_explicit_v2_path_wins_over_the_directory(tmp_path, attributes):
    _snapshot(tmp_path, [["L1", "FROM-DIR", "C1", 1, PUBLISHED]])
    explicit = tmp_path / "elsewhere" / f"as_of={FRESH}.parquet"
    explicit.parent.mkdir()
    pd.DataFrame([["L1", "FROM-PATH", "C1", 1, PUBLISHED]], columns=COLUMNS).to_parquet(explicit, index=False)
    settings = _settings(tmp_path, item_attributes_v2_path=str(explicit))
    q = FakeQdrantClient()

    _publish(settings, "g1", FakeRedis(), q, listings=["L1"])

    assert _payload(q, _item_collections(settings, "g1")[0], "L1")["seller_id"] == "FROM-PATH"


def test_point_payload_carries_attributes(tmp_path, attributes):
    _snapshot(tmp_path)
    settings = _settings(tmp_path)
    q = FakeQdrantClient()

    summary = _publish(settings, "g1", FakeRedis(), q)

    assert summary["serving"] == "g1"
    for collection in _item_collections(settings, "g1"):
        payload = _payload(q, collection, "L1")
        assert payload["seller_id"] == "S1"
        assert payload["category_id"] == "C1"
        assert payload["visible"] is True
        assert payload["listing_id"] == "L1" and payload["model_version"] == "g1"


@pytest.mark.parametrize(
    "status",
    [
        BANNED,
        "LISTING_STATUS_DRAFT",
        "LISTING_STATUS_HELD",
        "LISTING_STATUS_REJECTED",
        "LISTING_STATUS_PENDING_REVIEW",
    ],
)
def test_non_published_status_is_not_visible(tmp_path, attributes, status):
    _snapshot(tmp_path, [["L2", "S2", "C2", 1, status]])
    settings = _settings(tmp_path)
    q = FakeQdrantClient()

    _publish(settings, "g1", FakeRedis(), q, listings=["L2"])

    for collection in _item_collections(settings, "g1"):
        assert _payload(q, collection, "L2")["visible"] is False


def test_deleted_status_is_not_visible(tmp_path, attributes):
    _snapshot(tmp_path, [["L8", "S8", "C8", 1, "LISTING_STATUS_DELETED"]])
    settings = _settings(tmp_path)
    q, r = FakeQdrantClient(), FakeRedis()

    _publish(settings, "g1", r, q, listings=["L8"])

    for collection in _item_collections(settings, "g1"):
        payload = _payload(q, collection, "L8")
        assert "visible" in payload and payload["visible"] is False
        assert payload["seller_id"] == "S8" and payload["category_id"] == "C8"


def test_unknown_attributes_are_absent_not_empty(tmp_path, attributes):
    _snapshot(tmp_path)
    settings = _settings(tmp_path)
    q = FakeQdrantClient()

    _publish(settings, "g1", FakeRedis(), q)

    for collection in _item_collections(settings, "g1"):
        payload = _payload(q, collection, "L3")
        assert payload["seller_id"] == "S3"
        assert "category_id" not in payload and "visible" not in payload
        assert set(payload) == BASE_PAYLOAD | {"seller_id"}


def test_listing_without_snapshot_row(tmp_path, attributes):
    _snapshot(tmp_path)
    settings = _settings(tmp_path)
    q = FakeQdrantClient()

    summary = _publish(settings, "g1", FakeRedis(), q)

    assert summary["serving"] == "g1"
    for collection in _item_collections(settings, "g1"):
        assert set(_payload(q, collection, "L4")) == BASE_PAYLOAD


@pytest.mark.parametrize(
    "status", [None, "LISTING_STATUS_UNSPECIFIED", "LISTING_STATUS_NOT_A_REAL_STATUS", ""]
)
def test_unknown_status_writes_no_visible(tmp_path, attributes, status):
    _snapshot(tmp_path, [["L7", "S7", "C7", 1, status]])
    settings = _settings(tmp_path)
    q, r = FakeQdrantClient(), FakeRedis()

    _publish(settings, "g1", r, q, listings=["L7"])

    for collection in _item_collections(settings, "g1"):
        payload = _payload(q, collection, "L7")
        assert "visible" not in payload
        assert payload["seller_id"] == "S7" and payload["category_id"] == "C7"
    for entry in _entries(r, settings.gen_popular_key("g1")):
        assert "visible" not in entry


def test_value_over_128_characters_is_unknown(tmp_path, attributes):
    _snapshot(tmp_path)
    settings = _settings(tmp_path)
    q = FakeQdrantClient()

    _publish(settings, "g1", FakeRedis(), q)

    for collection in _item_collections(settings, "g1"):
        payload = _payload(q, collection, "L5")
        assert "seller_id" not in payload
        assert payload["category_id"] == "C5" and payload["visible"] is True


def test_value_of_128_characters_is_kept(tmp_path, attributes):
    _snapshot(tmp_path)
    settings = _settings(tmp_path)
    q = FakeQdrantClient()

    _publish(settings, "g1", FakeRedis(), q)

    for collection in _item_collections(settings, "g1"):
        assert _payload(q, collection, "L6")["seller_id"] == SELLER_128


def test_user_points_unchanged(tmp_path, attributes):
    _snapshot(tmp_path)
    settings = _settings(tmp_path)
    q = FakeQdrantClient()

    _publish(settings, "g1", FakeRedis(), q)

    user_collection = qdrant_load.generation_collection(settings.qdrant_user_collection, "g1")
    assert q.collections[user_collection]
    for point in q.collections[user_collection].values():
        assert not ATTR_KEYS & set(point["payload"])


def test_stale_snapshot_warns_and_publishes(tmp_path, attributes, caplog):
    caplog.set_level(logging.DEBUG)
    stamp = "20261105T110000Z"
    assert NOW - datetime.strptime(stamp, "%Y%m%dT%H%M%SZ").replace(tzinfo=timezone.utc) == timedelta(
        hours=25
    )
    _snapshot(tmp_path, stamp=stamp)
    settings = _settings(tmp_path)
    q = FakeQdrantClient()

    summary = _publish(settings, "g1", FakeRedis(), q)

    assert summary["serving"] == "g1"
    assert len(_warnings(caplog, stamp, "2026-11-05T11:00:00")) == 1
    assert _payload(q, _item_collections(settings, "g1")[0], "L1")["visible"] is True


def test_snapshot_at_24_hours_does_not_warn(tmp_path, attributes, caplog):
    caplog.set_level(logging.DEBUG)
    stamp = "20261105T120000Z"
    assert NOW - datetime.strptime(stamp, "%Y%m%dT%H%M%SZ").replace(tzinfo=timezone.utc) == timedelta(
        hours=24
    )
    _snapshot(tmp_path, stamp=stamp)
    settings = _settings(tmp_path)

    _publish(settings, "g1", FakeRedis(), FakeQdrantClient())

    assert _warnings(caplog, stamp, "2026-11-05T12:00:00") == []


def test_popular_entries_carry_attributes(tmp_path, attributes):
    _snapshot(tmp_path)
    settings = _settings(tmp_path)
    r = FakeRedis()

    _publish(settings, "g1", r, FakeQdrantClient(), listings=["L1", "L2"])

    entries = _entries(r, settings.gen_popular_key("g1"))
    by_id = {e["listing_id"]: e for e in entries}
    assert [e["listing_id"] for e in entries] == ["L1", "L2"]
    assert (
        by_id["L1"]["visible"] is True
        and by_id["L1"]["seller_id"] == "S1"
        and by_id["L1"]["category_id"] == "C1"
    )
    assert (
        by_id["L2"]["visible"] is False
        and by_id["L2"]["seller_id"] == "S2"
        and by_id["L2"]["category_id"] == "C2"
    )


def test_user_item_and_legacy_entries_carry_attributes(tmp_path, attributes):
    _snapshot(tmp_path)
    settings = _settings(tmp_path, write_legacy_keys=True)
    r, q = FakeRedis(), FakeQdrantClient()

    _publish(settings, "g1", r, q)

    _assert_entries_carry(r, _generation_keys(settings, "g1"))
    _assert_entries_carry(r, _legacy_keys(settings))
    for collection in _item_collections(settings, "g1"):
        for lid in LISTINGS:
            assert {k: v for k, v in _payload(q, collection, lid).items() if k in ATTR_KEYS} == EXPECTED[lid]


def test_entry_without_attributes_is_the_old_shape(tmp_path, attributes):
    _snapshot(tmp_path)
    settings = _settings(tmp_path, write_legacy_keys=True)
    r = FakeRedis()

    _publish(settings, "g1", r, FakeQdrantClient())

    for key in _generation_keys(settings, "g1") + _legacy_keys(settings):
        for entry in _entries(r, key):
            if entry["listing_id"] == "L4":
                assert set(entry) == {"listing_id", "score"}


def test_entries_add_at_most_three_keys_and_stay_under_64kib(tmp_path, attributes):
    listings = [f"listing-{i:04d}-" + "x" * 20 for i in range(60)]
    rows = [[lid, SELLER_128, "c" * 128, 1, PUBLISHED] for lid in listings]
    _snapshot(tmp_path, rows)
    settings = _settings(tmp_path, top_n=50, write_legacy_keys=True)
    r = FakeRedis()
    users = {"u0": [(lid, 1.0 - i / 100) for i, lid in enumerate(listings)]}
    items = {lid: [(o, 0.5) for o in listings if o != lid] for lid in listings[:3]}
    popular = [(lid, 9.0 - i / 100) for i, lid in enumerate(listings)]

    publish_generation(
        settings,
        "g1",
        users,
        items,
        popular,
        [(lid, list(VEC)) for lid in listings],
        [("u0", list(VEC))],
        redis_client=r,
        qdrant_client=FakeQdrantClient(),
    )

    keys = [
        k
        for k in r.store
        if k.startswith("recs:v1:")
        and k not in ("recs:v1:serving", "recs:v1:previous", "recs:v1:model_version")
    ]
    assert keys
    for key in keys:
        raw = r.get(key)
        assert len(raw.encode("utf-8")) < 65536, key
        entries = json.loads(raw)
        assert len(entries) <= settings.top_n
        for entry in entries:
            assert len(set(entry) - {"listing_id", "score"}) <= 3
            assert set(entry) <= {"listing_id", "score"} | ATTR_KEYS


class _BatchRecordingQdrant(FakeQdrantClient):
    def __init__(self):
        super().__init__()
        self.batches: dict[str, list[int]] = {}

    def upsert(self, collection_name, points, **kw):
        self.batches.setdefault(collection_name, []).append(len(points))
        return super().upsert(collection_name, points)


def test_upsert_batches_of_256(tmp_path, attributes):
    settings = _settings(tmp_path)
    q = _BatchRecordingQdrant()
    item_rows = [(f"L{i}", list(VEC)) for i in range(600)]

    qdrant_load.load_vectors(settings, "g1", item_rows, [("u0", list(VEC))], client=q)

    collection = qdrant_load.generation_collection(settings.qdrant_item_collection, "g1")
    assert q.batches[collection] == [256, 256, 88]
    assert len(q.collections[collection]) == 600


def test_ttl_order_and_scores_unchanged(tmp_path, attributes):
    settings = _settings(tmp_path, write_legacy_keys=True, cache_ttl_seconds=3600)
    plain_dir = tmp_path / "plain"
    plain_dir.mkdir()
    plain = _settings(plain_dir, write_legacy_keys=True, cache_ttl_seconds=3600)
    _snapshot(tmp_path)
    with_attrs, without = FakeRedis(), FakeRedis()

    _publish(settings, "g1", with_attrs, FakeQdrantClient())
    _publish(plain, "g1", without, FakeQdrantClient())

    assert set(with_attrs.store) == set(without.store)
    for key in with_attrs.store:
        if key in _generation_keys(settings, "g1") + _legacy_keys(settings):
            assert _core(with_attrs, key) == _core(without, key), key
            assert with_attrs.ttl(key) == without.ttl(key) == 3600, key
        else:
            assert with_attrs.store[key] == without.store[key], key


def test_in_stock_is_never_written(tmp_path, attributes):
    _snapshot(tmp_path, extra_columns={"in_stock": True})
    settings = _settings(tmp_path, write_legacy_keys=True)
    r, q = FakeRedis(), FakeQdrantClient()

    _publish(settings, "g1", r, q)

    for collection in _item_collections(settings, "g1"):
        for point in q.collections[collection].values():
            assert "in_stock" not in point["payload"]
    for key in _generation_keys(settings, "g1") + _legacy_keys(settings):
        for entry in _entries(r, key):
            assert "in_stock" not in entry


class _RecordingQdrant(FakeQdrantClient):
    def __init__(self):
        super().__init__()
        self.writes: list[str] = []
        self.patch_sizes: list[int] = []

    def upsert(self, *args, **kw):
        self.writes.append("upsert")
        return super().upsert(*args, **kw)

    def create_collection(self, *args, **kw):
        self.writes.append("create_collection")
        return super().create_collection(*args, **kw)

    def delete(self, *args, **kw):
        self.writes.append("delete")
        return super().delete(*args, **kw)

    def delete_collection(self, *args, **kw):
        self.writes.append("delete_collection")
        return super().delete_collection(*args, **kw)

    def update_collection_aliases(self, *args, **kw):
        self.writes.append("update_collection_aliases")
        return super().update_collection_aliases(*args, **kw)

    def set_payload(self, collection_name, payload, points=None, **kw):
        self.writes.append("set_payload")
        self.patch_sizes.append(len(points))
        store = self.collections[self._resolve(collection_name)]
        for pid in points:
            store[pid]["payload"].update(payload)

    def delete_payload(self, collection_name, keys, points=None, **kw):
        self.writes.append("delete_payload")
        store = self.collections[self._resolve(collection_name)]
        for pid in points:
            for key in keys:
                store[pid]["payload"].pop(key, None)


class _ScanThenExpire(fakeredis.FakeRedis):
    victims: tuple = ()

    def scan_iter(self, *args, **kw):
        keys = list(super().scan_iter(*args, **kw))
        for victim in self.victims:
            if victim in keys:
                self.delete(victim)
        yield from keys


class _RejectingRedis(fakeredis.FakeRedis):
    def execute_command(self, *args, **kw):
        raise AuthenticationError("invalid password")


def _redis():
    return _ScanThenExpire(decode_responses=True)


def _run_backfill(monkeypatch, settings, redis_client, qdrant_client, argv=("backfill-attributes",)):
    from recsys import __main__ as entry

    monkeypatch.setattr(redis_cache, "connect", lambda s: redis_client)
    monkeypatch.setattr(qdrant_load, "_connect", lambda s: qdrant_client)
    monkeypatch.setattr(entry, "load_settings", lambda: settings)
    return entry.main(list(argv))


def _old_world(tmp_path, with_g0=False):
    settings = _settings(tmp_path, write_legacy_keys=True)
    r, q = _redis(), _RecordingQdrant()
    _publish(settings, "g1", r, q)
    _publish(settings, "g2", r, q)
    assert redis_cache.pointers(settings, r) == ("g2", "g1")
    if with_g0:
        coll = "item_als_vectors__g0"
        q.collections[coll] = {
            qdrant_load.point_id("L1"): {
                "vector": list(VEC),
                "payload": {"listing_id": "L1", "model_version": "g0"},
            }
        }
        q.dims[coll] = 4
        r.set(settings.gen_popular_key("g0"), json.dumps([{"listing_id": "L1", "score": 1.0}]), ex=3600)
    for key in r.keys("recs:v1:*"):
        if key not in (settings.serving_key, settings.previous_key):
            r.pexpire(key, 3_600_000)
    q.writes.clear()
    q.patch_sizes.clear()
    return settings, r, q


def _redis_dump(r):
    return {k: (r.get(k), r.pttl(k) == -1) for k in r.keys("*")}


def _vectors(q):
    return {c: {pid: p["vector"] for pid, p in pts.items()} for c, pts in q.collections.items()}


def _assert_patched(settings, r, q, gens):
    for gen in gens:
        for collection in _item_collections(settings, gen):
            for lid in LISTINGS:
                got = {k: v for k, v in _payload(q, collection, lid).items() if k in ATTR_KEYS}
                assert got == EXPECTED[lid], (collection, lid)
        _assert_entries_carry(r, _generation_keys(settings, gen))
    _assert_entries_carry(r, _legacy_keys(settings))


def test_backfill_patches_serving_generation(tmp_path, attributes, monkeypatch, capsys):
    settings, r, q = _old_world(tmp_path)
    _snapshot(tmp_path)
    pointers_before = {
        k: r.get(k) for k in (settings.serving_key, settings.previous_key, "recs:v1:model_version")
    }
    vectors_before = _vectors(q)
    core_before = {k: _core(r, k) for k in _generation_keys(settings, "g2") + _legacy_keys(settings)}

    code = _run_backfill(monkeypatch, settings, r, q)

    assert code == 0
    _assert_patched(settings, r, q, ["g1", "g2"])
    assert {k: r.get(k) for k in pointers_before} == pointers_before
    assert _vectors(q) == vectors_before
    assert {k: _core(r, k) for k in core_before} == core_before
    assert set(q.writes) == {"set_payload"}
    assert q.patch_sizes and max(q.patch_sizes) <= 256
    user_collection = qdrant_load.generation_collection(settings.qdrant_user_collection, "g2")
    for point in q.collections[user_collection].values():
        assert not ATTR_KEYS & set(point["payload"])


def test_backfill_deleted_status_writes_visible_false(tmp_path, attributes, monkeypatch):
    settings, r, q = _old_world(tmp_path)
    rows = [row[:] for row in ROWS]
    rows[0][4] = "LISTING_STATUS_DELETED"
    _snapshot(tmp_path, rows)
    expected = {**EXPECTED, "L1": {"seller_id": "S1", "category_id": "C1", "visible": False}}

    assert _run_backfill(monkeypatch, settings, r, q) == 0

    for gen in ("g1", "g2"):
        for collection in _item_collections(settings, gen):
            payload = _payload(q, collection, "L1")
            assert "visible" in payload and payload["visible"] is False, (collection, gen)
        _assert_entries_carry(r, _generation_keys(settings, gen), expected)
    _assert_entries_carry(r, _legacy_keys(settings), expected)


def test_backfill_is_idempotent_and_keeps_ttl(tmp_path, attributes, monkeypatch):
    settings, r, q = _old_world(tmp_path)
    _snapshot(tmp_path)
    ttl_keys = [k for k in r.keys("recs:v1:*") if r.pttl(k) > 0]
    before = {k: r.pttl(k) for k in ttl_keys}
    assert ttl_keys

    assert _run_backfill(monkeypatch, settings, r, q) == 0
    first = _redis_dump(r)
    first_payloads = copy.deepcopy(q.collections)
    after_first = {k: r.pttl(k) for k in ttl_keys}
    assert _run_backfill(monkeypatch, settings, r, q) == 0

    assert _redis_dump(r) == first
    assert q.collections == first_payloads
    for key in ttl_keys:
        assert 0 < r.pttl(key) <= after_first[key] <= before[key], key


@pytest.mark.parametrize(
    "second_rows",
    [
        [["L1", "S1", "C1", 100, "LISTING_STATUS_UNSPECIFIED"]],
        [["L2", "S2", "C2", 200, BANNED]],
    ],
    ids=["unknown-status", "absent-from-snapshot"],
)
def test_backfill_removes_stale_attributes_in_qdrant(tmp_path, attributes, monkeypatch, second_rows):
    settings, r, q = _old_world(tmp_path)
    _snapshot(tmp_path, stamp="20261106T000000Z")
    assert _run_backfill(monkeypatch, settings, r, q) == 0
    for collection in _item_collections(settings, "g2"):
        assert _payload(q, collection, "L1")["visible"] is True

    _snapshot(tmp_path, second_rows, stamp="20261106T060000Z")
    assert _run_backfill(monkeypatch, settings, r, q) == 0

    expected = {}
    for row in second_rows:
        expected[row[0]] = {
            k: v
            for k, v in (
                ("seller_id", row[1]),
                ("category_id", row[2]),
                ("visible", True if row[4] == PUBLISHED else False if row[4] == BANNED else None),
            )
            if v is not None
        }
    for lid in LISTINGS:
        expected.setdefault(lid, {})
    for gen in ("g1", "g2"):
        for collection in _item_collections(settings, gen):
            for lid in LISTINGS:
                got = {k: v for k, v in _payload(q, collection, lid).items() if k in ATTR_KEYS}
                assert got == expected[lid], (collection, lid)
        _assert_entries_carry(r, _generation_keys(settings, gen), expected)
    _assert_entries_carry(r, _legacy_keys(settings), expected)


def test_backfill_without_snapshot_exits_2(tmp_path, attributes, monkeypatch):
    settings, r, q = _old_world(tmp_path)
    redis_before, qdrant_before = _redis_dump(r), copy.deepcopy(q.collections)

    code = _run_backfill(monkeypatch, settings, r, q)

    assert code == 2
    assert q.writes == []
    assert _redis_dump(r) == redis_before
    assert q.collections == qdrant_before


def test_backfill_legacy_keys_and_ttl_edges(tmp_path, attributes, monkeypatch):
    settings, r, q = _old_world(tmp_path)
    _snapshot(tmp_path)
    no_ttl = settings.item_cache_key("L1")
    expiring = settings.item_cache_key("L2")
    r.persist(no_ttl)
    r.victims = (expiring,)

    code = _run_backfill(monkeypatch, settings, r, q)

    assert code == 0
    assert r.pttl(no_ttl) == -1
    assert r.exists(expiring) == 0
    patched = [k for k in _legacy_keys(settings) if k != expiring]
    _assert_entries_carry(r, patched)
    for key in patched:
        if key != no_ttl:
            assert r.pttl(key) > 0


def test_backfill_corrupt_snapshot_exits_2(tmp_path, attributes, monkeypatch):
    settings, r, q = _old_world(tmp_path)
    _corrupt_snapshot(tmp_path)
    redis_before, qdrant_before = _redis_dump(r), copy.deepcopy(q.collections)

    code = _run_backfill(monkeypatch, settings, r, q)

    assert code == 2
    assert q.writes == []
    assert _redis_dump(r) == redis_before
    assert q.collections == qdrant_before


def test_backfill_without_credentials_writes_nothing(tmp_path, attributes, monkeypatch):
    settings, _, q = _old_world(tmp_path)
    _snapshot(tmp_path)
    qdrant_before = copy.deepcopy(q.collections)

    code = _run_backfill(monkeypatch, settings, _RejectingRedis(decode_responses=True), q)

    assert code not in (0, None)
    assert q.writes == []
    assert q.collections == qdrant_before


def test_backfill_touches_only_serving_and_previous(tmp_path, attributes, monkeypatch):
    settings, r, q = _old_world(tmp_path, with_g0=True)
    _snapshot(tmp_path)
    g0_collection_before = copy.deepcopy(q.collections["item_als_vectors__g0"])
    g0_key = settings.gen_popular_key("g0")
    g0_key_before = (r.get(g0_key), r.pttl(g0_key))

    code = _run_backfill(monkeypatch, settings, r, q)

    assert code == 0
    _assert_patched(settings, r, q, ["g1", "g2"])
    assert q.collections["item_als_vectors__g0"] == g0_collection_before
    assert r.get(g0_key) == g0_key_before[0]
    assert 0 < r.pttl(g0_key) <= g0_key_before[1]


def test_backfill_without_serving_generation(tmp_path, attributes, monkeypatch):
    settings = _settings(tmp_path)
    _snapshot(tmp_path)
    r, q = _redis(), _RecordingQdrant()

    code = _run_backfill(monkeypatch, settings, r, q)

    assert code == 0
    assert q.writes == []
    assert r.keys("*") == []
    assert q.collections == {}


def test_logs_carry_no_ids(tmp_path, attributes, monkeypatch, caplog, capsys):
    caplog.set_level(logging.DEBUG)
    listings = ["lst-secret-1", "lst-secret-2"]
    rows = [
        ["lst-secret-1", "seller-secret-1", "cat-secret-1", 1, PUBLISHED],
        ["lst-secret-2", "seller-secret-2", "cat-secret-2", 1, BANNED],
    ]
    settings = _settings(tmp_path, write_legacy_keys=True)
    r, q = _redis(), _RecordingQdrant()

    _publish(settings, "g1", r, q, listings=listings)
    _snapshot(tmp_path, rows)
    assert _run_backfill(monkeypatch, settings, r, q) == 0
    _publish(settings, "g2", r, q, listings=listings)

    out = capsys.readouterr()
    text = caplog.text + out.out + out.err
    for secret in ("secret-1", "secret-2"):
        assert secret not in text


def _training_world(tmp_path, reg, stamp=None, corrupt=False):
    from tests.dataset_fixture import write_dataset
    from tests.pipeline_fixture import als_rows, settings

    write_dataset(tmp_path / "ds", als_rows())
    if corrupt:
        _corrupt_snapshot(tmp_path)
    elif stamp:
        _snapshot(tmp_path, stamp=stamp)
    return settings(tmp_path / "ds", "g1", reg, item_attributes_v2_dir=str(tmp_path / "attr2"))


def _mlflow_runs(reg):
    ids = [e.experiment_id for e in reg.client.search_experiments()]
    return reg.client.search_runs(ids) if ids else []


def _assert_refused(tmp_path, reg, monkeypatch, caplog, corrupt):
    from tests.pipeline_fixture import forbid_spark, registered_models, run_main

    caplog.set_level(logging.DEBUG)
    cfg = _training_world(tmp_path, reg, corrupt=corrupt)
    forbid_spark(monkeypatch)
    r, q = FakeRedis(), FakeQdrantClient()
    r.store["recs:v1:serving"] = "g0"
    before = dict(r.store)

    assert run_main(monkeypatch, cfg, r, q) == 2

    assert "attributes=missing" in caplog.text
    assert registered_models(reg) == set() and _mlflow_runs(reg) == []
    assert q.collections == {} and q.aliases == {}
    assert r.store == before and r.store["recs:v1:serving"] == "g0"


def test_missing_snapshot_refuses_before_training(tmp_path, reg, monkeypatch, caplog):
    _assert_refused(tmp_path, reg, monkeypatch, caplog, corrupt=False)


def test_corrupt_snapshot_refuses_before_training(tmp_path, reg, monkeypatch, caplog):
    _assert_refused(tmp_path, reg, monkeypatch, caplog, corrupt=True)


def test_stale_snapshot_still_trains(tmp_path, reg, monkeypatch, caplog):
    from tests.pipeline_fixture import requires_spark, run_main

    if requires_spark.args[0]:
        pytest.skip("Spark needs a Java runtime")
    caplog.set_level(logging.DEBUG)
    as_of = datetime.now(timezone.utc) - timedelta(hours=30)
    stamp = as_of.strftime("%Y%m%dT%H%M%SZ")
    cfg = _training_world(tmp_path, reg, stamp=stamp)
    r, q = FakeRedis(), FakeQdrantClient()

    assert run_main(monkeypatch, cfg, r, q) == 0

    assert r.store["recs:v1:serving"] == "g1"
    assert q.collections
    assert len(_warnings(caplog, stamp, as_of.strftime("%Y-%m-%dT%H:%M:%S"))) == 1
