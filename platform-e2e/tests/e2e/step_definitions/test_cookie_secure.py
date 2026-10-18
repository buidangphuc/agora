from pytest_bdd import scenarios

from tests.e2e.step_definitions.cookie_secure_steps import *  # noqa: F403
from tests.e2e.step_definitions.session_continuity_steps import *  # noqa: F403

scenarios("../features/auth/cookie_secure.feature")
