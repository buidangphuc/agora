"""als_interactions@v2 / rank_training@v2: personalisation-consent filter with grant boundary.

Spec: openspec/changes/recsys-consent-and-context/specs/recommendations/spec.md
(Requirement: Training interactions exclude events without personalisation consent).
"""

import json
from pathlib import Path

import pyarrow as pa
import pyarrow.parquet as pq
from conftest import AS_OF, h, write_inputs
from test_materialize import ev

from featurestore import cli

STAMP = AS_OF.strftime("%Y%m%dT%H%M%SZ")
G = (True, 1)  # granted
D = (False, 1)  # denied


def add_consent(input_dir, consent):
    """Rewrite tracking_events_resolved.parquet as the new exporter does: append
    consent_personalization / consent_version per event id (absent id -> NULL / 0)."""
    path = input_dir / "tracking_events_resolved.parquet"
    table = pq.read_table(path)
    ids = table.column("event_id").to_pylist()
    table = table.append_column(
        "consent_personalization", pa.array([consent.get(i, (None, 0))[0] for i in ids], pa.bool_())
    )
    table = table.append_column(
        "consent_version", pa.array([consent.get(i, (None, 0))[1] for i in ids], pa.int32())
    )
    pq.write_table(table, path)


def build(dirs, env, name="als_interactions", key=("user_key", "listing_id")):
    assert cli.main(["dataset"], env=env) == 0
    base = dirs[1] / "datasets" / name / "v2" / f"as_of={STAMP}"
    table = pq.read_table(f"{base}.parquet")
    rows = {tuple(r[k] for k in key): r for r in table.to_pylist()}
    return rows, json.loads(Path(f"{base}.manifest.json").read_text()), table


def users(rows):
    return {k[0] for k in rows}


def test_denied_buyer_has_no_interaction_rows(dirs, env):
    """Scenario: Denied buyer has no interaction rows."""
    events = [ev(f"d{i}", "view", "u-deny", f"L{i}", h(10 - i)) for i in range(10)]
    events += [ev(f"o{i}", "view", "u-ok", f"L{i}", h(5 - i)) for i in range(3)]
    write_inputs(dirs[0], events)
    add_consent(dirs[0], {**{f"d{i}": D for i in range(10)}, **{f"o{i}": G for i in range(3)}})
    rows, _, _ = build(dirs, env)
    assert "u-deny" not in users(rows)
    assert len([k for k in rows if k[0] == "u-ok"]) >= 1


def test_denied_buyer_has_no_rank_training_rows(dirs, env):
    """Scenario: Denied buyer has no interaction rows (rank_training dataset)."""
    events = [
        ev("i1", "impression", "u-deny", "L1", h(5), impression="I1", position=1),
        ev("i2", "impression", "u-ok", "L1", h(5), impression="I2", position=1),
    ]
    write_inputs(dirs[0], events)
    add_consent(dirs[0], {"i1": D, "i2": G})
    rows, _, _ = build(dirs, env, "rank_training", ("user_key", "impression_id"))
    assert users(rows) == {"u-ok"}


def test_only_post_grant_events_are_used_after_a_later_grant(dirs, env):
    """Scenario: Only post-grant events are used after a later grant."""
    events = [ev(f"t1-{i}", "view", "u-late", "L1", h(20 - i)) for i in range(4)]
    events += [ev(f"t2-{i}", "view", "u-late", "L1", h(3 - i)) for i in range(2)]
    write_inputs(dirs[0], events)
    add_consent(dirs[0], {**{f"t1-{i}": D for i in range(4)}, **{f"t2-{i}": G for i in range(2)}})
    rows, _, _ = build(dirs, env)
    r = rows[("u-late", "L1")]
    assert r["interactions"] == 2
    assert r["weight"] == 2.0
    assert r["last_occurred_at"] == h(2)


def test_grant_boundary_is_per_user_and_drops_granted_before_a_later_denial(dirs, env):
    """Scenario: Only post-grant events are used after a later grant (boundary = latest denial)."""
    events = [
        ev("a1", "view", "u1", "L1", h(10)),  # granted, then denied later -> dropped
        ev("a2", "view", "u1", "L1", h(8)),  # denied
        ev("a3", "view", "u1", "L1", h(6)),  # granted after the latest denial -> kept
        ev("b1", "view", "u2", "L1", h(9)),  # other user unaffected by u1's denial
    ]
    write_inputs(dirs[0], events)
    add_consent(dirs[0], {"a1": G, "a2": D, "a3": G, "b1": G})
    rows, _, _ = build(dirs, env)
    assert rows[("u1", "L1")]["interactions"] == 1
    assert rows[("u1", "L1")]["last_occurred_at"] == h(6)
    assert rows[("u2", "L1")]["interactions"] == 1


def test_missing_consent_is_denied(dirs, env):
    """Scenario: Missing consent is treated as denied."""
    events = [
        ev("v0", "view", "u-v0", "L1", h(3)),  # consent_version = 0 (even with personalization true)
        ev("nul", "view", "u-null", "L1", h(3)),  # null consent_personalization
        ev("ok", "view", "u-ok", "L1", h(3)),
    ]
    write_inputs(dirs[0], events)
    add_consent(dirs[0], {"v0": (True, 0), "nul": (None, 1), "ok": G})
    rows, _, _ = build(dirs, env)
    assert users(rows) == {"u-ok"}


def test_legacy_parquet_without_consent_columns_loads_and_is_denied(dirs, env):
    """Scenario: Missing consent is treated as denied (tolerant read: legacy Parquet lacks the columns)."""
    write_inputs(dirs[0], [ev("e1", "view", "u1", "L1", h(3)), ev("e2", "click", "u2", "L2", h(2))])
    cols = pq.read_table(dirs[0] / "tracking_events_resolved.parquet").schema.names
    assert "consent_personalization" not in cols and "consent_version" not in cols
    rows, manifest, _ = build(dirs, env)  # exit 0: absent columns do not fail the build
    assert rows == {}
    assert (manifest["name"], manifest["version"], manifest["rows"]) == ("als_interactions", 2, 0)
