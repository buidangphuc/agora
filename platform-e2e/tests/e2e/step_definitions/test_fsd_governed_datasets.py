"""Binds featurestore/governed_datasets.feature (featurestore-datasets)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.fsd_steps import *  # noqa: F401,F403

scenarios("featurestore/governed_datasets.feature")
