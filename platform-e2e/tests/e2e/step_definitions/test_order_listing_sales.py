"""Binds order/listing_sales.feature (order-listing-sales)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.order_listing_sales_steps import *  # noqa: F401,F403

scenarios("order/listing_sales.feature")
