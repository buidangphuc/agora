"""Admin order operations need order.admin (authz-residuals-2: auth, order-read-access).

Black box through the gateway with distinct real users (seller, buyer b1, buyer b2). The
"admin without order.admin" caller is a token signed with the LOCAL dev identity key carrying
only the `admin` scope (edge_tokens.sign_dev_token): it passes the gateway's admin policy, so
only team-order's own check can refuse it. Everything else (orders, stock, order status)
reuses the OIC steps.
"""

from __future__ import annotations

import base64
import json
import time
import uuid

from pytest_bdd import given, parsers, then, when

from tests.e2e.step_definitions.oic_order_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.pear_authz_edge_steps import *  # noqa: F401,F403
from tests.e2e.support import oic_order_support as s
from tests.e2e.support.edge_tokens import sign_dev_token


def _scopes(token: str) -> list[str]:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return list(json.loads(base64.urlsafe_b64decode(payload)).get("scopes") or [])


def _admin_marker_only(oic) -> s.Actor:
    sub = f"e2e-ar2-{uuid.uuid4().hex[:10]}"
    token = sign_dev_token(
        {
            "sub": sub,
            "typ": "user",
            "name": "e2e-ar2",
            "scopes": ["admin"],
            "iat": int(time.time()),
            "exp": int(time.time()) + 600,
        }
    )
    return s.Actor(sub, token, "admin", sub)


@given("the seeded admin for the order-scope checks")
def seeded_admin(oic):
    s.login_admin(oic)


@then(parsers.parse('the admin\'s token scopes include "{a}" and "{b}"'))
def admin_scopes(oic, a, b):
    scopes = _scopes(oic.actors["admin"].token)
    assert a in scopes and b in scopes, f"admin scopes: {scopes}"


@then(parsers.parse('the token scopes of "{x}" and "{y}" include neither "{one}" nor "{two}"'))
def no_admin_scopes(oic, x, y, one, two):
    for name in (x, y):
        scopes = _scopes(oic.actors[name].token)
        assert one not in scopes and two not in scopes, f"{name} scopes: {scopes}"


@when(
    "a caller whose token carries the admin scope but not order.admin calls ForceFailSaga on the order"
)
def admin_only_force_fail(oic):
    caller = _admin_marker_only(oic)
    oic.responses["last"] = s.post(
        oic,
        caller,
        s.ORDER,
        "ForceFailSaga",
        {"orderId": oic.orders["order"], "failStep": "payment"},
    )


@when(
    "a caller whose token carries the admin scope but not order.admin calls GetSagaState on the order"
)
def admin_only_get_saga(oic):
    caller = _admin_marker_only(oic)
    oic.responses["last"] = s.post(
        oic, caller, s.ORDER, "GetSagaState", {"orderId": oic.orders["order"]}
    )


@when("the seeded admin calls GetSagaState on the order")
def admin_get_saga(oic):
    oic.responses["last"] = s.post(
        oic, oic.actors["admin"], s.ORDER, "GetSagaState", {"orderId": oic.orders["order"]}
    )


@when(
    parsers.parse(
        'the buyer "{other}" calls ForceFailSaga on the order of "{owner}" through the gateway'
    )
)
def other_buyer_force_fails(oic, other, owner):
    oic.responses["last"] = s.post(
        oic,
        oic.actors[other],
        s.ORDER,
        "ForceFailSaga",
        {"orderId": oic.orders["order"], "failStep": "payment"},
    )


@then(parsers.parse('the order call fails with "{code}"'))
def order_call_fails(oic, code):
    resp = oic.responses["last"]
    assert (
        s.code_of(resp) == code
    ), f"expected {code}, got HTTP {resp.status_code}: {resp.text[:300]}"


@then("the saga response is for that order")
def saga_for_order(oic):
    body = s.ok(oic.responses["last"])
    assert body.get("orderId") == oic.orders["order"], body
