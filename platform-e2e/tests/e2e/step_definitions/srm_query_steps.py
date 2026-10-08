"""Steps for search/search_query_correctness.feature (change port-search-read-model-correctness)."""

from __future__ import annotations

import uuid

from pytest_bdd import given, then, when

from tests.e2e.flows import srm_events_flow as ev
from tests.e2e.flows.oic_inv_flow import register
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

NEWEST = "SORT_BY_NEWEST"


def _order(world: World, resp) -> list[str]:
    wanted = {s.listing(world, "A").id, s.listing(world, "B").id}
    return [i for i in s.hit_ids(resp) if i in wanted]


# ── givens ───────────────────────────────────────────────────────────────
@given(
    "a seller creates and publishes listing A, then listing B, then updates listing A's "
    "description, all with a shared unique keyword in the title"
)
def a_then_b_then_update_a(world: World) -> None:
    a = s.create_listing(world, "A", stock=5)
    s.wait_indexed(world, "A")
    seller = s.seller_of(world)
    # Adversarial ids: the pre-change sort was `_id desc`, so a B whose id sorts below A's makes
    # the old code answer A, B. Candidates that do not qualify are deleted (the order of creation
    # A then B is unchanged); the corrected sort passes whatever the ids are.
    for _ in range(12):
        b = s.create_listing(world, "B", stock=5)
        if b.id < a.id:
            break
        s.ok_json(s.post(s.LISTING + "DeleteListing", {"id": b.id}, seller.token))
    current = s.ok_json(s.post(s.LISTING + "GetListing", {"id": a.id}))["listing"]
    current["description"] = "updated after B was created"
    s.ok_json(s.post(s.LISTING + "UpdateListing", {"listing": current}, seller.token))
    # positive control: both are indexed and A's update has reached the read-model, so a
    # wrong order below is the engine's, not a listing that is not there yet
    s.wait_indexed(world, "B")
    s.eventually(
        lambda: (ev.os_doc(a.id) or {}).get("description") == "updated after B was created",
        "listing A's update in the read-model",
        90.0,
    )


@given(
    "a seller has created and published a real listing whose title carries a unique keyword, "
    "and a listing id that the read-model does not hold"
)
def real_and_unknown(world: World) -> None:
    s.create_listing(world, "R", stock=5)
    s.wait_indexed(world, "R")
    real = s.listing(world, "R").id
    ghost = str(uuid.uuid4())
    while ghost >= real:  # pre-change `_id desc` order would put the real listing first
        ghost = str(uuid.uuid4())
    s.ctx(world).extra["ghost"] = ghost


@given("published listings that match a query")
def published_matching(world: World) -> None:
    s.create_listing(world, "A", stock=5)
    s.create_listing(world, "B", stock=5)
    s.wait_indexed(world, "A")
    s.wait_indexed(world, "B")


# ── whens ────────────────────────────────────────────────────────────────
@when("a buyer searches for that keyword with SORT_BY_NEWEST through the gateway")
def newest_default(world: World) -> None:
    s.ctx(world).resp = None  # observed by polling in the Then


@when(
    "a buyer searches for that keyword with SORT_BY_NEWEST and SEARCH_MODE_HYBRID, then with "
    "SORT_BY_NEWEST and no search mode"
)
def newest_hybrid_and_default(world: World) -> None:
    c = s.ctx(world)
    c.resps = [
        s.search(c.keyword, sort_by=NEWEST, mode="SEARCH_MODE_HYBRID"),
        s.search(c.keyword, sort_by=NEWEST),
    ]


@when(
    "a ListingChanged UPDATED for that id with occurred_at T2 is published to listing.events, "
    "followed by its ListingChanged CREATED with occurred_at T1, where the real listing's "
    "creation is before T1 and T1 is before T2"
)
def late_create(world: World) -> None:
    c = s.ctx(world)
    real_created = s.real_event(world, "R", ev.LISTING_CHANGED, ev.CREATED).occurred_at_ns
    t1 = real_created + 2_000_000_000
    t2 = t1 + 2_000_000_000
    ghost = c.extra["ghost"]
    seller_id = s.subject(s.seller_of(world).token)
    updated_title = f"{c.keyword} out-of-order updated"
    c.extra["ghost_title"] = updated_title
    updated = ev.listing_msg(ghost, title=updated_title, seller_id=seller_id, stock=5)
    created = ev.listing_msg(
        ghost, title=f"{c.keyword} out-of-order created", seller_id=seller_id, stock=5
    )
    ev.publish(ghost, ev.listing_changed(ev.UPDATED, updated, t2))
    c.event = ev.publish(ghost, ev.listing_changed(ev.CREATED, created, t1))
    ev.wait_consumed(*c.event, s.CONSUME_S)


@when(
    "a buyer calls SearchListings through the gateway with min_rating 4, then with min_rating 4 "
    "and SEARCH_MODE_SEMANTIC"
)
def min_rating_calls(world: World) -> None:
    c = s.ctx(world)
    c.resps = [
        s.search(c.keyword, min_rating=4),
        s.search(c.keyword, min_rating=4, mode="SEARCH_MODE_SEMANTIC"),
    ]


@when(
    "a buyer calls SearchListings for that query with min_rating 0, and runs a saved search "
    "with the same query"
)
def search_and_saved(world: World) -> None:
    c = s.ctx(world)
    buyer = register(world, "buyer")
    saved = s.ok_json(s.save_search(buyer.token, c.keyword))
    c.resps = [
        s.search(c.keyword, token=buyer.token, min_rating=0),
        s.run_saved(buyer.token, saved["savedSearch"]["id"]),
    ]


# ── thens ────────────────────────────────────────────────────────────────
@then("within 30 seconds the hits are B then A")
def hits_b_then_a(world: World) -> None:
    c = s.ctx(world)
    expected = [s.listing(world, "B").id, s.listing(world, "A").id]
    s.eventually(
        lambda: _order(world, s.search(c.keyword, sort_by=NEWEST)) == expected,
        "SORT_BY_NEWEST to return B then A",
    )


@then("both responses return B then A")
def both_b_then_a(world: World) -> None:
    expected = [s.listing(world, "B").id, s.listing(world, "A").id]
    for resp in s.ctx(world).resps:
        assert _order(world, resp) == expected, resp.text


@then(
    "within 30 seconds a SORT_BY_NEWEST search for that keyword returns the out-of-order "
    "listing first, with the UPDATED event's title, and the real listing second"
)
def ghost_first(world: World) -> None:
    c = s.ctx(world)
    ghost, real = c.extra["ghost"], s.listing(world, "R").id
    s.eventually(
        lambda: s.hit_ids(s.search(c.keyword, sort_by=NEWEST))[:2] == [ghost, real],
        "the out-of-order listing first and the real listing second",
    )
    assert (ev.os_doc(ghost) or {}).get("title") == c.extra["ghost_title"], ev.os_doc(ghost)


@then("both calls fail with invalid_argument")
def both_invalid(world: World) -> None:
    for resp in s.ctx(world).resps:
        assert resp.status_code == 400 and resp.json().get("code") == "invalid_argument", (
            resp.status_code,
            resp.text,
        )


@then(
    "both responses return the matching listings and an empty ratings facet while the "
    "categories facet is not empty"
)
def empty_ratings(world: World) -> None:
    ids = {s.listing(world, "A").id, s.listing(world, "B").id}
    for resp in s.ctx(world).resps:
        body = s.ok_json(resp)
        assert ids <= set(s.hit_ids(resp)), resp.text
        facets = body.get("facets", {})
        assert facets.get("ratings", []) == [], f"ratings facet not empty: {facets.get('ratings')}"
        assert facets.get("categories"), f"categories facet empty: {facets}"
