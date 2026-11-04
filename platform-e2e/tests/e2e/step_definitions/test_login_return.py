from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.login_return_steps import *  # noqa: F403

scenarios("../features/auth/login_return.feature")
