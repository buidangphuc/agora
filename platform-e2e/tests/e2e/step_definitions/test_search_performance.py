from pytest_bdd import scenarios

from tests.e2e.step_definitions.search_attribution_steps import *  # noqa: F401,F403

scenarios("analytics/search_performance.feature")
