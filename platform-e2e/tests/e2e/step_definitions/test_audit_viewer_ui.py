"""Binds frontend/audit_viewer_ui.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.audit_viewer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403

scenarios("frontend/audit_viewer_ui.feature")
