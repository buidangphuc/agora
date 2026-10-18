"""Hidden review exclusion (featurestore-hidden-review-exclusion).

Task 1 cases: the `facts` temp table always has a nullable VARCHAR `review_id`.
Spec: openspec/changes/featurestore-hidden-review-exclusion/specs/hidden-review-exclusion/spec.md
"""

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
