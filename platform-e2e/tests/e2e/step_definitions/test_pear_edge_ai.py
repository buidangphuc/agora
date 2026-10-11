"""Binds security/edge_ai_and_reflection.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.pear_edge_ai_steps import *  # noqa: F401,F403

scenarios("../features/security/edge_ai_and_reflection.feature")
