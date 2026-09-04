from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *
from tests.e2e.step_definitions.buyer_steps import *
from tests.e2e.step_definitions.order_timeline_steps import *

scenarios("../features/order/order_timeline_view.feature")
