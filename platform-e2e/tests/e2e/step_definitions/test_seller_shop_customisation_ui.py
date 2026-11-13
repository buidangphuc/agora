"""Binds frontend/seller_shop_customisation_ui.feature (change seller-shop-customisation-ui, initiative seller-centre)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_shop_customisation_ui_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_steps import *  # noqa: F401,F403

scenarios("frontend/seller_shop_customisation_ui.feature")
