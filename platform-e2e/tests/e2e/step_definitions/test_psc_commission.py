"""Binds payment/psc_commission.feature (payment-settlement-commission)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.psc_steps import *  # noqa: F401,F403

scenarios("payment/psc_commission.feature")
