"""Binds notification/alert_delivery.feature.

These scenarios drive the real async alert-delivery flow (subscribe -> seller
changes price/stock -> notification appears). The flow is currently broken in the
backend: team-domain publishes platform.listing.v1.ListingChanged on
listing.events, while team-notification's consumer only reacts to
ListingPricingChanged / ListingStockChanged (which nothing produces), so no
notification is ever created. The scenarios are therefore marked xfail — they
document the expected behaviour and self-heal (xpass) once the producer/consumer
contract is reconciled.
"""

from pytest_bdd import scenario

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.notification_alert_steps import *  # noqa: F401,F403


@scenario(
    "notification/alert_delivery.feature",
    "Price-drop notification after the seller lowers the price",
)
def test_price_drop_delivery() -> None:
    pass


@scenario(
    "notification/alert_delivery.feature",
    "Back-in-stock notification after the seller restocks",
)
def test_back_in_stock_delivery() -> None:
    pass
