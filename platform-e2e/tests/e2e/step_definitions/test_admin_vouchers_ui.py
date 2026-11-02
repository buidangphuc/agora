"""Binds frontend/admin_vouchers_ui.feature (change promotion-admin-vouchers, initiative p10-backend-gaps)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.admin_vouchers_ui_steps import *  # noqa: F401,F403

scenarios("frontend/admin_vouchers_ui.feature")
