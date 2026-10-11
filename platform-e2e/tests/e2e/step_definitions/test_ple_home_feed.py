"""Binds the destructive home_feed scenario of recommendations/placement_engine.feature.

The scenario publishes a fixture generation that includes the buyer, so the module republishes a REAL
generation afterwards (the production path, as recsys-generation-publish does).
"""

import os
import subprocess

import pytest
from pytest_bdd import scenario

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.ple_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.rgp_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.tpr_steps import *  # noqa: F401,F403
from tests.e2e.support.pear_edge_support import DC_WRAPPER_DEFAULT

_FEATURE = "recommendations/placement_engine.feature"


@pytest.fixture(scope="module", autouse=True)
def ple_restore_serving():
    """Leave a REAL generation serving after the module, not an e2e fixture."""
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
        assert proc.returncode == 0, f"[ple] restore step {args[-1]} failed:\n{outputs[-1][-3000:]}"
    assert "'decision': 'promoted'" in outputs[-1], (
        f"[ple] the real generation was not promoted; a fixture model may still be serving:\n"
        f"{outputs[-1][-3000:]}"
    )


@scenario(_FEATURE, "Home feed surfaces personalized recommendations with popularity fallback")
def test_home_feed_placement() -> None:
    pass
