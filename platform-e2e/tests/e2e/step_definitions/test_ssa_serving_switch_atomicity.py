"""Binds recommendations/serving_switch_atomicity.feature (serving-switch-atomicity)."""

import os
import subprocess

import pytest
from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.ssa_steps import *  # noqa: F401,F403
from tests.e2e.support.pear_edge_support import DC_WRAPPER_DEFAULT

scenarios("recommendations/serving_switch_atomicity.feature")


@pytest.fixture(scope="module", autouse=True)
def ssa_restore_serving():
    """Leave a REAL generation serving after the module (same production path as the
    recsys-generation-publish teardown), so no fixture model or edited alias leaks into other
    recommendation scenarios."""
    yield
    wrapper = os.getenv("DC_WRAPPER", DC_WRAPPER_DEFAULT)
    outputs = []
    for args in (
        ["--profile", "featurestore", "run", "--rm", "featurestore-dataset"],
        ["--profile", "jobs", "run", "--rm", "-e", "PROMOTION_FORCE=true", "platform-recsys"],
    ):
        proc = subprocess.run(
            [wrapper, *args], capture_output=True, text=True, timeout=1200, check=False
        )
        outputs.append(proc.stdout + proc.stderr)
        assert proc.returncode == 0, f"[ssa] restore step {args[-1]} failed:\n{outputs[-1][-3000:]}"
    assert "'decision': 'promoted'" in outputs[-1], (
        f"[ssa] the real generation was not promoted; a fixture model may still be serving:\n"
        f"{outputs[-1][-3000:]}"
    )
