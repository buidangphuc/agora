"""Binds notification/alert_delivery.feature.

These scenarios drive the real async alert-delivery flow (subscribe -> seller
changes price/stock -> notification appears). team-notification self-diffs each
platform.listing.v1.ListingChanged snapshot against the last price/stock it saw,
so it needs the listing's events in creation order (the outbox relayers keep
that order within a claimed batch).
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


@scenario(
    "notification/alert_delivery.feature",
    "Two buyers do not see each other's notifications",
)
def test_notifications_are_per_user() -> None:
    pass
