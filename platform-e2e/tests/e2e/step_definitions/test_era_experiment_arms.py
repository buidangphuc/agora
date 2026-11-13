"""Binds recommendations/experiment_arms.feature (recs-experiment-arms)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.era_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.in_stock_recommendations_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.rss_steps import *  # noqa: F401,F403

scenarios("../features/recommendations/experiment_arms.feature")
