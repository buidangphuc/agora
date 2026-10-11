"""Binds payment/plp_settlement_credit.feature (port-payment-ledger-integrity / seller-settlement-credit)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.plp_steps import *  # noqa: F401,F403

scenarios("payment/plp_settlement_credit.feature")
