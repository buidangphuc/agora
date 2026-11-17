"""Binds frontend/seller_ads_ui.feature (seller-ads-ui)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.seller_ads_ui_steps import *  # noqa: F401,F403

scenarios("frontend/seller_ads_ui.feature")
