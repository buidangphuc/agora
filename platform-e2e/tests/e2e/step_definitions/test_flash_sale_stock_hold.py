"""Binds promo/flash_sale_stock_hold.feature (change promotion-flash-stock-hold, initiative q5-advertised-price)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.flash_sale_stock_hold_steps import *  # noqa: F401,F403

scenarios("promo/flash_sale_stock_hold.feature")
