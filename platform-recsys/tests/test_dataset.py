"""Governed dataset resolution, refusal and lineage (PySpark-free)."""

import hashlib
import json
import logging
from datetime import datetime, timedelta, timezone

import pytest

from recsys import __main__ as entry
from recsys.config import ConfigError, Settings
from recsys.dataset import manifest_path_for, resolve_dataset
from tests.dataset_fixture import sample_rows, write_dataset
from tests.pipeline_fixture import forbid_spark

# INTERFACE ASSUMPTION (RAT-22..RAT-24): ``resolve_dataset(settings, now=None)`` raises ConfigError naming
# MAX_DATASET_AGE_MINUTES when the newest snapshot's manifest ``as_of`` is older than the setting (0 = off);
# ``now`` (timezone-aware) is injectable so the tests never read the wall clock.
NOW = datetime(2026, 10, 13, 12, 0, 0, tzinfo=timezone.utc)


def _stamp(age_minutes: float) -> str:
    return (NOW - timedelta(minutes=age_minutes)).strftime("%Y%m%dT%H%M%SZ")


def test_latest_snapshot_under_dataset_dir_wins(tmp_path):
    write_dataset(tmp_path, sample_rows(), as_of="20261003T020000Z")
    newest = write_dataset(tmp_path, sample_rows(), as_of="20261005T020000Z")
    write_dataset(tmp_path, sample_rows(), as_of="20261004T020000Z")
    ds = resolve_dataset(Settings(dataset_dir=str(tmp_path)))
    assert ds.path == str(newest)
    assert ds.manifest["as_of"] == "20261005T020000Z"


def test_dataset_path_overrides_dataset_dir(tmp_path):
    write_dataset(tmp_path / "dir", sample_rows(), as_of="20261009T020000Z")
    explicit = write_dataset(tmp_path / "pinned", sample_rows(), as_of="20261001T020000Z")
    ds = resolve_dataset(Settings(dataset_dir=str(tmp_path / "dir"), dataset_path=str(explicit)))
    assert ds.path == str(explicit)
    assert ds.manifest["as_of"] == "20261001T020000Z"


def test_manifest_name_replaces_parquet_suffix(tmp_path):
    p = tmp_path / "as_of=20261005T020000Z.parquet"
    assert manifest_path_for(p).name == "as_of=20261005T020000Z.manifest.json"


def test_snapshot_without_manifest_is_ignored(tmp_path):
    good = write_dataset(tmp_path, sample_rows(), as_of="20261003T020000Z")
    newer = write_dataset(tmp_path, sample_rows(), as_of="20261005T020000Z")
    manifest_path_for(newer).unlink()
    assert resolve_dataset(Settings(dataset_dir=str(tmp_path))).path == str(good)


def test_empty_dir_raises_naming_dataset_dir(tmp_path):
    with pytest.raises(ConfigError, match=f"DATASET_DIR={tmp_path}"):
        resolve_dataset(Settings(dataset_dir=str(tmp_path)))


def test_missing_dir_raises(tmp_path):
    with pytest.raises(ConfigError, match="DATASET_DIR="):
        resolve_dataset(Settings(dataset_dir=str(tmp_path / "nope")))


def test_dataset_path_without_file_or_manifest_raises(tmp_path):
    with pytest.raises(ConfigError, match="DATASET_PATH="):
        resolve_dataset(Settings(dataset_path=str(tmp_path / "x.parquet")))
    p = write_dataset(tmp_path, sample_rows())
    manifest_path_for(p).unlink()
    with pytest.raises(ConfigError, match="manifest"):
        resolve_dataset(Settings(dataset_path=str(p)))


def test_lineage_is_copied_from_the_manifest(tmp_path):
    write_dataset(tmp_path, sample_rows(), as_of="20261005T020000Z")
    ds = resolve_dataset(Settings(dataset_dir=str(tmp_path)))
    assert ds.lineage == {
        "name": "als_interactions",
        "version": 1,
        "as_of": "20261005T020000Z",
        "sha256": ds.manifest["file_sha256"],
    }
    assert len(ds.lineage["sha256"]) == 64


def test_main_exits_2_naming_dataset_dir_and_registers_nothing(tmp_path, reg, monkeypatch, caplog):
    monkeypatch.setenv("DATASET_DIR", str(tmp_path))
    monkeypatch.delenv("DATASET_PATH", raising=False)
    # Must refuse before any Spark work, and register nothing in the model registry.
    forbid_spark(monkeypatch)
    with caplog.at_level(logging.ERROR):
        assert entry.main([]) == 2
    assert f"DATASET_DIR={tmp_path}" in caplog.text
    assert not reg.client.search_registered_models()


# --- RAT-22 / RAT-23 / RAT-24: the freshness guard ---------------------------------------------------
def test_stale_dataset_is_refused_naming_the_setting(tmp_path):
    # RAT-22 (unit level): 26 h old with MAX_DATASET_AGE_MINUTES=1500
    write_dataset(tmp_path, sample_rows(), as_of=_stamp(26 * 60))
    with pytest.raises(ConfigError, match="MAX_DATASET_AGE_MINUTES"):
        resolve_dataset(Settings(dataset_dir=str(tmp_path), max_dataset_age_minutes=1500), now=NOW)


def test_age_guard_boundary(tmp_path):
    # RAT-23: 1499 minutes old with a 1500 window trains; the window itself is inclusive; one more is refused
    for age, ok in ((1499, True), (1500, True), (1501, False)):
        d = tmp_path / f"age-{age}"
        write_dataset(d, sample_rows(), as_of=_stamp(age))
        cfg = Settings(dataset_dir=str(d), max_dataset_age_minutes=1500)
        if ok:
            assert resolve_dataset(cfg, now=NOW).manifest["as_of"] == _stamp(age)
        else:
            with pytest.raises(ConfigError, match="MAX_DATASET_AGE_MINUTES"):
                resolve_dataset(cfg, now=NOW)


def test_age_guard_disabled(tmp_path):
    # RAT-24: 0 switches the check off, however old the snapshot is
    write_dataset(tmp_path, sample_rows(), as_of=_stamp(30 * 24 * 60))
    ds = resolve_dataset(Settings(dataset_dir=str(tmp_path), max_dataset_age_minutes=0), now=NOW)
    assert ds.manifest["as_of"] == _stamp(30 * 24 * 60)


def test_age_guard_judges_the_newest_snapshot(tmp_path):
    # RAT-22/23: an old snapshot beside a fresh one does not trip the guard (the newest is what trains)
    write_dataset(tmp_path, sample_rows(), as_of=_stamp(5 * 24 * 60))
    fresh = write_dataset(tmp_path, sample_rows(), as_of=_stamp(10))
    ds = resolve_dataset(Settings(dataset_dir=str(tmp_path), max_dataset_age_minutes=60), now=NOW)
    assert ds.path == str(fresh)


def test_age_guard_applies_to_an_explicit_dataset_path(tmp_path):
    # RAT-22: DATASET_PATH pins a file, it does not exempt it from the guard
    old = write_dataset(tmp_path / "pinned", sample_rows(), as_of=_stamp(26 * 60))
    with pytest.raises(ConfigError, match="MAX_DATASET_AGE_MINUTES"):
        resolve_dataset(Settings(dataset_path=str(old), max_dataset_age_minutes=1500), now=NOW)


def test_main_refuses_a_stale_dataset_with_exit_2_before_spark(tmp_path, reg, monkeypatch, caplog):
    # RAT-22 (job level): exit 2, Spark never starts, no model version registered
    write_dataset(tmp_path, sample_rows(), as_of="20200101T000000Z")  # years old, real clock
    monkeypatch.setenv("DATASET_DIR", str(tmp_path))
    monkeypatch.delenv("DATASET_PATH", raising=False)
    monkeypatch.setenv("MAX_DATASET_AGE_MINUTES", "1500")
    forbid_spark(monkeypatch)
    with caplog.at_level(logging.ERROR):
        assert entry.main([]) == 2
    assert "MAX_DATASET_AGE_MINUTES" in caplog.text
    assert not reg.client.search_registered_models()


def _with_extra_columns(parquet, manifest_path):
    """Rewrite a governed dataset parquet carrying the ecommerce columns (item_list_id, position,
    transaction_id) and re-pin the manifest sha, as a newer featurestore export would."""
    import pandas as pd

    df = pd.read_parquet(parquet)
    df["item_list_id"] = "search"
    df["position"] = 3
    df["transaction_id"] = ""
    df.to_parquet(parquet, index=False, coerce_timestamps="ms", allow_truncated_timestamps=True)
    m = json.loads(manifest_path.read_text())
    m["file_sha256"] = hashlib.sha256(parquet.read_bytes()).hexdigest()
    manifest_path.write_text(json.dumps(m))


def test_legacy_dataset_without_new_columns_resolves(tmp_path):
    """Scenario: Legacy Parquet without new columns."""
    p = write_dataset(tmp_path, sample_rows())
    import pandas as pd

    cols = list(pd.read_parquet(p).columns)
    assert not {"item_list_id", "position", "transaction_id"} & set(cols)
    ds = resolve_dataset(Settings(dataset_dir=str(tmp_path)))
    assert ds.path == str(p)
    assert ds.lineage["sha256"] == ds.manifest["file_sha256"]


def test_dataset_with_new_columns_resolves_and_keeps_lineage(tmp_path):
    """Scenario: New columns consumed when present (extra columns must not break resolution)."""
    p = write_dataset(tmp_path, sample_rows())
    _with_extra_columns(p, manifest_path_for(p))
    ds = resolve_dataset(Settings(dataset_dir=str(tmp_path)))
    assert ds.path == str(p)
    assert ds.lineage["sha256"] == hashlib.sha256(p.read_bytes()).hexdigest()
