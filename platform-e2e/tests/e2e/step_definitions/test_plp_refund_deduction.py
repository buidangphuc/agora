"""Binds payment/plp_refund_deduction.feature (port-payment-ledger-integrity, modified by payment-refund-model / seller-refund-deduction)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.plp_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.prm_pay_steps import *  # noqa: F401,F403

scenarios("payment/plp_refund_deduction.feature")
