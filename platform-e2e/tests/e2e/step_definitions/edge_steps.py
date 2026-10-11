"""Edge route policy + audit access control (port-security-hardening).

Black box through the public gateway (Connect JSON): internal-only RPCs answer 501,
admin-only RPCs are gated by scope, upstream failures leak nothing, signed tokens must
carry exp and sub, and X-Request-Id / Idempotency-Key are validated. Scenario names echo
the spec scenarios of openspec/changes/port-security-hardening so `make spec-check`
matches them.
"""

from __future__ import annotations

import os
import re
import time
import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import ListingService
from src.constants import gateway_endpoints as ep
from src.models import Listing, User
from tests.e2e.flows import stop_container, wait_for_gateway
from tests.e2e.support.edge_tokens import sign_dev_token, user_claims
from tests.e2e.support.world import World

PASSWORD = "Sup3r-secret-pass!"
AUDIT = "/platform.audit.v1.AuditService"
VOUCHER = "/platform.promotion.v1.VoucherService"
ADMIN_ONLY = {
    "QueryAuditLog": f"{AUDIT}/QueryAuditLog",
    "ReviewKyc": "/platform.verification.v1.VerificationService/ReviewKyc",
    "ResolveDispute": "/platform.engagement.v1.EngagementService/ResolveDispute",
}
ADMIN_ONLY_BODY = {
    "QueryAuditLog": {"page": {"pageSize": 5}},
    "ReviewKyc": {"id": "e2e-none", "decision": "approve"},
    "ResolveDispute": {"disputeId": "e2e-none", "status": "DISPUTE_STATUS_RESOLVED"},
}
STOCK = 40


def _post(
    path: str, body: dict, token: str | None = None, headers: dict[str, str] | None = None
) -> httpx.Response:
    hdrs = {"Content-Type": "application/json", **(headers or {})}
    if token:
        hdrs["Authorization"] = f"bearer {token}"
    url = f"{get_settings().gateway_url.rstrip('/')}{path}"
    return httpx.post(url, json=body, headers=hdrs, timeout=15)


def _extra(world: World) -> dict:
    return world.state.extra


def _register(world: World, role: str) -> str:
    username = f"e2e_{role}_{uuid.uuid4().hex[:10]}"
    return world.service_factory.auth.register(username, PASSWORD, role=role)


def _admin_token(world: World) -> str:
    from src.utils import get_test_data_manager

    admin = get_test_data_manager().get_user_by_role("admin")
    return world.service_factory.auth.login(admin.username, admin.password)


# ── Givens ───────────────────────────────────────────────────────────────
@given("a logged-in seller with a listing in stock")
def seller_with_listing(world: World) -> None:
    token = _register(world, "seller")
    listing = Listing(title=f"edge-{uuid.uuid4().hex[:6]}", stock=STOCK)
    listing_id = ListingService(token=token).create_listing(listing)
    assert listing_id, "seller listing was not created"
    _extra(world).update(edge_token=token, edge_listing_id=listing_id)


@given("a logged-in seller")
def logged_in_seller(world: World) -> None:
    _extra(world)["edge_token"] = _register(world, "seller")


@given("a logged-in buyer")
def logged_in_buyer(world: World) -> None:
    _extra(world)["edge_token"] = _register(world, "buyer")


@given("the seeded admin")
def seeded_admin(world: World) -> None:
    _extra(world)["edge_admin_token"] = _admin_token(world)


@given("an anonymous client")
def anonymous_client(world: World) -> None:
    _extra(world)["edge_token"] = None


# ── Internal-only RPCs ───────────────────────────────────────────────────
@when(
    "the seller calls ReserveStock, ReleaseStock, CommitReservation and ReleaseReservation through the gateway"
)
def call_internal_rpcs(world: World) -> None:
    x = _extra(world)
    lid = x["edge_listing_id"]
    stock_body = {"listingId": lid, "quantity": 1, "reservationId": f"e2e-{uuid.uuid4().hex[:8]}"}
    calls = {
        "ReserveStock": (ep.LISTING_RESERVE_STOCK, stock_body),
        "ReleaseStock": (ep.LISTING_RELEASE_STOCK, stock_body),
        "CommitReservation": (f"{VOUCHER}/CommitReservation", {"reservationId": "e2e-res"}),
        "ReleaseReservation": (f"{VOUCHER}/ReleaseReservation", {"reservationId": "e2e-res"}),
    }
    x["edge_responses"] = {n: _post(p, b, x["edge_token"]) for n, (p, b) in calls.items()}


@then("each call answers HTTP 501 with code unimplemented")
def each_501(world: World) -> None:
    for name, resp in _extra(world)["edge_responses"].items():
        assert resp.status_code == 501, f"{name}: expected 501, got {resp.status_code} {resp.text}"
        assert resp.json().get("code") == "unimplemented", f"{name}: {resp.text}"


@then("the listing's stock is unchanged")
def stock_unchanged(world: World) -> None:
    x = _extra(world)
    listing = ListingService(token=x["edge_token"]).get_listing(x["edge_listing_id"])
    assert int(listing.get("stock", -1)) == STOCK, f"stock changed: {listing}"


# ── Audit write ──────────────────────────────────────────────────────────
@when("the seller calls WriteAuditEvent through the gateway for a fresh target type")
def seller_writes_audit(world: World) -> None:
    x = _extra(world)
    x["edge_target_type"] = f"e2e-edge-{uuid.uuid4().hex[:8]}"
    x["edge_write"] = _post(f"{AUDIT}/WriteAuditEvent", _audit_body(x), x["edge_token"])


@when(
    "the seller and the seeded admin call WriteAuditEvent through the gateway for a fresh target type"
)
def seller_and_admin_write_audit(world: World) -> None:
    x = _extra(world)
    x["edge_target_type"] = f"e2e-edge-{uuid.uuid4().hex[:8]}"
    body = _audit_body(x)
    x["edge_writes"] = [
        _post(f"{AUDIT}/WriteAuditEvent", body, x["edge_token"]),
        _post(f"{AUDIT}/WriteAuditEvent", body, x["edge_admin_token"]),
    ]


def _audit_body(x: dict) -> dict:
    return {
        "actorId": "forged-actor",
        "action": "e2e.edge.write",
        "targetType": x["edge_target_type"],
        "targetId": "tgt-1",
    }


@then("the gateway answers HTTP 501 for each write")
def writes_501(world: World) -> None:
    for resp in _extra(world)["edge_writes"]:
        assert resp.status_code == 501, f"expected 501, got {resp.status_code} {resp.text}"


@then("the refused write call fails")
def write_failed(world: World) -> None:
    resp = _extra(world)["edge_write"]
    assert resp.status_code >= 400, f"a refused write must fail, got {resp.status_code}"


@then("no audit event is stored for that target type")
@then("the seeded admin's QueryAuditLog for that target type returns no events")
def no_event_stored(world: World) -> None:
    x = _extra(world)
    resp = _post(
        f"{AUDIT}/QueryAuditLog",
        {"targetType": x["edge_target_type"], "page": {"pageSize": 50}},
        x["edge_admin_token"] if "edge_admin_token" in x else _admin_token(world),
    )
    assert resp.status_code == 200, f"admin query failed: {resp.status_code} {resp.text}"
    assert resp.json().get("events", []) == [], f"an event was stored: {resp.text}"


# ── Admin-only RPCs ──────────────────────────────────────────────────────
def _call_admin_only(world: World, token: str | None) -> None:
    _extra(world)["edge_responses"] = {
        n: _post(p, ADMIN_ONLY_BODY[n], token) for n, p in ADMIN_ONLY.items()
    }


@when("the client calls QueryAuditLog, ReviewKyc and ResolveDispute through the gateway")
def anon_calls_admin_only(world: World) -> None:
    _call_admin_only(world, None)


@when("the buyer calls QueryAuditLog, ReviewKyc and ResolveDispute through the gateway")
def buyer_calls_admin_only(world: World) -> None:
    _call_admin_only(world, _extra(world)["edge_token"])


@then(parsers.parse("each admin-only call answers HTTP {status:d}"))
def each_status(world: World, status: int) -> None:
    bad = {
        n: (r.status_code, r.text)
        for n, r in _extra(world)["edge_responses"].items()
        if r.status_code != status
    }
    assert not bad, f"expected {status} on every admin-only RPC, got {bad}"


@when("the seeded admin calls QueryAuditLog through the gateway")
def admin_queries(world: World) -> None:
    _extra(world)["edge_query"] = _post(
        ADMIN_ONLY["QueryAuditLog"], {"page": {"pageSize": 5}}, _extra(world)["edge_admin_token"]
    )


@then("the request succeeds with an events list")
def succeeds_with_events(world: World) -> None:
    resp = _extra(world)["edge_query"]
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"
    assert isinstance(resp.json().get("events", []), list), resp.text


# ── Unreachable upstream (destructive) ───────────────────────────────────
@given("team-audit is stopped")
def stop_audit(world: World) -> None:
    name = os.getenv("AUDIT_CONTAINER", "agora-team-audit-svc")
    restore = stop_container(name)
    admin_token = _extra(world)["edge_admin_token"]

    def restore_and_wait() -> None:
        restore()
        wait_for_gateway(get_settings().gateway_url)
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            try:
                if _post(ADMIN_ONLY["QueryAuditLog"], {}, admin_token).status_code == 200:
                    return
            except httpx.HTTPError:
                pass
            time.sleep(1)
        raise TimeoutError("team-audit did not recover after the destructive scenario")

    world.add_cleanup(restore_and_wait)


@when("the seeded admin calls QueryAuditLog through the gateway while team-audit is down")
def admin_queries_audit_down(world: World) -> None:
    _extra(world)["edge_query"] = _post(
        ADMIN_ONLY["QueryAuditLog"], {"page": {"pageSize": 5}}, _extra(world)["edge_admin_token"]
    )


@then("the gateway answers with a fixed upstream-failure message")
def answers_fixed_upstream_failure(world: World) -> None:
    # A stopped container's DNS name disappears, so the gateway's gRPC client
    # waits and times out (504); a refused connection is 503. Both carry a fixed
    # message and never the client's resolver/LB text.
    resp = _extra(world)["edge_query"]
    expected = {503: "service unavailable", 504: "upstream timed out"}
    assert resp.status_code in expected, f"{resp.status_code} {resp.text}"
    assert resp.json().get("message") == expected[resp.status_code], resp.text


@then("the body contains no host, port or dial error text")
def no_internal_detail(world: World) -> None:
    text = _extra(world)["edge_query"].text.lower()
    for leak in (
        "dial",
        "tcp",
        "connection refused",
        "team-audit",
        "lookup",
        ":50",
        "no such host",
        "lb policy",
        "deadline exceeded",
    ):
        assert leak not in text, f"body leaks {leak!r}: {text}"
    assert not re.search(r"\d{1,3}(\.\d{1,3}){3}", text), f"body leaks an address: {text}"


# ── Token claims ─────────────────────────────────────────────────────────
def _real_user_id(world: World) -> str:
    return User(username="", password="", role="buyer", token=_register(world, "buyer")).user_id


def _status_with(token: str) -> httpx.Response:
    return _post("/platform.order.v1.OrderService/ListBuyerOrders", {}, token)


@given("a token signed by the identity key is accepted when it carries exp and sub")
def control_token_accepted(world: World) -> None:
    sub = _real_user_id(world)
    _extra(world)["edge_sub"] = sub
    resp = _status_with(sign_dev_token(user_claims(sub)))
    assert (
        resp.status_code == 200
    ), f"control token must be accepted: {resp.status_code} {resp.text}"


@when(
    "a client calls an authenticated RPC with a token signed by the identity key that has no exp claim"
)
def call_without_exp(world: World) -> None:
    _extra(world)["edge_resp"] = _status_with(
        sign_dev_token(user_claims(_extra(world)["edge_sub"], with_exp=False))
    )


@when(
    "a client calls an authenticated RPC with a token signed by the identity key whose sub is empty"
)
def call_without_sub(world: World) -> None:
    _extra(world)["edge_resp"] = _status_with(sign_dev_token(user_claims("")))


@then("the gateway answers HTTP 401")
def answers_401(world: World) -> None:
    resp = _extra(world)["edge_resp"]
    assert resp.status_code == 401, f"expected 401, got {resp.status_code} {resp.text}"


# ── Request id / idempotency key ─────────────────────────────────────────
@when(parsers.parse('a client sends X-Request-Id "{value}" on an RPC'))
def send_request_id(world: World, value: str) -> None:
    _extra(world)["edge_sent_rid"] = value
    _extra(world)["edge_resp"] = _post(ep.SEARCH_LISTINGS, {}, headers={"X-Request-Id": value})


@when("a client sends an X-Request-Id containing spaces and angle brackets")
def send_bad_request_id(world: World) -> None:
    bad = "bad id <script>"
    _extra(world)["edge_sent_rid"] = bad
    _extra(world)["edge_resp"] = _post(ep.SEARCH_LISTINGS, {}, headers={"X-Request-Id": bad})


@then(parsers.parse('the response header X-Request-Id is "{value}"'))
def rid_is(world: World, value: str) -> None:
    resp = _extra(world)["edge_resp"]
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"
    assert resp.headers.get("X-Request-Id") == value, dict(resp.headers)


@then("the response header X-Request-Id is a different, well-formed id")
def rid_replaced(world: World) -> None:
    resp = _extra(world)["edge_resp"]
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"
    rid = resp.headers.get("X-Request-Id", "")
    assert rid != _extra(world)["edge_sent_rid"], rid
    assert re.fullmatch(r"[A-Za-z0-9._-]{1,64}", rid), f"not well-formed: {rid!r}"


@when("the buyer calls CreateOrder with an Idempotency-Key longer than 255 bytes")
def create_order_long_key(world: World) -> None:
    _extra(world)["edge_resp"] = _post(
        ep.ORDER_CREATE,
        {"paymentMethod": "PAYMENT_METHOD_COD"},
        _extra(world)["edge_token"],
        headers={"Idempotency-Key": "k" * 300},
    )


@then("the gateway answers HTTP 400")
def answers_400(world: World) -> None:
    resp = _extra(world)["edge_resp"]
    assert resp.status_code == 400, f"expected 400, got {resp.status_code} {resp.text}"
