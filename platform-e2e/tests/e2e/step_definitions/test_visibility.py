"""Binds security/listing_visibility.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.visibility_steps import *  # noqa: F401,F403

scenarios("security/listing_visibility.feature")
