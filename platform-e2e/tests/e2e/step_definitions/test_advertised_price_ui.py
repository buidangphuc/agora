"""Binds frontend/advertised_price_ui.feature (change frontend-advertised-price, initiative q5-advertised-price)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.advertised_price_ui_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.cart_checkout_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d2_cart_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d2_checkout_steps import *  # noqa: F401,F403

scenarios("frontend/advertised_price_ui.feature")
