"""Binds modelserve/model_serving*.feature (change add-platform-modelserve)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.ms_gitops_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.ms_steps import *  # noqa: F401,F403

scenarios("modelserve/model_serving.feature", "modelserve/model_serving_gitops.feature")
