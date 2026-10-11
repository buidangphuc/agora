"""Binds ops/boot_guard_search.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.visibility_boot_guard_steps import *  # noqa: F401,F403

scenarios("ops/boot_guard_search.feature")
