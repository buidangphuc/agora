"""Binds frontend/legal_pages.feature (change frontend-legal-pages)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.frontend_legal_steps import *  # noqa: F401,F403

scenarios("frontend/legal_pages.feature")
