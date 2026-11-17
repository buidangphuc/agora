"""Binds frontend/seller_fee_breakdown_ui.feature (seller-fee-breakdown-ui)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.seller_fee_breakdown_ui_steps import *  # noqa: F401,F403

scenarios("frontend/seller_fee_breakdown_ui.feature")
