"""Binds frontend/search_rating_removed.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.discovery_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.search_sort_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.srm_frontend_steps import *  # noqa: F401,F403

scenarios("frontend/search_rating_removed.feature")
