"""Binds security/edge_route_policy.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.edge_steps import *  # noqa: F401,F403

scenarios("security/edge_route_policy.feature")
