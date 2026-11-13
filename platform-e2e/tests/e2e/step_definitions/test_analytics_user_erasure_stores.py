from pytest_bdd import scenarios

from tests.e2e.step_definitions.analytics_erasure_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.oic_order_steps import oic  # noqa: F401

scenarios("analytics/user_erasure_stores.feature")
