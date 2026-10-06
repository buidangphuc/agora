from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *
from tests.e2e.step_definitions.seller_order_steps import *
from tests.e2e.step_definitions.seller_steps import *
from tests.e2e.step_definitions.seller_cockpit_steps import *  # noqa: F401,F403

scenarios("../features/seller/seller_order_management.feature")
