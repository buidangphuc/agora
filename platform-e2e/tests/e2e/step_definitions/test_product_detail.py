"""Binds frontend/product_detail.feature (ui-phase-product-detail)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.product_detail_steps import *  # noqa: F401,F403

scenarios("../features/frontend/product_detail.feature")
