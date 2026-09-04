"""Binds ai/assistant.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.assistant_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403

scenarios("ai/assistant.feature")
