"""Binds analytics/order_facts.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.order_facts_steps import *  # noqa: F401,F403

scenarios("analytics/order_facts.feature")
