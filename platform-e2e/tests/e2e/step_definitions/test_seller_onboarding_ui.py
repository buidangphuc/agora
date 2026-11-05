"""Binds frontend/seller_onboarding_ui.feature (change seller-onboarding-ui, initiative p10-backend-gaps)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_onboarding_ui_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.user_enforcement_steps import *  # noqa: F401,F403

scenarios("frontend/seller_onboarding_ui.feature")
