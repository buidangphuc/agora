"""Binds order/order_events_published.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.group_a_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.order_events_steps import *  # noqa: F401,F403

scenarios("order/order_events_published.feature")
