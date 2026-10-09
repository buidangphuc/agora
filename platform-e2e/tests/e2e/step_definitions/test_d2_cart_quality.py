"""Binds frontend/ui_cart_quality.feature (OpenSpec change ui-phase-cart-checkout)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d2_cart_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d2_quality_steps import *  # noqa: F401,F403

scenarios("frontend/ui_cart_quality.feature")
