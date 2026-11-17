"""Binds promotion/seller_plan_billing.feature (promotion-plan-billing)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.seller_fee_charge_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_plan_billing_steps import *  # noqa: F401,F403

scenarios("promotion/seller_plan_billing.feature")
