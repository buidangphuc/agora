"""Binds frontend/cart_cross_sell_ui.feature (recsys-cart-cross-sell, destructive)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.cart_cross_sell_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403

scenarios("frontend/cart_cross_sell_ui.feature")
