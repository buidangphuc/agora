"""Binds recommendations/serving_switch_atomicity.feature (serving-switch-atomicity)."""

import os

import pytest
from pytest_bdd import scenarios

from src.api.recsys_job import restore_real_generation
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
    restore_real_generation(os.getenv("DC_WRAPPER", DC_WRAPPER_DEFAULT))
