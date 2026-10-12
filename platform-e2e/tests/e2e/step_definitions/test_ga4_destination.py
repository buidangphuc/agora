"""Binds tracking/ga4_destination.feature (add-ga4-destination, capability tracking)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.consent_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.ga4_datalayer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.ga4_destination_steps import *  # noqa: F401,F403

scenarios("tracking/ga4_destination.feature")
