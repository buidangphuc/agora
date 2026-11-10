"""Shared fixtures. `reg` = local MLflow file store (tests/registry_fixture.py)."""

import os

os.environ.setdefault("MLFLOW_ALLOW_FILE_STORE", "true")

from tests.registry_fixture import reg  # noqa: F401

import pytest  # noqa: E402


@pytest.fixture(autouse=True)
def _default_item_attributes_snapshot(monkeypatch):
    from tests.pipeline_fixture import default_attributes_dir

    monkeypatch.setenv("ITEM_ATTRIBUTES_V2_DIR", default_attributes_dir())
