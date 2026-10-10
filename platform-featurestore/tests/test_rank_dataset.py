"""rank_training@v1: one labelled row per item impression (recsys-gbdt-trainer)."""

import hashlib
import json
from datetime import timedelta
from pathlib import Path

import pyarrow.parquet as pq
from conftest import AS_OF, d, h, write_inputs
from test_materialize import ev

from featurestore import cli, registry
from featurestore.settings import Settings

STAMP = AS_OF.strftime("%Y%m%dT%H%M%SZ")


def build(dirs, env, extra=None):
    assert cli.main(["dataset"], env={**env, **(extra or {})}) == 0
    base = dirs[1] / "datasets" / "rank_training" / "v1" / f"as_of={STAMP}"
    table = pq.read_table(f"{base}.parquet")
    rows = {(r["impression_id"], r["listing_id"]): r for r in table.to_pylist()}
    return rows, json.loads(Path(f"{base}.manifest.json").read_text()), table


def imp(i, user, listing, at, impression="I1", position=1, **kw):
    return ev(i, "impression", user, listing, at, impression=impression, position=position, **kw)


def act(i, typ, user, listing, at, impression="I1"):
    return ev(i, typ, user, listing, at, impression=impression)


def test_columns_types_and_the_three_labels(dirs, env):
    events = [
        imp("i1", "u1", "L1", h(5), position=1),
        imp("i2", "u1", "L2", h(5), position=2),
        imp("i3", "u1", "L3", h(5), position=3),
        act("c1", "click", "u1", "L1", h(4)),
        act("a2", "add_to_cart", "u1", "L2", h(4)),
    ]
    write_inputs(dirs[0], events)
    rows, manifest, table = build(dirs, env)
    assert table.schema.names == ["user_key", "impression_id", "listing_id", "position", "label", "occurred_at"]
    assert str(table.schema.field("position").type).startswith("int")
    assert str(table.schema.field("label").type).startswith("int")
    assert str(table.schema.field("occurred_at").type).startswith("timestamp")
    assert {k[1]: (r["label"], r["position"]) for k, r in rows.items()} == {"L1": (1, 1), "L2": (2, 2), "L3": (0, 3)}
    assert rows[("I1", "L1")]["user_key"] == "u1"
    assert (manifest["name"], manifest["version"], manifest["rows"], manifest["users"]) == ("rank_training", 1, 3, 1)


def test_add_to_cart_beats_a_click_on_the_same_impression(dirs, env):
    events = [
        imp("i1", "u1", "L1", h(5)),
        act("c1", "click", "u1", "L1", h(4)),
        act("a1", "add_to_cart", "u1", "L1", h(3)),
    ]
    write_inputs(dirs[0], events)
    assert build(dirs, env)[0][("I1", "L1")]["label"] == 2


def test_outcomes_need_the_same_impression_id_and_listing_and_must_not_precede_it(dirs, env):
    events = [
        imp("i1", "u1", "L1", h(5), impression="I1"),
        act("c-other-imp", "click", "u1", "L1", h(4), impression="I2"),
        act("c-other-listing", "click", "u1", "L9", h(4), impression="I1"),
        act("c-before", "click", "u1", "L1", h(6), impression="I1"),  # before the impression
        ev("c-no-id", "click", "u1", "L1", h(4)),  # no impression id at all
    ]
    write_inputs(dirs[0], events)
    assert build(dirs, env)[0][("I1", "L1")]["label"] == 0


def test_impressions_without_ids_or_user_are_not_rows_and_a_repeat_is_one_row(dirs, env):
    events = [
        imp("i1", "u1", "L1", h(5), position=4),
        imp("i1b", "u1", "L1", h(4), position=1),  # the same pair again: the earliest is the row
        ev("noimp", "impression", "u1", "L2", h(5)),  # no impression_id
        imp("nouser", "", "L3", h(5)),
        ev("view", "view", "u1", "L4", h(5), impression="I1"),  # not an impression event
    ]
    write_inputs(dirs[0], events)
    rows, _, _ = build(dirs, env)
    assert list(rows) == [("I1", "L1")] and rows[("I1", "L1")]["position"] == 4


def test_unknown_position_is_zero(dirs, env):
    write_inputs(dirs[0], [imp("i1", "u1", "L1", h(5), position=None)])
    assert build(dirs, env)[0][("I1", "L1")]["position"] == 0


def test_impressions_after_as_of_or_outside_the_window_are_not_rows(dirs, env):
    events = [
        imp("early", "u1", "L1", h(5), impression="I1"),
        imp("late", "u1", "L2", AS_OF + timedelta(minutes=1), impression="I2", ing=h(1)),  # occurs after AS_OF
        imp("late-ingest", "u1", "L3", h(5), impression="I3", ing=AS_OF + timedelta(minutes=1)),
        imp("old", "u1", "L4", d(31), impression="I4"),  # outside the 30-day window
        # a click after AS_OF does not make the early impression positive
        act("late-click", "click", "u1", "L1", AS_OF + timedelta(minutes=2), impression="I1"),
    ]
    write_inputs(dirs[0], events)
    rows, _, _ = build(dirs, env)
    assert list(rows) == [("I1", "L1")] and rows[("I1", "L1")]["label"] == 0
    rows, _, _ = build(dirs, env, {"DATASET_WINDOW_DAYS": "60"})
    assert ("I4", "L4") in rows


def test_manifest_and_lock(dirs, env, settings):
    write_inputs(dirs[0], [imp("i1", "u1", "L1", h(5))])
    _, m, _ = build(dirs, env)
    f = dirs[1] / "datasets" / "rank_training" / "v1" / m["file"]
    assert m["file_sha256"] == hashlib.sha256(f.read_bytes()).hexdigest()
    datasets = {x.key: x for x in registry.load_datasets(Settings.from_env(env).registry_dir)}
    assert m["definition_sha256"] == datasets["rank_training@v1"].sha256
    registry.check_lock(settings.registry_dir, registry.load_registry(settings.registry_dir), list(datasets.values()))


def test_the_als_dataset_is_unchanged_by_per_dataset_columns(settings):
    als = {x.key: x for x in registry.load_datasets(settings.registry_dir)}["als_interactions@v1"]
    assert als.columns is None  # keeps the als columns, so its lock hash did not move
    assert registry.read_lock(settings.registry_dir)["als_interactions@v1"] == als.sha256
    assert als.sha256 == "9fb41c13f9085e3417bec6c8ec92c63a2d7cfae19b9139a641d3988cc17d62c4"
