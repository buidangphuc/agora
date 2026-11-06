from __future__ import annotations

import json
import os
import re
from pathlib import Path

from pytest_bdd import given, parsers, then, when

from tests.e2e.step_definitions.oic_order_steps import *  # noqa: F401,F403
from tests.e2e.support import oic_order_support as s
from tests.e2e.support import order_lifecycle_support as ol
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import plp_support as p

PROTO_FILE = "platform/order/v1/order.proto"
SERVICE = "platform.order.v1.OrderService"
SERVICE_ID = "service-team-promotion"
READ_SCOPE = "order.read"
ADMIN_SCOPE = "order.admin"
UNIMPLEMENTED_PATH = f"{s.ORDER}/HasCompletedOrder"


def _order_addr() -> str:
    return os.getenv("Q7_ORDER_ADDR", "team-order-svc:50055")


def _grpcurl_image() -> str:
    return os.getenv("FLASH_HOLD_GRPCURL_IMAGE", "fullstorydev/grpcurl:latest")


def _proto_dir() -> Path:
    env = os.getenv("Q7_ORDER_PROTO_DIR")
    if env:
        return Path(env)
    return Path(__file__).resolve().parents[4] / "team-order" / "proto"


def _snake(grpc_code: str) -> str:
    return re.sub(r"(?<!^)(?=[A-Z])", "_", grpc_code).lower()


def has_completed_order(buyer_id: str, headers: dict[str, str] | None) -> tuple[str, object]:
    pdir = _proto_dir()
    assert (pdir / PROTO_FILE).is_file(), f"order proto not found under {pdir} (Q7_ORDER_PROTO_DIR)"
    cmd = [
        "run", "--rm", "--network", stack.stack_network(), "-v", f"{pdir}:/proto:ro",
        _grpcurl_image(), "-plaintext", "-emit-defaults", "-import-path", "/proto",
        "-proto", PROTO_FILE,
    ]  # fmt: skip
    for name, value in (headers or {}).items():
        cmd += ["-H", f"{name}: {value}"]
    cmd += [
        "-d",
        json.dumps({"buyerId": buyer_id}),
        _order_addr(),
        f"{SERVICE}/HasCompletedOrder",
    ]
    proc = stack.docker(*cmd, check=False, timeout=120)
    if proc.returncode == 0:
        return "ok", json.loads(proc.stdout or "{}")
    err = (proc.stderr + proc.stdout).strip()
    m = re.search(r"Code:\s*(\w+)", err)
    return (_snake(m.group(1)) if m else f"grpcurl_exit_{proc.returncode}"), err


def _service_headers(scopes: str) -> dict[str, str]:
    headers = {"x-principal-id": SERVICE_ID, "x-principal-type": "service"}
    if scopes:
        headers["x-principal-scopes"] = scopes
    return headers


def _user_headers(user_id: str, scopes: str = "") -> dict[str, str]:
    headers = {"x-principal-id": user_id, "x-principal-type": "user"}
    if scopes:
        headers["x-principal-scopes"] = scopes
    return headers


def _lookup(oic, buyer_id: str, headers: dict[str, str] | None) -> None:
    oic.data["q7_lookup"] = has_completed_order(buyer_id, headers)


def _buyer_id(oic, name: str) -> str:
    return oic.actors[name].user_id


def _stash(oic, name: str, kind: str, order_id: str) -> None:
    oic.data.setdefault("q7_orders", {}).setdefault((name, kind), []).append(order_id)


def _latest(oic, name: str, kind: str) -> str:
    return oic.data["q7_orders"][(name, kind)][-1]


@given(parsers.parse('a seller "{name}" with a stocked listing "{listing}"'))
def q7_seller_listing(oic, name, listing):
    seller = oic.actors.get(name) or s.register(oic, name, "seller")
    s.create_listing(oic, listing, seller, 50, price=600000)


@given(
    parsers.re(
        r'"(?P<b>\w+)" has a (?P<kind>pending|paid|shipped|cancelled|completed) order '
        r'for "(?P<l>\w+)"'
    )
)
def q7_has_order(oic, b, kind, l):  # noqa: E741
    buyer = oic.actors[b]
    seller = oic.listings[l]["seller"]
    if kind == "paid":
        order_id, _tx = p.place_and_pay(oic, buyer, l)
    else:
        order_id = ol.place(oic, buyer, l, 1, "PAYMENT_METHOD_COD")
    if kind == "cancelled":
        s.ok(s.post(oic, buyer, s.ORDER, "CancelOrder", {"id": order_id}))
    if kind in ("shipped", "completed"):
        ol.ship(oic, order_id, seller)
    if kind == "completed":
        s.ok(s.post(oic, buyer, s.ORDER, "ConfirmReceipt", {"id": order_id}))
        s.wait_status(oic, buyer, order_id, s.COMPLETED)
    _stash(oic, b, kind, order_id)


@when(parsers.parse('the seller "{seller}" completes the shipped order of "{b}"'))
def q7_seller_completes(oic, seller, b):
    order_id = _latest(oic, b, "shipped")
    s.ok(
        s.post(
            oic,
            oic.actors[seller],
            s.ORDER,
            "UpdateOrderStatus",
            {"id": order_id, "status": s.COMPLETED},
        )
    )
    s.wait_status(oic, oic.actors[b], order_id, s.COMPLETED)


@when(parsers.parse('"{b}" confirms receipt of the shipped order'))
def q7_confirms(oic, b):
    order_id = _latest(oic, b, "shipped")
    s.ok(s.post(oic, oic.actors[b], s.ORDER, "ConfirmReceipt", {"id": order_id}))
    s.wait_status(oic, oic.actors[b], order_id, s.COMPLETED)


@when(parsers.parse('a service principal with order.read asks whether "{b}" has a completed order'))
def q7_service_asks(oic, b):
    _lookup(oic, _buyer_id(oic, b), _service_headers(READ_SCOPE))


@when("a service principal with order.read asks about an empty buyer id")
def q7_empty_id(oic):
    _lookup(oic, "", _service_headers(READ_SCOPE))


@when(parsers.parse('the user principal "{who}" asks whether "{b}" has a completed order'))
def q7_user_asks(oic, who, b):
    _lookup(oic, _buyer_id(oic, b), _user_headers(oic.actors[who].user_id))


@when(
    parsers.parse('a user principal holding order.admin asks whether "{b}" has a completed order')
)
def q7_admin_asks(oic, b):
    _lookup(oic, _buyer_id(oic, b), _user_headers("q7-admin-user", ADMIN_SCOPE))


@when(
    parsers.parse('a service principal without order.read asks whether "{b}" has a completed order')
)
def q7_service_no_scope(oic, b):
    _lookup(oic, _buyer_id(oic, b), _service_headers(""))


@when(parsers.parse('a call without any principal asks whether "{b}" has a completed order'))
def q7_no_principal(oic, b):
    _lookup(oic, _buyer_id(oic, b), None)


@when(parsers.parse('"{who}" posts HasCompletedOrder for "{b}" through the gateway'))
def q7_gateway_user(oic, who, b):
    oic.responses["last"] = s.post(
        oic, oic.actors[who], s.ORDER, "HasCompletedOrder", {"buyerId": _buyer_id(oic, b)}
    )


@when(parsers.parse('the admin posts HasCompletedOrder for "{b}" through the gateway'))
def q7_gateway_admin(oic, b):
    admin = s.login_admin(oic)
    oic.responses["last"] = s.post(
        oic, admin, s.ORDER, "HasCompletedOrder", {"buyerId": _buyer_id(oic, b)}
    )


@then(parsers.parse("the lookup succeeds with has_completed {expected}"))
def q7_lookup_ok(oic, expected):
    code, body = oic.data["q7_lookup"]
    assert code == "ok", f"want ok, got {code}: {body}"
    assert isinstance(body, dict) and body.get("hasCompleted") is (expected == "true"), (
        f"want has_completed {expected}, got {body}"
    )


@then(parsers.parse('the lookup fails with "{code}"'))
def q7_lookup_fails(oic, code):
    got, body = oic.data["q7_lookup"]
    assert got == code, f"want {code}, got {got}: {body}"


@then("the gateway answers HTTP 501 with code unimplemented")
def q7_gateway_501(oic):
    resp = oic.responses["last"]
    assert resp.status_code == 501, f"want 501, got {resp.status_code} {resp.text[:300]}"
    assert resp.json().get("code") == "unimplemented", resp.text[:300]
