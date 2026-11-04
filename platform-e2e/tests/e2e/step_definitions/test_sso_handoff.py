from pytest_bdd import scenarios

from tests.e2e.step_definitions.sso_handoff_steps import *  # noqa: F403

scenarios("../features/auth/sso_handoff_api.feature")
scenarios("../features/frontend/sso_handoff_ui.feature")
