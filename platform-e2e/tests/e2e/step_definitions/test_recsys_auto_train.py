"""Binds recommendations/recsys_auto_train.feature (change recsys-auto-train, destructive)."""

import os

import pytest
from pytest_bdd import scenarios

from src.api.recsys_job import restore_real_generation
from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.recsys_auto_train_steps import *  # noqa: F401,F403
from tests.e2e.support.pear_edge_support import DC_WRAPPER_DEFAULT

scenarios("recommendations/recsys_auto_train.feature")


@pytest.fixture(scope="module", autouse=True)
def rat_restore_serving():
    """Leave a REAL generation serving after the module, not an e2e fixture generation.

    The scenarios reset the shared generation state and the recsys-* registry; the teardown runs
    the production path (dataset job, then the recsys job, no force) from an empty registry.
    """
    yield
    restore_real_generation(os.getenv("DC_WRAPPER", DC_WRAPPER_DEFAULT))
