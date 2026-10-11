"""Binds analytics/seller_funnel_scope.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_funnel_scope_steps import *  # noqa: F401,F403

scenarios("analytics/seller_funnel_scope.feature")
