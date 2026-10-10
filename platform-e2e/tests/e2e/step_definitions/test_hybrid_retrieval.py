"""Binds search/hybrid_retrieval.feature and search/hybrid_indexing.feature to step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.hrp_steps import *  # noqa: F401,F403

scenarios("search/hybrid_retrieval.feature", "search/hybrid_indexing.feature")
