"""Binds seller/kyc_payout_notice.feature (change seller-kyc-ui)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_cockpit_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_kyc_ui_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_steps import *  # noqa: F401,F403

scenarios("seller/kyc_payout_notice.feature")
