"""Binds ops/upstream_recovery.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.pear_edge_recovery_steps import *  # noqa: F401,F403

scenarios("../features/ops/upstream_recovery.feature")
