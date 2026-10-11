"""Binds payment/plp_payout_holdback.feature (port-payment-ledger-integrity / seller-payout-holdback)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.plp_steps import *  # noqa: F401,F403

scenarios("payment/plp_payout_holdback.feature")
