"""Binds frontend/ui_token_gates.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.d1_gates_steps import *  # noqa: F401,F403

scenarios("frontend/ui_token_gates.feature")
