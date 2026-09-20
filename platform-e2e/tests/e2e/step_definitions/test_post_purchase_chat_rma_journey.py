"""Binds journeys/post_purchase_chat_rma_journey.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.journey_steps import *  # noqa: F401,F403

scenarios("journeys/post_purchase_chat_rma_journey.feature")
