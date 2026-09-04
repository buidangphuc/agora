"""Binds auth/unauthorized_access.feature to step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.unauthorized_steps import *  # noqa: F401,F403

scenarios("auth/unauthorized_access.feature")
