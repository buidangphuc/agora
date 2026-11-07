"""Binds recommendations/candidate_attributes.feature (recsys-candidate-attributes, destructive)."""

import os

import pytest
from pytest_bdd import scenarios

from src.api.recsys_job import restore_real_generation
from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.candidate_attributes_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.support.pear_edge_support import DC_WRAPPER_DEFAULT

scenarios("recommendations/candidate_attributes.feature")


@pytest.fixture(scope="module", autouse=True)
def cattr_restore_serving():
    yield
    restore_real_generation(os.getenv("DC_WRAPPER", DC_WRAPPER_DEFAULT))
