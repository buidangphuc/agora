"""Binds promo/promotion_admin_campaigns.feature (change promotion-admin-campaigns, initiative p10-backend-gaps)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.promotion_admin_campaigns_steps import *  # noqa: F401,F403

scenarios("promo/promotion_admin_campaigns.feature")
