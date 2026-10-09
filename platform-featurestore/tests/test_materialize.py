import dataclasses
import json
from datetime import datetime, timezone

import pyarrow.parquet as pq
import pytest
from conftest import AS_OF, d, h, manifest_of, write_inputs

from featurestore import cli, job, registry
from featurestore.settings import ConfigError, Settings, parse_as_of


def ev(i, typ, user, listing, at, **kw):
    return {"id": i, "type": typ, "user": user, "listing": listing, "at": at, **kw}


def fact(i, fact_, user, at, listing=None, seller=None, rating=None, **kw):
    return {
        "id": i,
        "fact": fact_,
        "user": user,
        "listing": listing,
        "seller": seller,
        "rating": rating,
        "at": at,
        **kw,
    }


def run(settings, redis):
    views = registry.load_registry(settings.registry_dir)
    return job.materialize(settings, views, redis)["_rows"]


def rows_by_id(rows):
    return {r["entity_id"]: r for r in rows}


def test_user_activity_features_and_windows(dirs, settings, redis):
    events = [
        ev("e1", "view", "u1", "L1", h(1)),
        ev("e2", "view", "u1", "L1", h(2)),
        ev("e3", "view", "u1", "L2", d(6, 23)),  # inside 7d
        ev("e4", "view", "u1", "L2", d(7, 1)),  # outside 7d
        ev("e5", "click", "u1", "L1", h(3)),
        ev("e6", "add_to_cart", "u1", "L1", h(3)),
        ev("e7", "view", "anon:z", "L1", h(1), ptype="ANONYMOUS"),
    ]
    facts = [
        fact("f1", "favorite_added", "u1", h(5), listing="L1", seller="S1"),
        fact("f2", "favorite_added", "u1", h(6), listing="L2", seller="S1"),
        fact("f3", "favorite_removed", "u1", h(4), listing="L2", seller="S1"),
        fact("f4", "seller_followed", "u1", h(5), seller="S1"),
        fact("f5", "seller_followed", "u1", h(5), seller="S2"),
        fact("f6", "seller_unfollowed", "u1", h(1), seller="S2"),
    ]
    write_inputs(dirs[0], events, facts)
    rows = rows_by_id(run(settings, redis)["user_activity@v1"])
    assert rows["u1"] == {
        "entity_id": "u1",
        "views_7d": 3,
        "clicks_7d": 1,
        "add_to_cart_7d": 1,
        "favorites_current": 1,
        "follows_current": 1,
    }
    assert rows["anon:z"]["views_7d"] == 1 and rows["anon:z"]["favorites_current"] == 0
    assert "paid_orders_30d" not in rows["u1"]  # order_facts has no buyer column


def test_item_popularity_features(dirs, settings, redis):
    events = [ev(f"v{i}", "view", f"u{i}", "L1", h(i + 1)) for i in range(3)]
    events += [
        ev("c1", "click", "u1", "L1", h(1)),
        ev("i1", "impression", "u1", "L1", h(1)),
        ev("i2", "impression", "u2", "L1", h(1)),
        ev("i3", "impression", "u2", "L1", h(1)),
        ev("i4", "impression", "u2", "L1", h(1)),
        ev("x", "view", "u1", "L9", d(8)),
        ev("c2", "click", "u1", "L3", h(1)),  # clicks, no impressions -> ctr 0
    ]
    facts = [
        fact("f1", "favorite_added", "u1", h(2), listing="L1"),
        fact("f2", "favorite_added", "u2", h(2), listing="L1"),
        fact("f3", "favorite_removed", "u2", h(1), listing="L1"),
        fact("r1", "review_created", "u1", h(2), listing="L1", rating=5),
        fact("r2", "review_created", "u2", d(20), listing="L1", rating=2),  # reviews are not windowed
    ]
    write_inputs(dirs[0], events, facts)
    rows = rows_by_id(run(settings, redis)["item_popularity@v1"])
    assert rows["L1"] == {
        "entity_id": "L1",
        "views_7d": 3,
        "clicks_7d": 1,
        "add_to_cart_7d": 0,
        "favorites_current": 1,
        "review_count": 2,
        "avg_rating": 3.5,
        "ctr_7d": 0.25,
    }
    assert rows["L3"]["ctr_7d"] == 0.0 and rows["L3"]["avg_rating"] is None
    assert "L9" not in rows  # seen only outside the window


def test_as_of_excludes_later_rows(dirs, settings, redis):
    events = [
        ev("e1", "view", "early", "L1", h(1)),
        ev("e2", "view", "late-ingest", "L1", h(1), ing=h(-1)),  # ingested after AS_OF
        ev("e3", "view", "late-event", "L1", h(-1), ing=h(1)),  # occurred after AS_OF
    ]
    facts = [fact("f1", "favorite_added", "late-fav", h(1), listing="L1", ing=h(-2))]
    write_inputs(dirs[0], events, facts)
    assert set(rows_by_id(run(settings, redis)["user_activity@v1"])) == {"early"}


def test_snapshot_manifest_and_online(dirs, settings, redis):
    write_inputs(dirs[0], [ev("e1", "view", "u1", "L1", h(1))], [])
    views = registry.load_registry(settings.registry_dir)
    job.materialize(settings, views, redis, now=datetime(2026, 10, 9, 12, 5, 0))
    out = dirs[1]
    snap = out / "user_activity/v1/as_of=20261009T120000Z.parquet"
    snap_row = pq.read_table(snap).to_pylist()[0]
    assert snap_row["user_key"] == "u1" and snap_row["views_7d"] == 1 and "entity_id" not in snap_row
    item = pq.read_table(out / "item_popularity/v1/as_of=20261009T120000Z.parquet").to_pylist()[0]
    assert item["listing_id"] == "L1"
    m = manifest_of(out, "20261009T120000Z")
    assert m["as_of"] == "2026-10-09T12:00:00Z" and m["input_watermark"] == "2026-10-09T11:00:00Z"
    by = {v["name"]: v for v in m["views"]}
    assert by["user_activity"]["rows"] == 1 and len(by["user_activity"]["definition_sha256"]) == 64
    assert "order_facts.parquet" in {i["name"] for i in m["inputs"]}
    assert json.loads(redis.get("fs:user_activity:v1:u1"))["views_7d"] == 1
    assert redis.get("fs:user_activity:current") == "1"
    meta = json.loads(redis.get("fs:user_activity:meta"))
    assert meta["as_of"] == m["as_of"] and meta["materialized_at"] == "2026-10-09T12:05:00Z"
    assert meta["input_watermark"] <= meta["as_of"]
    assert 0 < redis.ttl("fs:user_activity:v1:u1") <= 172800


def test_two_runs_keep_both_snapshots(dirs, env, redis):
    write_inputs(dirs[0], [ev("e1", "view", "u1", "L1", h(1))], [])
    for a in ("2026-10-09T12:00:00Z", "2026-10-09T13:00:00Z"):
        assert cli.main(["materialize"], {**env, "AS_OF": a}, redis) == 0
    for view in ("user_activity", "item_popularity"):
        assert len(list((dirs[1] / view / "v1").glob("as_of=*.parquet"))) == 2
    assert manifest_of(dirs[1], "20261009T130000Z")["as_of"] == "2026-10-09T13:00:00Z"


def test_parity_ok_then_tampered_exits_3(dirs, env, redis, capsys):
    write_inputs(dirs[0], [ev("e1", "view", "buyer", "L1", h(1))], [])
    assert cli.main(["materialize"], env, redis) == 0
    assert cli.main(["parity"], env, redis) == 0
    key = "fs:user_activity:v1:buyer"
    row = json.loads(redis.get(key))
    row["views_7d"] = 99
    redis.set(key, json.dumps(row))
    capsys.readouterr()
    assert cli.main(["parity"], env, redis) == 3
    out = capsys.readouterr().out
    assert "parity mismatch view=user_activity entity=buyer feature=views_7d online=99 offline=1" in out


def test_missing_online_row_is_a_mismatch(dirs, env, redis):
    write_inputs(dirs[0], [ev("e1", "view", "buyer", "L1", h(1))], [])
    assert cli.main(["materialize"], env, redis) == 0
    redis.delete("fs:user_activity:v1:buyer")
    assert cli.main(["parity"], env, redis) == 3


def test_missing_input_exits_2(env, redis):
    assert cli.main(["materialize"], env, redis) == 2


def test_hash_guard_exits_4(dirs, env, redis, registry_copy, monkeypatch):
    write_inputs(dirs[0], [ev("e1", "view", "u1", "L1", h(1))], [])
    sql = registry_copy / "sql/user_activity.sql"
    sql.write_text(sql.read_text() + "\n-- edited without a version bump\n")
    orig = Settings.from_env.__func__
    monkeypatch.setattr(
        Settings,
        "from_env",
        classmethod(lambda cls, e=None: dataclasses.replace(orig(cls, e), registry_dir=registry_copy)),
    )
    assert cli.main(["materialize"], env, redis) == 4
    assert not list(dirs[1].glob("runs/*"))  # nothing written


def test_committed_lock_matches_registry(settings):
    views = registry.load_registry(settings.registry_dir)
    registry.check_lock(settings.registry_dir, views)
    assert {v.key for v in views} == {"user_activity@v1", "item_popularity@v1"}


def test_unlocked_view_fails(registry_copy):
    views = registry.load_registry(registry_copy)
    (registry_copy / "features.lock").write_text("{}")
    with pytest.raises(registry.RegistryDrift):
        registry.check_lock(registry_copy, views)


@pytest.mark.parametrize("raw", [None, "", "   ", "\t\n"])
def test_empty_as_of_means_now(raw):
    now = datetime(2026, 10, 9, 8, 30, 15, tzinfo=timezone.utc)
    assert parse_as_of(raw, now=now) == datetime(2026, 10, 9, 8, 30, 15)
    s = Settings.from_env({"AS_OF": raw or ""})  # compose passes AS_OF="" when unset
    assert abs((s.as_of - datetime.now(timezone.utc).replace(tzinfo=None)).total_seconds()) < 5


def test_as_of_rfc3339():
    assert parse_as_of("2026-10-09T19:00:00+07:00") == AS_OF
    assert parse_as_of("2026-10-09T12:00:00Z") == AS_OF
    with pytest.raises(ConfigError):
        parse_as_of("yesterday")
    with pytest.raises(ConfigError):
        parse_as_of("2026-10-09T12:00:00")  # no offset


def test_a_definition_cannot_read_parquet_directly(dirs):
    import duckdb

    from featurestore import inputs

    in_dir, _ = dirs
    write_inputs(in_dir)
    con = inputs.connect(in_dir, AS_OF)
    with pytest.raises(duckdb.Error):
        con.execute(f"SELECT * FROM read_parquet('{in_dir / 'tracking_events_resolved.parquet'}')").fetchall()
    with pytest.raises(duckdb.Error):
        con.execute("SET enable_external_access = true")
    assert con.execute("SELECT count(*) FROM events").fetchone()[0] == 0
