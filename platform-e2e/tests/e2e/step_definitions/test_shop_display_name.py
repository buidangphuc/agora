from pytest_bdd import scenarios

from tests.e2e.step_definitions.c2_shop_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.cart_checkout_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.shop_display_name_steps import *  # noqa: F401,F403

scenarios("../features/shop/shop_display_name.feature")
