"""Steps for ai/assistant_grounding.feature (change ai-assistant-grounding, spec ai-assistant-grounding)."""

from __future__ import annotations

import time

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from tests.e2e.flows import stop_container, wait_for_search
from tests.e2e.support import agr_support as a
from tests.e2e.support import apr_support as apr
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

SETTINGS = get_settings()
RAG_OFF_OVERLAY = pe.REPO_ROOT / "platform-e2e" / "compose" / "rag-off.override.yaml"


@when(
    "a seller publishes a listing with stock 5 and a buyer asks the assistant for its unique title"
)
def publish_and_ask(world: World) -> None:
    w = a.word(world)
    a.publish(world, "L", f"{w} blender", price=123_000, stock=5)
    a.bag(world)["resp"] = a.wait_returned(world, w, "L")


@then(
    "the response has a product card whose listing id, title and price equal that listing's domain values"
)
def card_equals_domain(world: World) -> None:
    listed = s.listing(world, "L")
    domain = a.domain_listing(listed.id)
    card = next(c for c in a.cards(a.bag(world)["resp"]) if c["listingId"] == listed.id)
    assert card["title"] == domain["title"] == listed.title, (card, domain)
    assert int(card["price"]) == int(domain["price"]) == 123_000, (card, domain)


@given("the assistant returns a published in-stock control listing")
def control_listing(world: World) -> None:
    w = a.word(world)
    a.publish(world, "keep", f"{w} control")
    a.bag(world)["resp"] = a.wait_returned(world, w, "keep")


@given("a published in-stock control listing exists in the catalog")
def control_listing_exists(world: World) -> None:
    a.publish(world, "keep", f"{a.word(world)} control")
    a.wait_searchable(world, "keep")


@given("a published listing with stock 0 shares the control listing's title word")
def out_of_stock_listing(world: World) -> None:
    a.publish(world, "oos", f"{a.word(world)} soldout", stock=0)
    a.wait_searchable(world, "oos", strict=False)


@then(
    "the response contains no product card for the listing with stock 0 and still has the control card"
)
def no_oos_card(world: World) -> None:
    ids = a.card_ids(a.bag(world)["resp"])
    assert s.listing(world, "keep").id in ids, ids
    assert s.listing(world, "oos").id not in ids, ids


@given("the assistant returns a published in-stock control listing and a second published listing")
def control_and_second(world: World) -> None:
    w = a.word(world)
    a.publish(world, "keep", f"{w} control")
    a.publish(world, "draft", f"{w} second")
    a.wait_returned(world, w, "keep", "draft")


@when("the seller changes the second listing to draft")
def second_to_draft(world: World) -> None:
    current = a.domain_listing(s.listing(world, "draft").id)
    current["status"] = "LISTING_STATUS_DRAFT"
    s.ok_json(s.post(s.LISTING + "UpdateListing", {"listing": current}, s.seller_of(world).token))


@then("the assistant no longer returns the second listing and still returns the control listing")
def second_gone(world: World) -> None:
    w, keep, gone = a.word(world), s.listing(world, "keep").id, s.listing(world, "draft").id

    def _settled():
        resp = a.ask(world, w)
        if resp.status_code == 429:
            return None
        got = a.card_ids(resp)
        return keep in got and gone not in got and got

    s.eventually(
        _settled, "the draft listing to leave the assistant's cards", a.INDEX_S, a.ASK_EVERY_S
    )


@given("published listings priced 300000 and 800000 share a unique title word")
def two_prices(world: World) -> None:
    w = a.word(world)
    a.publish(world, "cheap", f"{w} cheap", price=300_000)
    a.publish(world, "dear", f"{w} dear", price=800_000)
    a.wait_returned(world, w, "cheap", "dear")


@when(parsers.parse('a buyer asks the assistant for that word with the budget "{budget}"'))
def ask_with_budget(world: World, budget: str) -> None:
    message = f"{a.word(world)} {budget}"
    cheap = s.listing(world, "cheap").id

    def _returned():
        r = a.ask(world, message)
        if r.status_code == 429:
            return None
        return cheap in a.card_ids(r) and r

    a.bag(world)["resp"] = s.eventually(
        _returned, f"the assistant to answer {message!r}", a.INDEX_S, a.ASK_EVERY_S
    )


@then("every returned card has a price of at most 500000 and the 300000 listing is returned")
def within_budget(world: World) -> None:
    got = a.cards(a.bag(world)["resp"])
    assert got and all(int(c["price"]) <= 500_000 for c in got), got
    assert s.listing(world, "dear").id not in [c["listingId"] for c in got], got


@when("a buyer asks the assistant for that title word twice")
def ask_twice(world: World) -> None:
    first = a.ask(world, a.word(world))
    time.sleep(a.ASK_EVERY_S)
    second = a.ask(world, a.word(world))
    a.bag(world)["pair"] = (first, second)


@then("both replies are byte-identical and name the first card's title and its price")
def deterministic_reply(world: World) -> None:
    first, second = a.bag(world)["pair"]
    r1, r2 = a.body(first), a.body(second)
    assert r1["replyText"] == r2["replyText"], (r1["replyText"], r2["replyText"])
    top = a.cards(first)[0]
    assert top["title"] in r1["replyText"], (top, r1["replyText"])
    assert str(int(top["price"])) in a.digits(r1["replyText"]), (top, r1["replyText"])


@when("a buyer asks the assistant for a product no published in-stock listing matches")
def ask_no_match(world: World) -> None:
    s.buyer_of(world)
    a.bag(world)["resp"] = a.ask(world, "zqx" + a.word(world) + "nomatch")


@then("the response has no product cards and a non-empty reply naming no listing title")
def no_match(world: World) -> None:
    resp = a.bag(world)["resp"]
    data = a.body(resp)
    assert a.cards(resp) == [], a.cards(resp)
    assert data.get("replyText"), data
    assert "blender" not in data["replyText"] and " control" not in data["replyText"], data


@then(
    'the request id is 32 lowercase hex characters and the model version matches "mock:assistant-v1|" '
    "followed by the search model version"
)
def provenance(world: World) -> None:
    data = a.body(a.bag(world)["resp"])
    assert a.REQUEST_ID.match(data.get("requestId", "")), data
    assert a.MODEL_VERSION.match(data.get("modelVersion", "")), data


@given("team-search is stopped")
def search_stopped(world: World) -> None:
    restore = stop_container(SETTINGS.search_container)

    def _restore() -> None:
        restore()
        wait_for_search(SETTINGS.gateway_url)

    a.bag(world)["restart"] = _restore
    world.add_cleanup(_restore)


@when("a buyer asks the assistant for that title word")
def ask_word(world: World) -> None:
    a.bag(world)["resp"] = a.ask(world, a.word(world))


@then(
    'the call succeeds with the retrieval-failed reply, no product cards and the model version "mock:assistant-v1"'
)
def degraded(world: World) -> None:
    resp = a.bag(world)["resp"]
    data = a.body(resp)
    assert data.get("replyText"), data
    assert a.cards(resp) == [], a.cards(resp)
    assert data.get("modelVersion") == a.RETRIEVAL_FAILED_MODEL_VERSION, data
    assert a.REQUEST_ID.match(data.get("requestId", "")), data


@then("the control card comes back once team-search is restored")
def card_returns(world: World) -> None:
    a.bag(world)["restart"]()
    a.wait_returned(world, a.word(world), "keep")


def _wait_assistant(world: World, message: str, timeout_s: float = 120) -> httpx.Response:
    deadline = time.monotonic() + timeout_s
    last = "no answer"
    while time.monotonic() < deadline:
        try:
            resp = a.ask(world, message)
            if resp.status_code == 200:
                return resp
            last = f"{resp.status_code}: {resp.text[:200]}"
        except httpx.HTTPError as exc:
            last = str(exc)
        time.sleep(2)
    raise TimeoutError(f"the assistant did not answer after the team-ai restart ({last})")


@given("team-ai runs with RAG disabled")
def rag_off(world: World) -> None:
    s.buyer_of(world)

    def restore() -> None:
        apr.dc("up", "-d", "--no-deps", "team-ai")
        pe.wait_healthy(pe.ai_container(), 120)

    world.add_cleanup(restore)
    apr.dc("-f", str(RAG_OFF_OVERLAY), "up", "-d", "--no-deps", "team-ai")
    pe.wait_healthy(pe.ai_container(), 120)
    assert (
        apr.ai_env().get("RAG_ENABLED", "").lower() == "false"
    ), "team-ai did not pick up RAG_ENABLED=false; is the wrapper applying the overlay?"
    _wait_assistant(world, "ready?")


@when("a buyer asks the assistant for a t-shirt")
def ask_tshirt(world: World) -> None:
    a.bag(world)["resp"] = _wait_assistant(world, "áo thun")


@then("every returned card id is a listing that exists in team-domain")
def only_real_listings(world: World) -> None:
    for card_id in a.card_ids(a.bag(world)["resp"]):
        assert a.domain_listing(card_id)["id"] == card_id, card_id
