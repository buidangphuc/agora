"""Binds promotion/seller_erasure_monetization.feature (change promotion-seller-erasure-monetization)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.seller_erasure_monetization_steps import *  # noqa: F401,F403

scenarios("promotion/seller_erasure_monetization.feature")
