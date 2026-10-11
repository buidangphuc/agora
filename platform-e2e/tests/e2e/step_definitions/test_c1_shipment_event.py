from pytest_bdd import scenarios

from tests.e2e.step_definitions.adq_cockpit_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.auth_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.c1_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.cockpit_metrics_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.journey_steps import *  # noqa: F401,F403

scenarios("../features/notification/c1_shipment_event.feature")
