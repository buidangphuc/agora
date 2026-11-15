"""Binds verification/kyc_documents.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.kyc_documents_steps import *  # noqa: F401,F403

scenarios("verification/kyc_documents.feature")
