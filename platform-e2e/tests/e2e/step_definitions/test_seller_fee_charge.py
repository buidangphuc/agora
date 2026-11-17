"""Binds payment/seller_fee_charge.feature (payment-seller-fee-charge)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.seller_fee_charge_steps import *  # noqa: F401,F403

scenarios("payment/seller_fee_charge.feature")
