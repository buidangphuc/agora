"""Binds frontend/account.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.account_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403

scenarios("frontend/account.feature")
