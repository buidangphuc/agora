"""Binds frontend/seller_getting_started.feature (change seller-getting-started, initiative seller-centre)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_getting_started_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_marketing_hub_steps import *  # noqa: F401,F403

scenarios("frontend/seller_getting_started.feature")
