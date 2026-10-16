from pytest_bdd import scenarios

from tests.e2e.step_definitions.session_continuity_steps import *  # noqa: F403

scenarios("../features/auth/session_continuity.feature")
