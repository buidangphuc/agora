"""Binds ai_and_cockpit/ai_and_cockpit.feature to step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.ai_and_cockpit_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403

scenarios("ai_and_cockpit/ai_and_cockpit.feature")
