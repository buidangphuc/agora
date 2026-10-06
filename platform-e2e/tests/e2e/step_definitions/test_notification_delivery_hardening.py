from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.journey_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.notification_alert_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.notification_hardening_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.shop_display_name_steps import *  # noqa: F401,F403

scenarios("../features/notification/delivery_hardening.feature")
