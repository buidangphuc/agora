"""Binds featurestore/feature_materialization.feature (featurestore-materialization)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.fsm_steps import *  # noqa: F401,F403

scenarios("featurestore/feature_materialization.feature")
