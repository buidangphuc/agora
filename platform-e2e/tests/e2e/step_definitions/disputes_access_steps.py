"""Dispute and Q&A access control (port-security-hardening, dispute-and-qa-access).

Drives CreateDispute / GetDispute / AnswerQuestion through the gateway Connect API and
asserts the HTTP status and the stored answer. The order's seller id is read from the
order itself (GetOrder), never guessed. @needsOrder seeds the buyer, the seller that
owns the listing, and a COD order.
"""

from __future__ import annotations

import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from tests.e2e.step_definitions.seller_analytics_access_steps import _subject
from tests.e2e.support.world import World

PASSWORD = "Sup3r-secret-pass!"
_SVC = "/platform.engagement.v1.EngagementService"


def _post(world: World, token: str | None, method: str, body: dict) -> httpx.Response:
    headers = {"Content-Type": "application/json", "Authorization": f"bearer {token}"}
    return httpx.post(
        f"{world.settings.gateway_url}{_SVC}/{method}", json=body, headers=headers, timeout=30.0
    )


def _order_seller_id(world: World) -> str:
    buyer = world.state.extra["seeded_buyer"]
    world.service_factory.set_token(buyer.token)
    order = world.service_factory.order.get_order(world.state.order_id).get("order", {})
    seller_id = order.get("sellerId", "")
    assert seller_id, f"order carries no sellerId: {order}"
    return seller_id


def _create_dispute(world: World, token: str, defendant_id: str) -> httpx.Response:
    return _post(
        world,
        token,
        "CreateDispute",
        {
            "orderId": world.state.order_id,
            "defendantId": defendant_id,
            "reason": "Khong nhan duoc hang dung mo ta",
            "evidenceUrls": [],
        },
    )


@given("a buyer who placed an order on a seller's listing")
def buyer_with_order(world: World) -> None:
    assert world.state.order_id, "scenario must be tagged @needsOrder"
    world.state.extra["order_seller_id"] = _order_seller_id(world)


@given("a second buyer who did not place the order")
def second_buyer(world: World) -> None:
    username = f"e2e_buyer_{uuid.uuid4().hex[:10]}"
    world.state.extra["stranger_token"] = world.service_factory.auth.register(
        username, PASSWORD, role="buyer"
    )


@given("the buyer has opened a dispute against the order's seller")
def buyer_opened_dispute(world: World) -> None:
    buyer = world.state.extra["seeded_buyer"]
    resp = _create_dispute(world, buyer.token, world.state.extra["order_seller_id"])
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text}"
    world.state.extra["dispute_id"] = resp.json()["dispute"]["id"]


@when("the buyer of an order opens a dispute naming the order's seller")
def open_dispute_against_seller(world: World) -> None:
    buyer = world.state.extra["seeded_buyer"]
    world.state.extra["resp"] = _create_dispute(
        world, buyer.token, world.state.extra["order_seller_id"]
    )


@when("the buyer of an order opens a dispute naming a user who is not the order's seller")
def open_dispute_wrong_defendant(world: World) -> None:
    buyer = world.state.extra["seeded_buyer"]
    other = f"e2e_seller_{uuid.uuid4().hex[:10]}"
    other_id = _subject(world.service_factory.auth.register(other, PASSWORD, role="seller"))
    assert other_id != world.state.extra["order_seller_id"]
    world.state.extra["resp"] = _create_dispute(world, buyer.token, other_id)


@when("a buyer who did not place the order opens a dispute on it")
def stranger_opens_dispute(world: World) -> None:
    world.state.extra["resp"] = _create_dispute(
        world, world.state.extra["stranger_token"], world.state.extra["order_seller_id"]
    )


@when("a logged-in user who is neither party nor admin calls GetDispute for an existing dispute")
def stranger_reads_dispute(world: World) -> None:
    world.state.extra["resp"] = _post(
        world,
        world.state.extra["stranger_token"],
        "GetDispute",
        {"disputeId": world.state.extra["dispute_id"]},
    )


@then("the dispute is created")
def dispute_created(world: World) -> None:
    resp: httpx.Response = world.state.extra["resp"]
    assert resp.status_code == 200, f"expected 200, got {resp.status_code}: {resp.text}"
    dispute = resp.json()["dispute"]
    assert dispute["id"], dispute
    assert dispute["defendantId"] == world.state.extra["order_seller_id"], dispute
    world.state.extra["dispute_id"] = dispute["id"]


@then("both the buyer and the seller can read it")
def parties_can_read(world: World) -> None:
    for role in ("seeded_buyer", "seeded_seller"):
        user = world.state.extra[role]
        resp = _post(
            world, user.token, "GetDispute", {"disputeId": world.state.extra["dispute_id"]}
        )
        assert resp.status_code == 200, f"{role}: {resp.status_code}: {resp.text}"
        assert resp.json()["dispute"]["id"] == world.state.extra["dispute_id"]


@then(parsers.parse("the gateway answers HTTP {status:d}"))
def gateway_answers(world: World, status: int) -> None:
    resp: httpx.Response = world.state.extra["resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"


# ── Shop reply ───────────────────────────────────────────────────────────


@given("a question on a seller's listing")
def question_on_listing(world: World) -> None:
    buyer = world.state.extra["seeded_buyer"]
    resp = _post(
        world,
        buyer.token,
        "AskQuestion",
        {"listingId": world.state.listing.listing_id, "questionText": "Con hang khong shop?"},
    )
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text}"
    world.state.extra["question_id"] = resp.json()["question"]["id"]


@when("a buyer answers a question on another seller's listing with is_shop_reply set")
def non_owner_answers(world: World) -> None:
    buyer = world.state.extra["seeded_buyer"]
    world.state.extra["resp"] = _post(
        world,
        buyer.token,
        "AnswerQuestion",
        {
            "questionId": world.state.extra["question_id"],
            "answerText": "Shop xin tra loi",
            "isShopReply": True,
        },
    )


@then("the stored answer is not marked as a shop reply")
def answer_not_shop_reply(world: World) -> None:
    resp: httpx.Response = world.state.extra["resp"]
    assert resp.status_code == 200, f"expected 200, got {resp.status_code}: {resp.text}"
    answer = resp.json()["answer"]
    assert not answer.get("isShopReply"), f"non-owner answer stored as shop reply: {answer}"
    listed = world.service_factory.engagement.list_questions(world.state.listing.listing_id)
    answers = [
        a
        for q in listed.get("questions", [])
        if q.get("id") == world.state.extra["question_id"]
        for a in q.get("answers", [])
    ]
    assert answers, f"answer not stored: {listed}"
    assert not any(a.get("isShopReply") for a in answers), f"stored as shop reply: {answers}"
