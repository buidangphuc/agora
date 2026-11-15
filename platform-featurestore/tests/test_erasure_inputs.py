"""AUE-25 / AUE-26: pseudonymised `erased:<hex>` rows never become features or training rows."""

import pyarrow.parquet as pq
from conftest import AS_OF, ROOT, d, h, write_inputs
from test_materialize import ev, fact, order, rows_by_id, run

from featurestore import cli, registry

STAMP = AS_OF.strftime("%Y%m%dT%H%M%SZ")
ERASED = "erased:9f2c01ab"


def listing(i, category):
    return {"id": i, "category": category, "price": 10, "at": d(3), "seller": "s1"}


def _seed(dirs):
    events = [
        ev("e1", "view", "u-ok", "L1", h(3)),
        ev("e2", "click", "u-ok", "L1", h(2)),
        ev("x1", "view", ERASED, "L1", h(3)),
        ev("x2", "click", ERASED, "L1", h(2)),
        ev("x3", "add_to_cart", ERASED, "L2", h(2)),
    ]
    facts = [
        fact("f1", "favorite_added", "u-ok", h(4), listing="L1", seller="s1"),
        fact("fx", "favorite_added", ERASED, h(4), listing="L1", seller="s1"),
        fact("fy", "seller_followed", ERASED, h(4), seller="s1"),
    ]
    orders = [order("o1", "u-ok", h(5)), order("ox", ERASED, h(5)), order("oy", ERASED, h(6))]
    write_inputs(dirs[0], events, facts, orders, listings=[listing("L1", "cat-a"), listing("L2", "cat-b")])


def test_pseudonymised_rows_excluded_from_views_and_dataset_AUE_25(dirs, env, settings, redis):
    _seed(dirs)
    out = run(settings, redis)
    activity = rows_by_id(out["user_activity@v2"])
    assert ERASED not in activity
    assert activity["u-ok"]["paid_orders_30d"] == 1
    assert activity["u-ok"]["views_7d"] == 1
    assert ERASED not in rows_by_id(out["user_preferences@v1"])
    assert "u-ok" in rows_by_id(out["user_preferences@v1"])

    assert cli.main(["materialize"], env, redis) == 0
    assert cli.main(["dataset"], env=env) == 0
    for view in ("user_activity", "user_preferences"):
        for snap in (dirs[1] / view).glob("v*/as_of=*.parquet"):
            keys = {r["user_key"] for r in pq.read_table(snap).to_pylist()}
            assert ERASED not in keys and "u-ok" in keys
    files = list((dirs[1] / "datasets" / "als_interactions").glob("v*/as_of=*.parquet"))
    assert files
    for f in files:
        assert not [r for r in pq.read_table(f).to_pylist() if str(r["user_key"]).startswith("erased:")]
    v1 = dirs[1] / "datasets" / "als_interactions" / "v1" / f"as_of={STAMP}.parquet"
    assert "u-ok" in {r["user_key"] for r in pq.read_table(v1).to_pylist()}


def test_erased_buyer_counts_for_nobody_in_paid_orders_AUE_25(dirs, settings, redis):
    write_inputs(dirs[0], [ev("e1", "view", "u-ok", "L1", h(1))], [], [order("ox", ERASED, h(3))])
    rows = rows_by_id(run(settings, redis)["user_activity@v2"])
    assert set(rows) == {"u-ok"} and rows["u-ok"]["paid_orders_30d"] == 0


def test_erased_prefix_is_exact_not_a_substring_AUE_25(dirs, settings, redis):
    keys = ["erasedfoo", "u-erased:x", "user:erased"]
    write_inputs(dirs[0], [ev(f"k{i}", "view", k, "L1", h(1)) for i, k in enumerate(keys)])
    assert set(rows_by_id(run(settings, redis)["user_activity@v2"])) == set(keys)


def test_item_views_and_registry_hashes_unchanged_AUE_26(dirs, env, redis):
    write_inputs(dirs[0], [ev("e1", "view", "u-ok", "L1", h(1)), ev("x", "view", ERASED, "L9", h(1))])
    redis.set("fs:item_popularity:v1:Lstale", '{"views_7d": 4}')
    redis.set("fs:item_attributes:v1:Lstale", '{"price": 1}')
    assert cli.main(["materialize"], env, redis) == 0
    assert redis.get("fs:item_popularity:v1:Lstale") is not None
    assert redis.get("fs:item_attributes:v1:Lstale") is not None
    reg = ROOT / "registry"
    registry.check_lock(reg, registry.load_registry(reg), registry.load_datasets(reg))
