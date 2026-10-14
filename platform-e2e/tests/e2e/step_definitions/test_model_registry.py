"""Binds model_registry/model_registry.feature (change add-model-registry)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.model_registry_steps import *  # noqa: F401,F403

scenarios("model_registry/model_registry.feature")
