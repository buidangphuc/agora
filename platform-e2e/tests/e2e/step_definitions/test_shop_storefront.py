from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *
from tests.e2e.step_definitions.buyer_steps import *
from tests.e2e.step_definitions.shop_storefront_steps import *

scenarios("../features/shop/shop_storefront.feature")
