from pytest_bdd import scenarios

from tests.e2e.step_definitions.dispute_cases_steps import *  # noqa: F401,F403

scenarios("frontend/dispute_cases_ui.feature")
