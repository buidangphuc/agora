from pytest_bdd import scenarios

from tests.e2e.step_definitions.welcome_checklist_steps import *  # noqa: F403

scenarios("../features/frontend/welcome_checklist.feature")
