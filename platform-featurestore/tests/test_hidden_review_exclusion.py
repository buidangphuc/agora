"""Hidden review exclusion (featurestore-hidden-review-exclusion).

Task 1 cases: the `facts` temp table always has a nullable VARCHAR `review_id`.
Spec: openspec/changes/featurestore-hidden-review-exclusion/specs/hidden-review-exclusion/spec.md
"""

from datetime import timedelta

import pyarrow as pa
import pyarrow.parquet as pq
from conftest import AS_OF, h, write_inputs

from featurestore import inputs

REVIEWS = [
    {"id": "f1", "fact": "review_created", "user": "u1", "listing": "L1", "rating": 5, "at": h(5)},
    {"id": "f2", "fact": "review_created", "user": "u2", "listing": "L1", "rating": 1, "at": h(4)},
]


def _facts_rows(con):
    return con.execute("SELECT event_id, review_id FROM facts ORDER BY event_id").fetchall()


def test_old_export_without_review_id_still_loads(dirs):
    """Scenario: Hidden review exclusion: a review without a review id is still counted
    (export has no review_id column: facts gains a NULL review_id, rows kept)."""
    i, _ = dirs
    write_inputs(i, facts=REVIEWS)
    assert "review_id" not in pq.read_table(i / "engagement_facts.parquet").column_names
    con = inputs.connect(i, AS_OF)
    assert _facts_rows(con) == [("f1", None), ("f2", None)]


def test_missing_review_id_column_is_varchar(dirs):
    """Scenario: Hidden review exclusion: a review without a review id is still counted
    (the added column is a nullable VARCHAR)."""
    i, _ = dirs
    write_inputs(i, facts=REVIEWS)
    con = inputs.connect(i, AS_OF)
    types = {r[0]: r[1] for r in con.execute("DESCRIBE facts").fetchall()}
    assert types["review_id"] == "VARCHAR"


def test_review_id_column_present_is_kept(dirs):
    """Scenario: Hidden review exclusion: a review without a review id is still counted
    (a new export's review_id values, including NULL for a legacy review, are preserved)."""
    i, _ = dirs
    write_inputs(i, facts=REVIEWS)
    table = pq.read_table(i / "engagement_facts.parquet")
    table = table.append_column("review_id", pa.array(["R1", None], pa.string()))
    pq.write_table(table, i / "engagement_facts.parquet")
    con = inputs.connect(i, AS_OF)
    assert _facts_rows(con) == [("f1", "R1"), ("f2", None)]


# ---- Task 2: item_popularity@v2 ----

from conftest import d

from featurestore import job, registry


def _rev(i, rid, rating, at, listing="L1", **kw):
    return {
        "id": i,
        "fact": "review_created",
        "user": "u" + i,
        "listing": listing,
        "rating": rating,
        "at": at,
        "review_id": rid,
        **kw,
    }


def _hide(i, rid, at, listing="L1", **kw):
    return {
        "id": i,
        "fact": "review_hidden",
        "user": "admin",
        "listing": listing,
        "at": at,
        "review_id": rid,
        **kw,
    }


def _view(settings, redis, key):
    views = registry.load_registry(settings.registry_dir)
    rows = job.materialize(settings, views, redis)["_rows"][key]
    return {r["entity_id"]: r for r in rows}


def test_popularity_v2_excludes_hidden_review(dirs, settings, redis):
    """Scenario: Hidden review exclusion: a hidden review leaves review_count and avg_rating."""
    facts = [_rev("f1", "R1", 5, h(10)), _rev("f2", "R2", 1, h(9)), _hide("f3", "R2", h(5))]
    write_inputs(dirs[0], facts=facts)
    row = _view(settings, redis, "item_popularity@v2")["L1"]
    assert row["review_count"] == 1
    assert row["avg_rating"] == 5.0


def test_popularity_v2_only_review_hidden(dirs, settings, redis):
    """Scenario: Hidden review exclusion: a listing whose only review is hidden has no rating."""
    facts = [_rev("f1", "R1", 5, h(10)), _hide("f2", "R1", h(5))]
    events = [
        {"id": "e1", "type": "view", "user": "u1", "listing": "L1", "at": h(2)},
        {"id": "e2", "type": "view", "user": "u2", "listing": "L1", "at": h(3)},
    ]
    write_inputs(dirs[0], events=events, facts=facts)
    row = _view(settings, redis, "item_popularity@v2")["L1"]
    assert row["review_count"] == 0
    assert row["avg_rating"] is None
    assert row["views_7d"] == 2


def test_popularity_v2_hide_after_as_of_not_applied(dirs, settings, redis):
    """Scenario: Hidden review exclusion: a hide recorded after AS_OF is not applied."""
    late = AS_OF + timedelta(hours=3)
    facts = [
        _rev("f1", "R1", 5, h(10)),
        _rev("f2", "R2", 1, h(9)),
        _hide("f3", "R2", h(5), ing=late),
    ]
    write_inputs(dirs[0], facts=facts)
    row = _view(settings, redis, "item_popularity@v2")["L1"]
    assert row["review_count"] == 2
    assert row["avg_rating"] == 3.0


def test_popularity_v2_unknown_review_id_ignored(dirs, settings, redis):
    """Scenario: Hidden review exclusion: a review_hidden for an unknown review id changes nothing."""
    base = [_rev("f1", "R1", 5, h(10)), _rev("f2", "R2", 1, h(9)), _rev("f4", "R4", 4, h(8), listing="L2")]
    write_inputs(dirs[0], facts=base)
    before = _view(settings, redis, "item_popularity@v2")
    assert before["L1"]["review_count"] == 2  # v2 exists and counts visible reviews
    write_inputs(dirs[0], facts=[*base, _hide("f5", "R-unknown", h(5))])
    after = _view(settings, redis, "item_popularity@v2")
    assert {k: (v["review_count"], v["avg_rating"]) for k, v in after.items()} == {
        k: (v["review_count"], v["avg_rating"]) for k, v in before.items()
    }


def test_popularity_v2_null_review_id_still_counted(dirs, settings, redis):
    """Scenario: Hidden review exclusion: a review without a review id is still counted
    (legacy NULL review_id is never excluded, even when the listing has a hide for another id)."""
    facts = [
        _rev("f1", None, 4, h(10)),
        _rev("f2", "R2", 2, h(9)),
        _hide("f3", "R2", h(5)),
    ]
    write_inputs(dirs[0], facts=facts)
    row = _view(settings, redis, "item_popularity@v2")["L1"]
    assert row["review_count"] == 1
    assert row["avg_rating"] == 4.0


def test_popularity_v2_non_review_columns_equal_v1(dirs, settings, redis):
    """Scenario: Hidden review exclusion: non-review columns match v1."""
    events = [
        {"id": "e1", "type": "view", "user": "u1", "listing": "L1", "at": h(1)},
        {"id": "e2", "type": "click", "user": "u1", "listing": "L1", "at": h(2)},
        {"id": "e3", "type": "add_to_cart", "user": "u1", "listing": "L1", "at": h(3)},
        {"id": "e4", "type": "impression", "user": "u1", "listing": "L1", "at": h(3)},
        {"id": "e5", "type": "impression", "user": "u2", "listing": "L1", "at": h(3)},
        {"id": "e6", "type": "view", "user": "u2", "listing": "L2", "at": d(6, 23)},
    ]
    facts = [
        {"id": "f1", "fact": "favorite_added", "user": "u1", "listing": "L1", "at": h(6)},
        {"id": "f2", "fact": "favorite_added", "user": "u2", "listing": "L2", "at": h(6)},
        {"id": "f3", "fact": "favorite_removed", "user": "u2", "listing": "L2", "at": h(5)},
        _rev("f4", "R1", 5, h(8)),
        _hide("f5", "R1", h(4)),
    ]
    write_inputs(dirs[0], events=events, facts=facts)
    v1 = _view(settings, redis, "item_popularity@v1")
    v2 = _view(settings, redis, "item_popularity@v2")
    cols = ["views_7d", "clicks_7d", "add_to_cart_7d", "favorites_current", "ctr_7d"]
    assert v2.keys() == v1.keys()
    for k in v1:
        assert {c: v2[k][c] for c in cols} == {c: v1[k][c] for c in cols}
    assert v1["L1"]["favorites_current"] == 1 and v1["L1"]["ctr_7d"] == 0.5
