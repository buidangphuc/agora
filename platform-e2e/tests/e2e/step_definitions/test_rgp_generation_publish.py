"""Binds recommendations/generation_publish.feature (recsys-generation-publish, destructive)."""

import os

import pytest
from pytest_bdd import scenarios

from src.api.recsys_job import restore_real_generation
from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.rgp_steps import *  # noqa: F401,F403
from tests.e2e.support.pear_edge_support import DC_WRAPPER_DEFAULT

scenarios("recommendations/generation_publish.feature")


@pytest.fixture(scope="module", autouse=True)
def rgp_restore_serving():
    """Leave a REAL generation serving after the module, not an e2e fixture.

    The scenarios reset the shared generation state and leave a fixture model as serving and as
    `recsys-als` champion; every other recommendation scenario (homepage row, Recommend for stack
    buyers) then sees fixture listing ids. Teardown runs the production path instead: build the
    governed dataset from the stack's own exports, then train and publish it. The nightly job is
    denied a force promotion (recsys-auto-train), so the `recsys-*` registry is emptied first and
    the real candidate is promotable as the first one (src/api/recsys_job.restore_real_generation).
    """
    yield
    restore_real_generation(os.getenv("DC_WRAPPER", DC_WRAPPER_DEFAULT))
