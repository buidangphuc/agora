"""Binds buyer/order_tracking_and_rma.feature to step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.order_steps import *  # noqa: F401,F403

scenarios("buyer/order_tracking_and_rma.feature")
