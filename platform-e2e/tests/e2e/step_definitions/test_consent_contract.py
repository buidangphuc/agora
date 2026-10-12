"""Binds tracking/consent_contract.feature (ga4-consent-contract, capability tracking-consent)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.consent_contract_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_analytics_access_steps import *  # noqa: F401,F403

scenarios("tracking/consent_contract.feature")
