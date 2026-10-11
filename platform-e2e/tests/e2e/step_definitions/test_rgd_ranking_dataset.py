"""Binds featurestore/ranking_dataset.feature (recsys-gbdt-trainer)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.rgd_steps import *  # noqa: F401,F403

scenarios("featurestore/ranking_dataset.feature")
