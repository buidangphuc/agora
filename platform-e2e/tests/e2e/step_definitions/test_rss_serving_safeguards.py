"""Binds recommendations/serving_safeguards.feature (recs-serving-safeguards)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.rss_steps import *  # noqa: F401,F403

scenarios("../features/recommendations/serving_safeguards.feature")
