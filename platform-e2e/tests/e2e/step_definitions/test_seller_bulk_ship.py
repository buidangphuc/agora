"""Binds frontend/seller_bulk_ship.feature (change seller-bulk-ship, initiative seller-centre)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_bulk_ship_steps import *  # noqa: F401,F403

scenarios("frontend/seller_bulk_ship.feature")
