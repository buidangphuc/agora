"""Binds buyer/cart_checkout.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.cart_checkout_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.cart_management_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.promo_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.shop_display_name_steps import *  # noqa: F401,F403

scenarios("buyer/cart_checkout.feature")
