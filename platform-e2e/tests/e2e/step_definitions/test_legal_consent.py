from pytest_bdd import scenarios

from tests.e2e.step_definitions.legal_consent_steps import *  # noqa: F403

scenarios("identity/legal_consent.feature")
