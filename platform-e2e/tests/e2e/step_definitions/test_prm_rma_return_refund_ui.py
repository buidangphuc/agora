"""Binds seller/return_refund_ui.feature (payment-refund-model / seller-return-refund-ui)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.prm_rma_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.prm_rma_ui_steps import *  # noqa: F401,F403

scenarios("seller/return_refund_ui.feature")
