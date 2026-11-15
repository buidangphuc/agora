"""Binds seller/seller_forecast_card.feature (seller-forecast-card)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.ai_interaction_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_analytics_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_forecast_card_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_steps import *  # noqa: F401,F403

scenarios("seller/seller_forecast_card.feature")
