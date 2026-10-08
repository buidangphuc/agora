"""Binds the ai-auth port-security-hardening features to their step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.ai_auth_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403

scenarios("../features/security/ai_access.feature")
