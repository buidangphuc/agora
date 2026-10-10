"""Binds recommendations/gbl_ranker_serving.feature (recsys-gbdt-trainer)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.gbl_steps import *  # noqa: F401,F403

scenarios("../features/recommendations/gbl_ranker_serving.feature")
