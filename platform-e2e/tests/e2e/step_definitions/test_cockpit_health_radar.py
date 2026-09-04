from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *
from tests.e2e.step_definitions.cockpit_health_steps import *

scenarios("../features/admin/cockpit_health_radar.feature")
