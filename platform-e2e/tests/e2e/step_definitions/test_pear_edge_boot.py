"""Binds ops/boot_guard_gateway.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.pear_edge_boot_steps import *  # noqa: F401,F403

scenarios("../features/ops/boot_guard_gateway.feature")
