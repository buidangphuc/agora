"""Binds frontend/account_ui.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.account_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.auth_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d1_account_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d1_common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.notification_alert_steps import *  # noqa: F401,F403

scenarios("frontend/account_ui.feature")
