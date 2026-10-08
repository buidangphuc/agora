"""A SERVICE token cannot place an order (port-edge-authz-residuals / order-checkout-correctness).

The token is signed with the local dev identity key (`support.edge_tokens`), typ "service",
scopes broad enough that only the principal type can be the reason for a refusal, and the
buyer's user id as subject so it addresses the buyer's real cart.
"""

from __future__ import annotations

import time

from pytest_bdd import parsers, when

from tests.e2e.step_definitions.oic_order_steps import *  # noqa: F401,F403
from tests.e2e.support import oic_order_support as s
from tests.e2e.support.edge_tokens import sign_dev_token


@when(
    parsers.parse(
        "a client calls CreateOrder through the gateway with a validly signed SERVICE token "
        'for the cart of "{buyer}"'
    )
)
def service_token_create_order(oic, buyer):
    buyer_actor = oic.actors[buyer]
    now = int(time.time())
    token = sign_dev_token(
        {
            "sub": buyer_actor.user_id,
            "typ": "service",
            "name": "e2e-service",
            "scopes": ["listing.write", "order.write", "admin"],
            "iat": now,
            "exp": now + 600,
        }
    )
    service = s.Actor("e2e-service", token, "service", buyer_actor.user_id)
    oic.responses["last"] = s.checkout(oic, service)
