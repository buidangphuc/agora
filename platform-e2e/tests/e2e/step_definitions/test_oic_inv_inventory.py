"""Binds inventory/oic_inv_inventory_reservations.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.oic_inv_inventory_steps import *  # noqa: F401,F403

scenarios("inventory/oic_inv_inventory_reservations.feature")
