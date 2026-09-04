from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *
from tests.e2e.step_definitions.seller_order_steps import *
from tests.e2e.step_definitions.seller_steps import *

scenarios("../features/seller/seller_order_management.feature")
