"""Binds identity/data_export.feature (change identity-data-export, capability identity-data-export)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.data_export_steps import *  # noqa: F401,F403

scenarios("identity/data_export.feature")
