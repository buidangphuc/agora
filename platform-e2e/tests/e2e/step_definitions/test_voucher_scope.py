"""Binds promo/voucher_scope.feature (change promotion-voucher-scope, initiative q11-checkout-coherence)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.voucher_scope_steps import *  # noqa: F401,F403

scenarios("promo/voucher_scope.feature")
