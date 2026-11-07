"""Shared fixtures. `reg` = local MLflow file store (tests/registry_fixture.py)."""

import os

os.environ.setdefault("MLFLOW_ALLOW_FILE_STORE", "true")

from tests.registry_fixture import reg  # noqa: F401
