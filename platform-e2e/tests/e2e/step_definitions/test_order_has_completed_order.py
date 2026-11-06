from pytest_bdd import scenarios

from tests.e2e.step_definitions.order_q7_steps import *  # noqa: F401,F403

scenarios("order/has_completed_order.feature")
