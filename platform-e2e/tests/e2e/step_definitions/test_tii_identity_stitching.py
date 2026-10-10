"""Binds tracking/identity_stitching.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.tii_stitching_steps import *  # noqa: F401,F403

scenarios("tracking/identity_stitching.feature")
