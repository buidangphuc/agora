"""Binds seller/seller_plans.feature (seller-plan-ui)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_plan_billing_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_plan_ui_steps import *  # noqa: F401,F403

scenarios("seller/seller_plans.feature")
