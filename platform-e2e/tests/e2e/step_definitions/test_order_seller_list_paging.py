"""Binds order/seller_list_paging.feature (change order-seller-list-paging, initiative seller-centre)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.order_seller_list_paging_steps import *  # noqa: F401,F403

scenarios("order/seller_list_paging.feature")
