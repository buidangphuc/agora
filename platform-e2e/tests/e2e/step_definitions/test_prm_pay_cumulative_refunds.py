"""Binds payment/cumulative_refunds.feature (payment-refund-model / payment-cumulative-refunds)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.plp_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.prm_pay_steps import *  # noqa: F401,F403

scenarios("payment/cumulative_refunds.feature")
