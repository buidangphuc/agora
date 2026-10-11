"""Binds frontend/uif_account.feature (real service outages, destructive)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.uif_account_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.uif_steps import *  # noqa: F401,F403

scenarios("frontend/uif_account.feature")
