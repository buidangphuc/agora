"""Binds frontend/shop_bundle_purchase_ui.feature (change shop-bundle-purchase-ui, initiative seller-centre)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.shop_bundle_purchase_ui_steps import *  # noqa: F401,F403

scenarios("frontend/shop_bundle_purchase_ui.feature")
