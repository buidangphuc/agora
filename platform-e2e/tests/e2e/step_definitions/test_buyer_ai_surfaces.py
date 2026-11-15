"""Binds frontend/buyer_ai_surfaces.feature (change buyer-ai-surfaces-ui)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.ai_interaction_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.buyer_ai_surfaces_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_steps import *  # noqa: F401,F403

scenarios("frontend/buyer_ai_surfaces.feature")
