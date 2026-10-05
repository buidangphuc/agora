from pytest_bdd import scenarios

from tests.e2e.step_definitions.session_revocation_steps import *  # noqa: F403

scenarios("../features/auth/session_revocation.feature")
