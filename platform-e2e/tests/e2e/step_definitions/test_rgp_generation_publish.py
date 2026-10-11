"""Binds recommendations/generation_publish.feature (recsys-generation-publish, destructive)."""

import os
import subprocess

import pytest
from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.rgp_steps import *  # noqa: F401,F403
from tests.e2e.support.pear_edge_support import DC_WRAPPER_DEFAULT

scenarios("recommendations/generation_publish.feature")


@pytest.fixture(scope="module", autouse=True)
def rgp_restore_serving():
    """Leave a REAL generation serving after the module, not an e2e fixture.

    The scenarios reset the shared generation state and leave a fixture model as serving and as
    registry champion; every other recommendation scenario (homepage row, Recommend for stack
    buyers) then sees fixture listing ids. Teardown runs the production path instead: build the
    governed dataset from the stack's own exports, then train and publish it with
    PROMOTION_FORCE (the fixture champion's holdout score would otherwise reject it).
    """
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
        # A failed restore is reported as a teardown ERROR (pytest keeps it apart from the
        # scenarios' own result) instead of silently leaving a fixture generation serving.
        assert proc.returncode == 0, f"[rgp] restore step {args[-1]} failed:\n{outputs[-1][-3000:]}"
    # PROMOTION_FORCE skips only the metric gate; the structural gate can still reject the real
    # data, and the job then exits 0 with decision "rejected".
    assert "'decision': 'promoted'" in outputs[-1], (
        f"[rgp] the real generation was not promoted; a fixture model may still be serving:\n"
        f"{outputs[-1][-3000:]}"
    )
