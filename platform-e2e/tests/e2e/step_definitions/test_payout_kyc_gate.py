"""Binds payment/payout_kyc_gate.feature (change payment-payout-kyc-gate)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.payout_kyc_gate_steps import *  # noqa: F401,F403

scenarios("payment/payout_kyc_gate.feature")
