"""Binds promo/promotion_admin_vouchers.feature (change promotion-admin-vouchers, initiative p10-backend-gaps)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.promotion_admin_vouchers_steps import *  # noqa: F401,F403

scenarios("promo/promotion_admin_vouchers.feature")
