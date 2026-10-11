"""Governed dataset resolution, refusal and lineage (PySpark-free)."""

import logging

import pytest

from recsys import __main__ as entry
from recsys.config import ConfigError, Settings
from recsys.dataset import manifest_path_for, resolve_dataset
from tests.dataset_fixture import sample_rows, write_dataset


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


def test_main_exits_2_naming_dataset_dir_and_registers_nothing(tmp_path, monkeypatch, caplog):
    import recsys.pipeline as pipeline

    monkeypatch.setenv("DATASET_DIR", str(tmp_path))
    monkeypatch.delenv("DATASET_PATH", raising=False)
    # Must refuse before any Spark or registry work.
    monkeypatch.setattr(pipeline, "build_spark", lambda *_: pytest.fail("Spark started without a dataset"))
    monkeypatch.setattr(
        pipeline, "ModelRegistry", lambda *_a, **_k: pytest.fail("registry touched without a dataset")
    )
    with caplog.at_level(logging.ERROR):
        assert entry.main([]) == 2
    assert f"DATASET_DIR={tmp_path}" in caplog.text
