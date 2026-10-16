from pytest_bdd import scenarios

from tests.e2e.step_definitions.login_return_steps import *  # noqa: F403

scenarios("../features/auth/login_return.feature")
