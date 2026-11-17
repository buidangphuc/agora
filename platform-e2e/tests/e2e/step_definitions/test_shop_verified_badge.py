"""Binds buyer/shop_verified_badge.feature (change seller-kyc-ui)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_kyc_ui_steps import *  # noqa: F401,F403

scenarios("buyer/shop_verified_badge.feature")
