"""Binds order/return_refund_settlement.feature (payment-refund-model / return-refund-settlement)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.prm_rma_steps import *  # noqa: F401,F403

scenarios("order/return_refund_settlement.feature")
