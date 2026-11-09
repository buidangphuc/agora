"""item_attributes@v1 and user_preferences@v1 (featurestore-item-attributes)."""

import json

import pyarrow.parquet as pq
import pytest
from conftest import d, h, write_inputs, write_listings
from test_materialize import ev, rows_by_id, run

from featurestore import cli, job, registry


def listing(i, category, price, at=None, seller="s1"):
    return {"id": i, "category": category, "price": price, "at": at or d(3), "seller": seller}


def test_item_attributes_carry_seller_category_price_and_null_when_unknown(dirs, settings, redis):
    write_inputs(
        dirs[0],
        listings=[listing("L1", "cat-a", 4200, seller="sellerA"), listing("L2", None, None)],
    )
    rows = rows_by_id(run(settings, redis)["item_attributes@v1"])
    assert rows["L1"] == {"entity_id": "L1", "seller_id": "sellerA", "category_id": "cat-a", "price": 4200}
    assert rows["L2"]["category_id"] is None and rows["L2"]["price"] is None


def test_listing_changed_after_as_of_is_not_in_the_snapshot(dirs, settings, redis):
    write_inputs(dirs[0], listings=[listing("before", "cat-a", 1, h(2)), listing("after", "cat-a", 1, h(-2))])
    assert set(rows_by_id(run(settings, redis)["item_attributes@v1"])) == {"before"}


def test_preferred_categories_are_the_top_three_by_weighted_interactions(dirs, settings, redis):
    listings = [listing(f"L{c}", f"cat-{c}", 1) for c in "abcde"]
    events = (
        [ev(f"va{i}", "view", "u1", "La", h(i + 1)) for i in range(3)]  # cat-a: 3
        + [ev("cb", "click", "u1", "Lb", h(1))]  # cat-b: 2
        + [ev("vc", "view", "u1", "Lc", h(1)), ev("vd", "view", "u1", "Ld", h(1))]  # cat-c 1, cat-d 1
        + [ev("ce", "add_to_cart", "u1", "Le", h(1))]  # cat-e: 5
        + [ev("old", "view", "u1", "Lb", d(31))]  # outside 30 days
        + [ev("imp", "impression", "u2", "La", h(1))]  # impressions carry no preference
        + [ev("u3v", "view", "u3", "Lunknown", h(1))]  # listing without attributes
    )
    write_inputs(dirs[0], events, listings=listings)
    rows = rows_by_id(run(settings, redis)["user_preferences@v1"])
    assert rows["u1"]["preferred_categories"] == "cat-e,cat-a,cat-b"  # 5, 3, 2; cat-c/d tie lost the cut
    assert "u2" not in rows and "u3" not in rows


def test_preference_ties_break_by_category_name(dirs, settings, redis):
    listings = [listing("Lz", "cat-z", 1), listing("La", "cat-a", 1)]
    events = [ev("1", "view", "u1", "Lz", h(1)), ev("2", "view", "u1", "La", h(1))]
    write_inputs(dirs[0], events, listings=listings)
    assert rows_by_id(run(settings, redis)["user_preferences@v1"])["u1"]["preferred_categories"] == "cat-a,cat-z"


def test_online_values_and_parity(dirs, env, redis, capsys):
    write_inputs(dirs[0], [ev("1", "view", "u1", "L1", h(1))], listings=[listing("L1", "cat-a", 99)])
    assert cli.main(["materialize"], env, redis) == 0
    assert json.loads(redis.get("fs:item_attributes:v1:L1")) == {
        "seller_id": "s1",
        "category_id": "cat-a",
        "price": 99,
    }
    assert json.loads(redis.get("fs:user_preferences:v1:u1")) == {"preferred_categories": "cat-a"}
    assert redis.get("fs:item_attributes:current") == "2" and redis.get("fs:user_preferences:current") == "1"
    row = json.loads(redis.get("fs:item_attributes:v1:L1"))
    row["category_id"] = "cat-tampered"
    redis.set("fs:item_attributes:v1:L1", json.dumps(row))
    capsys.readouterr()
    assert cli.main(["parity"], env, redis) == 3
    out = capsys.readouterr().out
    assert "view=item_attributes entity=L1 feature=category_id online=cat-tampered offline=cat-a" in out


def test_missing_listing_export_gives_empty_views_and_a_warning(dirs, env, redis, capsys):
    write_inputs(dirs[0], [ev("1", "view", "u1", "L1", h(1))])  # no listing_sellers.parquet
    assert cli.main(["materialize"], env, redis) == 0
    assert "listing_sellers.parquet not found" in capsys.readouterr().err
    for view in ("item_attributes/v1", "user_preferences/v1"):
        (snap,) = (dirs[1] / view).glob("as_of=*.parquet")
        assert pq.read_table(snap).num_rows == 0
    assert redis.get("fs:user_activity:v2:u1")  # the other views are unaffected


@pytest.mark.parametrize("column", ["category_id", "price"])
def test_listing_export_without_the_new_columns_exits_2(dirs, env, redis, capsys, column):
    write_inputs(dirs[0], [ev("1", "view", "u1", "L1", h(1))])
    write_listings(dirs[0], [listing("L1", "cat-a", 1)], drop=[column])
    assert cli.main(["materialize"], env, redis) == 2
    assert column in capsys.readouterr().err


def test_committed_lock_covers_the_attribute_views(settings):
    views = registry.load_registry(settings.registry_dir)
    assert {"item_attributes@v1", "user_preferences@v1"} <= {v.key for v in views}
    registry.check_lock(settings.registry_dir, views, registry.load_datasets(settings.registry_dir))


def test_manifest_lists_the_listing_input_only_when_present(dirs, settings, redis):
    write_inputs(dirs[0], listings=[listing("L1", "cat-a", 1)])
    views = registry.load_registry(settings.registry_dir)
    manifest = job.materialize(settings, views, redis)
    assert "listings" in {i["input"] for i in manifest["inputs"]}
    assert {v["name"] for v in manifest["views"]} >= {"item_attributes", "user_preferences"}


V1_LOCKED_HASH = "d50a0e9050bd3b4887c23410555141a6b1819a486ece056a01b13a6538914973"


def v2_rows(settings, redis):
    return rows_by_id(run(settings, redis)["item_attributes@v2"])


def test_v2_carries_status(dirs, env, settings, redis):
    write_inputs(
        dirs[0],
        listings=[{**listing("L1", "cat-a", 4200, seller="sellerA"), "status": "LISTING_STATUS_PUBLISHED"}],
    )
    assert v2_rows(settings, redis)["L1"] == {
        "entity_id": "L1",
        "seller_id": "sellerA",
        "category_id": "cat-a",
        "price": 4200,
        "status": "LISTING_STATUS_PUBLISHED",
    }
    assert cli.main(["materialize"], env, redis) == 0
    assert json.loads(redis.get("fs:item_attributes:v2:L1"))["status"] == "LISTING_STATUS_PUBLISHED"


def test_v2_without_status_column_is_null_not_an_error(dirs, env, settings, redis, capsys):
    write_inputs(dirs[0], listings=[listing("L1", "cat-a", 1), listing("L2", "cat-b", 2)])
    assert cli.main(["materialize"], env, redis) == 0
    assert "status" in capsys.readouterr().err
    (snap,) = (dirs[1] / "item_attributes/v2").glob("as_of=*.parquet")
    table = pq.read_table(snap)
    assert table.num_rows == 2
    assert table.column("status").to_pylist() == [None, None]
    assert json.loads(redis.get("fs:item_attributes:v2:L1"))["status"] is None


def test_v1_definition_is_unchanged(settings):
    views = {v.key: v for v in registry.load_registry(settings.registry_dir)}
    assert views["item_attributes@v1"].sha256 == V1_LOCKED_HASH
    assert "item_attributes@v2" in views
    registry.check_lock(
        settings.registry_dir, list(views.values()), registry.load_datasets(settings.registry_dir)
    )


def test_v2_listing_after_as_of_excluded(dirs, settings, redis):
    published = "LISTING_STATUS_PUBLISHED"
    write_inputs(
        dirs[0],
        listings=[
            {**listing("before", "cat-a", 1, h(2)), "status": published},
            {**listing("after", "cat-a", 1, h(-2)), "status": published},
        ],
    )
    assert set(v2_rows(settings, redis)) == {"before"}


def test_v2_unspecified_or_unknown_status_is_null(dirs, settings, redis):
    write_inputs(
        dirs[0],
        listings=[
            {**listing("L1", "cat-a", 1), "status": "LISTING_STATUS_UNSPECIFIED"},
            {**listing("L2", "cat-a", 1), "status": "LISTING_STATUS_FOO"},
            {**listing("L3", "cat-a", 1), "status": "LISTING_STATUS_BANNED"},
        ],
    )
    rows = v2_rows(settings, redis)
    assert rows["L1"]["status"] is None and rows["L2"]["status"] is None
    assert rows["L3"]["status"] == "LISTING_STATUS_BANNED"


def test_current_is_2_and_both_versions_exist(dirs, env, redis):
    write_inputs(dirs[0], listings=[{**listing("L1", "cat-a", 99), "status": "LISTING_STATUS_PUBLISHED"}])
    assert cli.main(["materialize"], env, redis) == 0
    assert redis.get("fs:item_attributes:current") == "2"
    assert redis.get("fs:item_attributes:v1:L1") is not None
    assert redis.get("fs:item_attributes:v2:L1") is not None
