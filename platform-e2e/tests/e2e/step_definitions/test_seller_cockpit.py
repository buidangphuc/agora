"""Binds seller/seller_cockpit.feature (OpenSpec change ui-phase-seller)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_cockpit_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_order_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.shop_display_name_steps import *  # noqa: F401,F403

scenarios("seller/seller_cockpit.feature")
