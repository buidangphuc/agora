"""Scenarios: the six NOT-PORTED serving files are never imported by the offline suite, and the
removed local registry / gate modules are gone."""

import importlib.util
import subprocess
import sys

import pytest
from conftest import PROJECT, TESTS_DIR

NOT_PORTED = (
    "datascience.serve.local",
    "datascience.package",
    "datascience.optimize",
    "datascience.serve_analyzer",
    "datascience.serve.triton",
    "datascience.serve.deploy_sagemaker",
)
REMOVED = (
    "datascience.registry.base",
    "datascience.registry.local",
    "datascience.registry.s3",
    "datascience.registry.stages",
    "datascience.gate",
)

_PROBE = """
import sys
import datascience.cli
import datascience.pipeline.stages
import datascience.tune.loop
import datascience.tune.promote
import datascience.perf.cli
bad = [m for m in {mods!r} if m in sys.modules]
print("BAD:" + ",".join(bad))
"""


def test_not_ported_files_not_imported():
    # Scenario: The offline suite does not import the not-ported files
    # (a) modules imported by this pytest process so far
    assert [m for m in NOT_PORTED if m in sys.modules] == []
    # (b) everything the entry points and both register sites pull in
    r = subprocess.run([sys.executable, "-c", _PROBE.format(mods=NOT_PORTED)], capture_output=True,
                       text=True, cwd=PROJECT, check=False, timeout=240)
    assert r.returncode == 0, r.stderr[-2000:]
    assert r.stdout.strip().splitlines()[-1] == "BAD:"
    # (c) no test module imports them either
    for f in TESTS_DIR.glob("*.py"):
        if f.name == "test_import_hygiene.py":
            continue
        text = f.read_text()
        for mod in NOT_PORTED:
            assert f"import {mod}" not in text and f"from {mod} " not in text, (f.name, mod)


@pytest.mark.parametrize("module", REMOVED)
def test_removed_modules_are_gone(module):
    # Scenario: Both register sites write to the registry at runtime (no local registry or gate remains)
    try:
        spec = importlib.util.find_spec(module)
    except ModuleNotFoundError:  # parent package removed as well
        spec = None
    assert spec is None
