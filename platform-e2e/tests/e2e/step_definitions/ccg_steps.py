"""Steps for the ai-copilot-grounding scenarios (change ai-copilot-grounding)."""

from __future__ import annotations

from pytest_bdd import given, parsers, then, when

from tests.e2e.support import ccg_support as c
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World


@given(parsers.parse("the seller has a published listing with stock {stock:d}"))
def own_listing(world: World, stock: int) -> None:
    c.publish(world, "own", stock=stock)


@given(parsers.parse("another seller has a published listing with stock {stock:d}"))
def foreign_listing(world: World, stock: int) -> None:
    c.publish(world, "foreign", stock=stock, seller=c.other_seller(world))


@when(parsers.parse('the seller asks the copilot "{message}" about the listing'))
def ask_own(world: World, message: str) -> None:
    c.bag(world)["resp"] = c.copilot(world, message, s.listing(world, "own").id)


@when(parsers.parse('the seller asks the copilot "{message}" about the other seller\'s listing'))
def ask_foreign(world: World, message: str) -> None:
    c.bag(world)["resp"] = c.copilot(world, message, s.listing(world, "foreign").id)


@then("no draft claims the item is available and at least one draft says it is out of stock")
def out_of_stock_drafts(world: World) -> None:
    got = c.drafts(c.bag(world)["resp"])
    assert got, got
    assert not [d for d in got if c.AVAILABILITY.search(d)], got
    assert [d for d in got if c.OUT_OF_STOCK.search(d)], got


@then("no draft claims the item is available")
def no_availability_claim(world: World) -> None:
    got = c.drafts(c.bag(world)["resp"])
    assert got, got
    assert not [d for d in got if c.AVAILABILITY.search(d)], got


@then("no draft contains the other seller's listing title or id")
def no_foreign_fields(world: World) -> None:
    resp = c.bag(world)["resp"]
    foreign = s.listing(world, "foreign")
    text = c.ok(resp).__repr__()
    assert foreign.id not in text and foreign.title not in text, text


@then("no draft states a delivery duration, cut-off hour or same-day dispatch")
def no_delivery_promise(world: World) -> None:
    got = c.drafts(c.bag(world)["resp"])
    assert got, got
    assert not [d for d in got if c.DELIVERY_PROMISE.search(d)], got


@then("at least one draft asks the buyer for the delivery address")
def asks_address(world: World) -> None:
    got = c.drafts(c.bag(world)["resp"])
    assert [d for d in got if c.ADDRESS_ASK.search(d)], got


@then('the response has a 32-character hex request id and the model version "rules-v1"')
def copilot_provenance(world: World) -> None:
    body = c.ok(c.bag(world)["resp"])
    assert c.REQUEST_ID.match(body.get("requestId", "")), body
    assert body.get("modelVersion") == c.RULES_VERSION, body


@when("the seller makes 200 sequential copilot calls about the listing")
def latency_calls(world: World) -> None:
    c.bag(world)["lat"] = c.timed_calls(
        world, "còn hàng không?", s.listing(world, "own").id, c.LATENCY_CALLS
    )


@then("the 95th percentile latency is at most 300 ms")
def latency_budget(world: World) -> None:
    p = c.p95(c.bag(world)["lat"])
    assert p <= c.LATENCY_P95_S, f"p95 {p * 1000:.0f} ms"


@when("the seller calls Magic Listing with a title and a category")
def call_magic(world: World) -> None:
    c.bag(world)["magic"] = c.magic(world)


@then('the Magic Listing response has a 32-character hex request id and the model version "rules-v1"')
def magic_provenance(world: World) -> None:
    body = c.ok(c.bag(world)["magic"])
    assert c.REQUEST_ID.match(body.get("requestId", "")), body
    assert body.get("modelVersion") == c.RULES_VERSION, body
    assert body.get("generatedDescription"), body
