"""Binds the destructive home_feed scenario of recommendations/placement_engine.feature.

The scenario publishes a fixture generation that includes the buyer, so the module republishes a REAL
generation afterwards (the production path, as recsys-generation-publish does).
"""

import os

import pytest
from pytest_bdd import scenario

from src.api.recsys_job import restore_real_generation
from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.ple_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.rgp_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.tpr_steps import *  # noqa: F401,F403
from tests.e2e.support.pear_edge_support import DC_WRAPPER_DEFAULT

_FEATURE = "recommendations/placement_engine.feature"


@pytest.fixture(scope="module", autouse=True)
def ple_restore_serving():
    """Leave a REAL generation serving after the module, not an e2e fixture.

    The registry is emptied first (no forcing lever exists), see restore_real_generation."""
    yield
    restore_real_generation(os.getenv("DC_WRAPPER", DC_WRAPPER_DEFAULT))


@scenario(_FEATURE, "Home feed surfaces personalized recommendations with popularity fallback")
def test_home_feed_placement() -> None:
    pass
