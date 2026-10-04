from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F403
from tests.e2e.step_definitions.group_c_steps import *  # noqa: F403
from tests.e2e.step_definitions.seller_cockpit_steps import *  # noqa: F401,F403

scenarios("seller/listing_delete.feature")
