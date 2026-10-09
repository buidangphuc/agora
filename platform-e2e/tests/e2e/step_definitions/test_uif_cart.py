"""Binds frontend/uif_cart.feature (real service outage, destructive)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d2_cart_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.uif_cart_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.uif_steps import *  # noqa: F401,F403

scenarios("frontend/uif_cart.feature")
