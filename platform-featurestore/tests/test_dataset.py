import dataclasses
import hashlib
import json
from datetime import timedelta
from pathlib import Path

import pyarrow.parquet as pq
from conftest import AS_OF, d, h, write_inputs
from test_materialize import ev, fact

from featurestore import cli, registry
from featurestore.settings import Settings

STAMP = AS_OF.strftime("%Y%m%dT%H%M%SZ")


def build(dirs, env, extra=None):
    e = {**env, **(extra or {})}
    assert cli.main(["dataset"], env=e) == 0
    base = dirs[1] / "datasets" / "als_interactions" / "v1" / f"as_of={STAMP}"
    table = pq.read_table(f"{base}.parquet")
    rows = {(r["user_key"], r["listing_id"]): r for r in table.to_pylist()}
    manifest = json.loads(Path(f"{base}.manifest.json").read_text())
    return rows, manifest, table


def test_event_weights_and_columns(dirs, env):
    types = {
        "impression": 0.5, "view": 1.0, "click": 2.0, "view_cart": 2.5, "add_to_cart": 5.0,
        "add_shipping_info": 6.0, "add_payment_info": 7.0, "begin_checkout": 8.0, "purchase": 10.0,
        "other_thing": 0.0,
    }  # fmt: skip
    events = [ev(f"e{i}", t, "u1", f"L-{t}", h(1)) for i, t in enumerate(types)]
    write_inputs(dirs[0], events)
    rows, _, table = build(dirs, env)
    assert table.schema.names == ["user_key", "listing_id", "weight", "interactions", "last_occurred_at"]
    assert str(table.schema.field("weight").type) == "double"
    assert str(table.schema.field("interactions").type).startswith("int")
    assert str(table.schema.field("last_occurred_at").type).startswith("timestamp")
    for t, w in types.items():
        if w > 0:
            assert rows[("u1", f"L-{t}")]["weight"] == w
        else:
            assert ("u1", f"L-{t}") not in rows  # weight 0 dropped


def test_views_plus_favourite_is_weight_5_interactions_3(dirs, env):
    events = [ev("e1", "view", "u1", "L1", h(3)), ev("e2", "view", "u1", "L1", h(2))]
    facts = [fact("f1", "favorite_added", "u1", h(1), listing="L1")]
    write_inputs(dirs[0], events, facts)
    rows, _, _ = build(dirs, env)
    r = rows[("u1", "L1")]
    assert (r["weight"], r["interactions"], r["last_occurred_at"]) == (5.0, 3, h(1))


def test_favourite_removed_contributes_nothing(dirs, env):
    facts = [
        fact("f1", "favorite_added", "u1", h(3), listing="L1"),
        fact("f2", "favorite_removed", "u1", h(2), listing="L1"),
        fact("f3", "favorite_added", "u1", h(5), listing="L2"),
        fact("f4", "favorite_removed", "u1", h(4), listing="L2"),
        fact("f5", "favorite_added", "u1", h(1), listing="L2"),  # re-added: current again
    ]
    write_inputs(dirs[0], [], facts)
    rows, _, _ = build(dirs, env)
    assert ("u1", "L1") not in rows
    assert rows[("u1", "L2")]["weight"] == 3.0


def test_review_adjustments_and_non_positive_dropped(dirs, env):
    events = [ev("e1", "view", "u1", "L1", h(5)), ev("e2", "view", "u2", "L1", h(5)),
              ev("e3", "view", "u3", "L1", h(5))]  # fmt: skip
    facts = [
        fact("f1", "review_created", "u1", h(1), listing="L1", rating=5),  # 1 + 2
        fact("f2", "review_created", "u2", h(1), listing="L1", rating=1),  # 1 - 2 -> dropped
        fact("f3", "review_created", "u3", h(1), listing="L1", rating=3),  # 1 + 0
        fact("f4", "review_created", "u4", h(1), listing="L2", rating=2),  # -2 -> dropped
        fact("f5", "review_created", "u5", h(1), listing="L2", rating=4),  # +2 alone
    ]
    write_inputs(dirs[0], events, facts)
    rows, _, _ = build(dirs, env)
    assert rows[("u1", "L1")]["weight"] == 3.0 and rows[("u1", "L1")]["interactions"] == 2
    assert ("u2", "L1") not in rows and ("u4", "L2") not in rows
    assert rows[("u3", "L1")]["weight"] == 1.0
    assert rows[("u5", "L2")]["weight"] == 2.0 and rows[("u5", "L2")]["interactions"] == 1


def test_stitched_user_key_merges_pre_login(dirs, env):
    # the resolved export already carries the stitched key: both views share one user_key
    events = [ev("e1", "view", "buyer", "L1", h(3)), ev("e2", "view", "buyer", "L1", h(1))]
    write_inputs(dirs[0], events)
    rows, _, _ = build(dirs, env)
    assert list(rows) == [("buyer", "L1")] and rows[("buyer", "L1")]["weight"] == 2.0


def test_window_and_as_of_exclusion(dirs, env):
    events = [
        ev("e1", "view", "u1", "L1", d(29)),  # inside 30d
        ev("e2", "view", "u1", "L2", d(31)),  # outside window
        ev("e3", "view", "u2", "L1", AS_OF + timedelta(minutes=1)),  # occurs after AS_OF
        ev("e4", "view", "u3", "L1", h(2), ing=AS_OF + timedelta(minutes=1)),  # ingested after AS_OF
    ]
    write_inputs(dirs[0], events)
    rows, _, _ = build(dirs, env)
    assert list(rows) == [("u1", "L1")]
    rows, _, _ = build(dirs, env, {"DATASET_WINDOW_DAYS": "60"})
    assert ("u1", "L2") in rows


def test_manifest_sha_matches_file(dirs, env):
    write_inputs(dirs[0], [ev("e1", "view", "u1", "L1", h(1)), ev("e2", "view", "u2", "L1", h(1)),
                           ev("e3", "view", "u2", "L2", h(1))])  # fmt: skip
    _, m, _ = build(dirs, env)
    f = dirs[1] / "datasets" / "als_interactions" / "v1" / m["file"]
    assert m["file_sha256"] == hashlib.sha256(f.read_bytes()).hexdigest()
    assert (m["name"], m["version"], m["as_of"], m["window_days"]) == (
        "als_interactions",
        1,
        "2026-10-09T12:00:00Z",
        30,
    )
    assert (m["rows"], m["users"], m["items"]) == (3, 2, 2)
    assert m["input_watermark"] == "2026-10-09T11:00:00Z"
    ds = registry.load_datasets(Settings.from_env(env).registry_dir)[0]
    assert m["definition_sha256"] == ds.sha256


def test_committed_lock_covers_dataset(settings):
    views = registry.load_registry(settings.registry_dir)
    datasets = registry.load_datasets(settings.registry_dir)
    registry.check_lock(settings.registry_dir, views, datasets)
    assert registry.read_lock(settings.registry_dir)["als_interactions@v1"] == datasets[0].sha256


def test_dataset_definition_drift_exits_4(dirs, env, registry_copy, monkeypatch):
    write_inputs(dirs[0], [ev("e1", "view", "u1", "L1", h(1))])
    sql = registry_copy / "sql" / "als_interactions_v1.sql"
    sql.write_text(sql.read_text() + "\n-- tweak")
    orig = Settings.from_env.__func__
    monkeypatch.setattr(
        Settings,
        "from_env",
        classmethod(lambda cls, e=None: dataclasses.replace(orig(cls, e), registry_dir=registry_copy)),
    )
    assert cli.main(["dataset"], env=env) == 4
    assert not (dirs[1] / "datasets").exists()


def test_missing_input_exits_2(env):
    assert cli.main(["dataset"], env=env) == 2


def test_bad_window_exits_2(dirs, env):
    assert cli.main(["dataset"], env={**env, "DATASET_WINDOW_DAYS": "0"}) == 2
