"""Binds frontend/product_detail_ui.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d1_common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d1_pdp_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.pdp_shop_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.promo_steps import *  # noqa: F401,F403

scenarios("frontend/product_detail_ui.feature")
