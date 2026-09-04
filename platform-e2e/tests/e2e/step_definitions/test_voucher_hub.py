"""Binds promo/voucher_hub.feature to step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.voucher_hub_steps import *  # noqa: F401,F403

scenarios("promo/voucher_hub.feature")
