"""Binds seller/fulfillment_and_payout.feature to step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_steps import *  # noqa: F401,F403

scenarios("seller/fulfillment_and_payout.feature")
