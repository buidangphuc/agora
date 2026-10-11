"""A buyer cannot force-fail their own order (port-edge-authz-residuals / edge-route-policy).

Black box through the gateway: a real buyer with a real Pending order calls ForceFailSaga with
their own token. The spec promises HTTP 403 and an untouched order (still Pending, stock held).
Reuses the order/inventory fixtures and the order and stock assertions of the OIC steps.
"""

from __future__ import annotations

from pytest_bdd import parsers, then, when

from tests.e2e.step_definitions.oic_order_steps import *  # noqa: F401,F403
from tests.e2e.support import oic_order_support as s


@when(
    parsers.parse('the buyer "{buyer}" calls ForceFailSaga on their own order through the gateway')
)
def buyer_force_fails_own_order(oic, buyer):
    oic.responses["last"] = s.post(
        oic,
        oic.actors[buyer],
        s.ORDER,
        "ForceFailSaga",
        {"orderId": oic.orders["order"], "failStep": "payment"},
    )


@then("the gateway answers HTTP 403 to the force-fail call")
def force_fail_403(oic):
    resp = oic.responses["last"]
    assert resp.status_code == 403, f"expected HTTP 403, got {resp.status_code}: {resp.text[:300]}"
