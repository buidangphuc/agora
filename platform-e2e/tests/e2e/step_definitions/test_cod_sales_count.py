"""Binds order/cod_sales_count.feature (order-cod-sales-count)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.cod_sales_count_steps import *  # noqa: F401,F403

scenarios("order/cod_sales_count.feature")
