"""Binds journeys/post_purchase_chat_rma_journey.feature to its step definitions.

The journey runs against the real stack. The notification scenarios depend on
the notify-chat-and-shipment change: team-chat publishes ChatMessage (with
recipient_id) to chat.events and team-order writes OrderShipped to order.events;
team-notification consumes both and creates the CHAT / ORDER notifications for
the right user, honouring the user's notification preferences.
"""

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


@scenario(_FEATURE, "Seller reply notifies the buyer")
def test_chat_reply_notifies_buyer() -> None:
    pass


@scenario(_FEATURE, "Chat notifications respect preferences")
def test_chat_notifications_respect_preferences() -> None:
    pass


@scenario(_FEATURE, "Shipping the order notifies the buyer")
def test_shipment_notifies_buyer() -> None:
    pass
