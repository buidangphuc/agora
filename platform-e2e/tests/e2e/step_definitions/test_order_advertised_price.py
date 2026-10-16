"""Binds order/advertised_price.feature (change order-advertised-price, initiative q5-advertised-price)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.order_advertised_price_steps import *  # noqa: F401,F403

scenarios("order/advertised_price.feature")
