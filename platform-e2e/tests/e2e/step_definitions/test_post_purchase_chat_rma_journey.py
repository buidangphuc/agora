"""Binds journeys/post_purchase_chat_rma_journey.feature to its step definitions.

The first scenario runs green against the real stack. The two notification
scenarios are bound explicitly and marked xfail(strict) on documented backend gaps:
  * a seller's chat reply creates no CHAT notification: team-chat only publishes
    chat.events for the gateway SSE edge (team-chat/internal/handler/chat.go:265)
    and team-notification consumes only listing.events (cmd/server/main.go:57);
  * shipping an order creates no ORDER notification: team-order emits only
    OrderPaidEvent (internal/events/publisher.go:19).
strict=True so the marker is removed (XPASS fails) once the backend implements them.
"""

import pytest
from pytest_bdd import scenario

from tests.e2e.step_definitions.auth_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.journey_steps import *  # noqa: F401,F403

_FEATURE = "journeys/post_purchase_chat_rma_journey.feature"


@scenario(
    _FEATURE,
    "Post-purchase buyer seller chat, order tracking, and RMA return flow",
)
def test_post_purchase_journey() -> None:
    pass


@pytest.mark.xfail(
    strict=True,
    reason="backend gap: a seller chat reply creates no CHAT notification "
    "(team-chat only publishes chat.events to the SSE edge; team-notification "
    "consumes only listing.events)",
)
@scenario(_FEATURE, "Seller reply notifies the buyer")
def test_chat_reply_notifies_buyer() -> None:
    pass


@pytest.mark.xfail(
    strict=True,
    reason="backend gap: shipping an order creates no ORDER notification "
    "(team-order emits only OrderPaidEvent; nothing turns it into a notification)",
)
@scenario(_FEATURE, "Shipping the order notifies the buyer")
def test_shipment_notifies_buyer() -> None:
    pass
