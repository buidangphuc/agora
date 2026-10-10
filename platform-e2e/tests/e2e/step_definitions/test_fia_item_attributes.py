"""Binds featurestore/item_attributes.feature (featurestore-item-attributes)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.fia_steps import *  # noqa: F401,F403

scenarios("featurestore/item_attributes.feature")
