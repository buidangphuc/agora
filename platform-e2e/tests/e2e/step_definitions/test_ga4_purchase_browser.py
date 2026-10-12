"""Binds tracking/ga4_purchase_browser.feature (add-ga4-purchase-relay, capability ga4-purchase-relay)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.cart_checkout_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d2_cart_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d2_checkout_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.ga4_datalayer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.ga4_purchase_browser_steps import *  # noqa: F401,F403

scenarios("tracking/ga4_purchase_browser.feature")
