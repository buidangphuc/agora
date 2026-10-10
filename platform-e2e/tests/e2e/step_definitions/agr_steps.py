"""Steps for ai/assistant_grounding.feature (change assistant-rag-grounding, spec ai-assistant)."""

from __future__ import annotations

import httpx
from pytest_bdd import given, then, when

from tests.e2e.flows.stack_flow import stop_container
from tests.e2e.support import agr_support as a
from tests.e2e.support import hrp_support as h
from tests.e2e.support import ms_support as ms
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World


def _word(world: World) -> str:
    return a.bag(world).setdefault("word", h.word())


@when(
    "a seller publishes a listing with a unique title and a buyer asks the assistant for that title"
)
def publish_and_ask(world: World) -> None:
    word = _word(world)
    a.publish(world, "L", f"{word} blender")
    a.bag(world)["resp"] = a.wait_returned(world, word, "L")


@then(
    "the response contains a product card whose listing id is that listing's id with its title and "
    "price"
)
def card_for_listing(world: World) -> None:
    listed = s.listing(world, "L")
    card = next(c for c in a.cards(a.bag(world)["resp"]) if c["listingId"] == listed.id)
    assert card["title"] == listed.title, card
    assert int(card["price"]) == 100_000, card
    assert card.get("currency") == "VND", card


@given(
    "the assistant returns a published control listing, a listing to unpublish and a listing to delete"
)
def three_listings(world: World) -> None:
    word = _word(world)
    for label in ("keep", "draft", "gone"):
        a.publish(world, label, f"{word} {label}")
    a.wait_returned(world, word, "keep", "draft", "gone")


@when("the seller changes the first to draft and deletes the second")
def draft_and_delete(world: World) -> None:
    seller = s.seller_of(world).token
    draft = s.listing(world, "draft")
    current = s.ok_json(s.post(s.LISTING + "GetListing", {"id": draft.id}))["listing"]
    current["status"] = "LISTING_STATUS_DRAFT"
    s.ok_json(s.post(s.LISTING + "UpdateListing", {"listing": current}, seller))
    s.ok_json(s.post(s.LISTING + "DeleteListing", {"id": s.listing(world, "gone").id}, seller))


@then("the assistant no longer returns either of them and still returns the control listing")
def gone_but_control_stays(world: World) -> None:
    word, keep = _word(world), s.listing(world, "keep").id
    gone = {s.listing(world, "draft").id, s.listing(world, "gone").id}

    def _settled():
        got = a.card_ids(a.ask(world, word))
        return keep in got and not gone & set(got) and got

    s.eventually(
        _settled, "the draft and deleted listings to leave the assistant's cards", a.INDEX_S, 2.0
    )


@given("the assistant returns a published listing")
def one_listing(world: World) -> None:
    word = _word(world)
    a.publish(world, "L", f"{word} blender")
    a.wait_returned(world, word, "L")


@given("the modelserve router is stopped")
def stop_router(world: World) -> None:
    restore = stop_container(ms.router_container())
    a.bag(world)["restore"] = restore

    def _restart() -> None:
        restore()
        s.eventually(
            lambda: httpx.get(ms.router_url() + "/healthz", timeout=3).status_code == 200,
            "the modelserve router to come back",
            90.0,
            2.0,
        )

    a.bag(world)["restart"] = _restart
    world.add_cleanup(_restart)


@when("a buyer asks the assistant for that listing")
def ask_while_down(world: World) -> None:
    a.bag(world)["resp"] = a.ask(world, _word(world))


@then("the call succeeds with a non-empty reply and no product cards")
def answers_without_cards(world: World) -> None:
    resp = a.bag(world)["resp"]
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"
    assert resp.json().get("replyText"), resp.text[:300]
    assert a.cards(resp) == [], a.cards(resp)


@then("the product card comes back once the router is restored")
def card_returns(world: World) -> None:
    a.bag(world)["restart"]()
    a.wait_returned(world, _word(world), "L")
