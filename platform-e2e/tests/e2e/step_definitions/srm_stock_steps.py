"""Steps for search/search_stock_read_model.feature (change port-search-read-model-correctness)."""

from __future__ import annotations

import json

from pytest_bdd import given, parsers, then, when

from tests.e2e.flows import srm_events_flow as ev
from tests.e2e.flows.oic_inv_flow import register
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

MAIN = "main"


# ── givens ───────────────────────────────────────────────────────────────
@given(parsers.parse("a published listing with stock {stock:d} that appears in search"))
def listing_in_search(world: World, stock: int) -> None:
    s.create_listing(world, MAIN, stock=stock)
    s.wait_indexed(world, MAIN)


@given(
    parsers.parse(
        "a published listing with stock {stock:d} whose search stock is {shown:d} "
        "after a real checkout of quantity {qty:d}"
    )
)
def listing_after_checkout(world: World, stock: int, shown: int, qty: int) -> None:
    s.create_listing(world, MAIN, stock=stock)
    s.wait_indexed(world, MAIN)
    order = s.checkout(world, s.buyer_of(world), MAIN, qty)
    s.ctx(world).extra["order_id"] = order["id"]
    s.wait_stock(world, MAIN, shown)


@given(parsers.parse("a published listing whose search stock is {stock:d}"))
def listing_with_search_stock(world: World, stock: int) -> None:
    s.create_listing(world, MAIN, stock=stock)
    s.wait_indexed(world, MAIN)
    s.wait_stock(world, MAIN, stock)


@given(parsers.parse("a buyer has saved a search whose query matches a listing with stock {n:d}"))
def saved_search_matching(world: World, n: int) -> None:
    s.create_listing(world, MAIN, stock=n)
    s.wait_indexed(world, MAIN)
    saver = register(world, "buyer")
    resp = s.save_search(saver.token, s.ctx(world).keyword)
    s.ctx(world).saved_id = s.ok_json(resp)["savedSearch"]["id"]
    s.ctx(world).extra["saver"] = saver


@given("a sold-out listing and an in-stock listing that both match a query")
def sold_out_and_in_stock(world: World) -> None:
    s.create_listing(world, "soldout", stock=1)
    s.create_listing(world, "instock", stock=5)
    s.wait_indexed(world, "soldout")
    s.wait_indexed(world, "instock")
    s.checkout(world, s.buyer_of(world), "soldout", 1)
    s.wait_stock(world, "soldout", 0)


@given("a buyer has saved that query with filters_json in_stock true")
def saved_in_stock(world: World) -> None:
    saver = register(world, "buyer")
    resp = s.save_search(saver.token, s.ctx(world).keyword, json.dumps({"in_stock": "true"}))
    s.ctx(world).saved_id = s.ok_json(resp)["savedSearch"]["id"]
    s.ctx(world).extra["saver"] = saver


# ── whens ────────────────────────────────────────────────────────────────
@when(parsers.parse("a buyer checks out quantity {qty:d} of it through the gateway"))
def buyer_checks_out(world: World, qty: int) -> None:
    s.checkout(world, s.buyer_of(world), MAIN, qty)


@when(parsers.parse("another buyer checks out quantity {qty:d} of that listing"))
def other_buyer_checks_out(world: World, qty: int) -> None:
    s.checkout(world, s.second_buyer(world), MAIN, qty)


@when("the buyer cancels the order through the gateway")
def buyer_cancels(world: World) -> None:
    s.cancel_order(s.buyer_of(world), s.ctx(world).extra["order_id"])


@when(
    "a ListingStockChanged with stock 10 and an occurred_at earlier than that checkout "
    "event is published to listing.events"
)
def stale_stock_event(world: World) -> None:
    checkout_event = s.last_real_stock_event(world, MAIN, 8)
    stale_ns = checkout_event.occurred_at_ns - 1_000_000_000
    value = ev.stock_changed(s.listing(world, MAIN).id, 10, stale_ns)
    s.ctx(world).event = ev.publish_and_wait(s.listing(world, MAIN).id, value, s.CONSUME_S)


@when(
    "a valid ListingStockChanged with stock 5 for a listing id that was never created is published to listing.events"
)
def stock_event_unknown(world: World) -> None:
    import uuid

    ghost = str(uuid.uuid4())
    s.ctx(world).extra["ghost"] = ghost
    s.ctx(world).event = ev.publish_and_wait(
        ghost, ev.stock_changed(ghost, 5, ev.now_ns()), s.CONSUME_S
    )


@when("a ListingStockChanged for that listing with stock -1 is published to listing.events")
def malformed_stock_event(world: World) -> None:
    lid = s.listing(world, MAIN).id
    s.ctx(world).event = ev.publish(lid, ev.stock_changed(lid, -1, ev.now_ns()))


@when(
    "a ListingChanged UPDATED with a new title, stock 10 and an occurred_at between its "
    "creation and that checkout is published to listing.events"
)
def late_listing_update(world: World) -> None:
    created = s.real_event(world, MAIN, ev.LISTING_CHANGED, ev.CREATED)
    checkout_event = s.last_real_stock_event(world, MAIN, 8)
    between = (created.occurred_at_ns + checkout_event.occurred_at_ns) // 2
    new_title = f"{s.ctx(world).keyword} renamed"
    s.ctx(world).extra["new_title"] = new_title
    value = ev.listing_changed(
        ev.UPDATED, s.craft_listing(world, MAIN, title=new_title, stock=10), between
    )
    s.ctx(world).event = ev.publish_and_wait(s.listing(world, MAIN).id, value, s.CONSUME_S)


@when(parsers.parse("the seller updates the listing's stock to {stock:d} through the gateway"))
def seller_edits_stock(world: World, stock: int) -> None:
    lid = s.listing(world, MAIN).id
    seller = s.seller_of(world)
    current = s.ok_json(s.post(s.LISTING + "GetListing", {"id": lid}))["listing"]
    current["stock"] = stock
    s.ok_json(s.post(s.LISTING + "UpdateListing", {"listing": current}, seller.token))


@when("a buyer runs that query with the in-stock filter in semantic mode, then in hybrid mode")
def in_stock_semantic_hybrid(world: World) -> None:
    c = s.ctx(world)
    c.resps = [
        s.search(c.keyword, filters={"in_stock": "true"}, mode=mode)
        for mode in ("SEARCH_MODE_SEMANTIC", "SEARCH_MODE_HYBRID")
    ]


@when("the buyer runs the saved search through the gateway")
def run_saved_search(world: World) -> None:
    c = s.ctx(world)
    c.resp = s.run_saved(c.extra["saver"].token, c.saved_id)


@when(
    "a buyer calls SearchListings with in_stock maybe, then SaveSearch with filters_json in_stock yes"
)
def invalid_in_stock(world: World) -> None:
    c = s.ctx(world)
    c.extra["caller"] = register(world, "buyer")
    token = c.extra["caller"].token
    c.resps = [
        s.search(c.keyword, token=token, filters={"in_stock": "maybe"}),
        s.save_search(token, c.keyword, json.dumps({"in_stock": "yes"})),
    ]


# ── thens ────────────────────────────────────────────────────────────────
@then(parsers.parse("within 30 seconds search returns that listing with stock {stock:d}"))
def search_shows_stock(world: World, stock: int) -> None:
    s.wait_stock(world, MAIN, stock)


@then(parsers.parse("within 30 seconds RunSavedSearch returns that listing with stock {stock:d}"))
def saved_run_shows_stock(world: World, stock: int) -> None:
    c = s.ctx(world)
    lid = s.listing(world, MAIN).id
    s.eventually(
        lambda: s.stock_of_hit(s.hit_of(s.run_saved(c.extra["saver"].token, c.saved_id), lid))
        == stock,
        f"RunSavedSearch stock to be {stock}",
    )


@then(
    "after the search indexer has consumed past that event, search still returns that listing "
    "with stock 8"
)
def stock_still_eight(world: World) -> None:
    s.wait_stock(world, MAIN, 8)


@then(
    "after the search indexer has consumed past that event, the read-model holds no document "
    "for that id and no search returns it"
)
def ghost_not_created(world: World) -> None:
    ghost = s.ctx(world).extra["ghost"]
    assert ev.os_doc(ghost) is None, f"a stock event created a document for {ghost}"
    assert ghost not in s.hit_ids(s.search(ghost))
    assert ghost not in s.hit_ids(s.search("", filters={"in_stock": "true"}, page_size=100))


@then(
    "within 60 seconds that record appears on listing.events.dlq and search still returns "
    "the listing with stock 10"
)
def parked_not_applied(world: World) -> None:
    c = s.ctx(world)
    lid = s.listing(world, MAIN).id

    def parked() -> bool:
        return any(
            r.type == ev.STOCK_CHANGED and r.offset >= 0
            for r in ev.dlq_records_for(lid, c.started_ms)
        )

    s.eventually(parked, f"the malformed stock event for {lid} on {ev.DLQ_TOPIC}", s.DLQ_S)
    assert s.stock_of_hit(s.hit_of(s.search(s.listing(world, MAIN).title), lid)) == 10


@then("within 30 seconds a search for the new title returns that listing with stock 8")
def new_title_stock_eight(world: World) -> None:
    c = s.ctx(world)
    lid = s.listing(world, MAIN).id
    s.eventually(
        lambda: s.stock_of_hit(s.hit_of(s.search(c.extra["new_title"]), lid)) == 8,
        "the renamed listing to show stock 8",
    )


@then(
    "within 30 seconds an in-stock search for that query no longer returns or counts it while "
    "the same search without the filter returns it with stock 0"
)
def sold_out_hidden(world: World) -> None:
    c = s.ctx(world)
    lid = s.listing(world, MAIN).id
    # positive control first: the unfiltered read shows the sold-out listing with stock 0
    s.wait_stock(world, MAIN, 0)
    filtered = s.search(c.keyword, filters={"in_stock": "true"})
    assert lid not in s.hit_ids(filtered), f"sold-out listing returned: {filtered.text}"
    assert s.total_of(filtered) == 0, f"total still counts it: {filtered.text}"


@then("both responses return the in-stock listing and neither returns the sold-out one")
def modes_filter(world: World) -> None:
    c = s.ctx(world)
    for resp in c.resps:
        ids = s.hit_ids(resp)
        assert s.listing(world, "instock").id in ids, resp.text
        assert s.listing(world, "soldout").id not in ids, resp.text


@then("the response returns the in-stock listing with its stock and not the sold-out one")
def saved_in_stock_result(world: World) -> None:
    c = s.ctx(world)
    ids = s.hit_ids(c.resp)
    assert s.listing(world, "soldout").id not in ids, c.resp.text
    hit = s.hit_of(c.resp, s.listing(world, "instock").id)
    assert hit is not None, c.resp.text
    assert s.stock_of_hit(hit) == 5, hit


@then("both calls fail with invalid_argument and no saved search is created")
def both_invalid(world: World) -> None:
    c = s.ctx(world)
    for resp in c.resps:
        assert resp.status_code == 400 and resp.json().get("code") == "invalid_argument", (
            resp.status_code,
            resp.text,
        )
    assert s.list_saved(c.extra["caller"].token) == []
