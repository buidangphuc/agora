"""Binds security/oic_inv_edge_listing_commit.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.oic_inv_edge_steps import *  # noqa: F401,F403

scenarios("security/oic_inv_edge_listing_commit.feature")
