"""Binds frontend/ui_seller_orders_money.feature (OpenSpec change ui-phase-seller)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d2_seller_steps import *  # noqa: F401,F403

scenarios("frontend/ui_seller_orders_money.feature")
