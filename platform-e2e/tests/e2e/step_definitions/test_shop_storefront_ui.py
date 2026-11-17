"""Binds frontend/shop_storefront_ui.feature (change shop-page-featured-theme, initiative seller-centre)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.shop_storefront_ui_steps import *  # noqa: F401,F403

scenarios("frontend/shop_storefront_ui.feature")
