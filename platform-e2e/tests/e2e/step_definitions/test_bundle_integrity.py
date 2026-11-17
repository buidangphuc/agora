"""Binds seller/bundle_integrity.feature (domain-bundle-integrity)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.bundle_integrity_steps import *  # noqa: F401,F403

scenarios("seller/bundle_integrity.feature")
