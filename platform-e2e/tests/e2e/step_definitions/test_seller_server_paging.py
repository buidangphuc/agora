"""Binds seller/seller_server_paging.feature (change seller-server-paging-ui, initiative seller-centre)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_server_paging_steps import *  # noqa: F401,F403

scenarios("seller/seller_server_paging.feature")
