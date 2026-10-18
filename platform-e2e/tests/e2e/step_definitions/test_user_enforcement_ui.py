"""Binds frontend/user_enforcement_ui.feature (change frontend-user-enforcement, initiative q6-trust-enforcement)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.user_enforcement_steps import *  # noqa: F401,F403

scenarios("frontend/user_enforcement_ui.feature")
