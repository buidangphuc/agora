"""Binds frontend/ui_checkout_order.feature (OpenSpec change ui-phase-cart-checkout)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.cart_checkout_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d2_cart_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d2_checkout_steps import *  # noqa: F401,F403

scenarios("frontend/ui_checkout_order.feature")
