from pytest_bdd import scenarios

from tests.e2e.step_definitions.discovery_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.search_sort_steps import *  # noqa: F401,F403

scenarios("../features/frontend/search_facets.feature")
