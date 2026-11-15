"""Binds admin/insights_panels.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.admin_insights_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403

scenarios("../features/admin/insights_panels.feature")
