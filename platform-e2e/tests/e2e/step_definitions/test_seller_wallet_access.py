"""Binds payment/seller_wallet_access.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_analytics_access_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_wallet_access_steps import *  # noqa: F401,F403

scenarios("payment/seller_wallet_access.feature")
