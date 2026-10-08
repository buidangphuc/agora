"""Binds ops/boot_guard_payment.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.commerce_steps import *  # noqa: F401,F403

scenarios("ops/boot_guard_payment.feature")
