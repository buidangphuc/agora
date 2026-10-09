"""Binds frontend/orders_ui.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d2_orders_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d2_orders_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.orders_ui_steps import *  # noqa: F401,F403

scenarios("frontend/orders_ui.feature")
