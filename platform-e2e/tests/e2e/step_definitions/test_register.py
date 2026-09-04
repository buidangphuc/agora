"""Binds auth/register.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *  # noqa: F401,F403 (reuses "lands on home")
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.register_steps import *  # noqa: F401,F403

scenarios("auth/register.feature")
