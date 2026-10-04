"""Binds tracking/discovery_tracking_unchanged.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.discovery_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.search_sort_steps import *  # noqa: F401,F403

scenarios("tracking/discovery_tracking_unchanged.feature")
