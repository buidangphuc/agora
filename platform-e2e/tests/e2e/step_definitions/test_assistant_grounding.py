"""Binds ai/assistant_grounding.feature (change assistant-rag-grounding)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.agr_steps import *  # noqa: F401,F403

scenarios("../features/ai/assistant_grounding.feature")
