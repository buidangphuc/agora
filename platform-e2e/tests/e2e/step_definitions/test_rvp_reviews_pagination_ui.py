"""Binds frontend/rvp_reviews_pagination_ui.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.rvp_steps import *  # noqa: F401,F403

scenarios("../features/frontend/rvp_reviews_pagination_ui.feature")
