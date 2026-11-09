"""Steps for search/search_query_correctness.feature (change port-search-read-model-correctness)."""

from __future__ import annotations

import uuid

from pytest_bdd import given, then, when

from tests.e2e.flows import srm_events_flow as ev
from tests.e2e.step_definitions import search_best_selling_rating_steps as bs
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
    # The last candidate is always kept: deleting it too left no B at all (a small A id made
    # every try miss, ~46% for an A id starting with "1"), and B is never searchable.
    tries = 12
    for i in range(tries):
        b = s.create_listing(world, "B", stock=5)
        if b.id < a.id or i == tries - 1:
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


@given("published listings matching one keyword with rating_avg 4.5, 3.5, 2 and no review")
def rated_listings(world: World) -> None:
    bs.rated_listings(world)


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
    "a buyer calls SearchListings for the keyword with min_rating 4, then with min_rating 4 and "
    "SEARCH_MODE_SEMANTIC"
)
def min_rating_calls(world: World) -> None:
    c = s.ctx(world)
    token = bs.buyer_token(world)
    c.resps = [
        s.search(c.keyword, token=token, min_rating=4),
        s.search(c.keyword, token=token, min_rating=4, mode="SEARCH_MODE_SEMANTIC"),
    ]


@when("a buyer calls SearchListings for the keyword with min_rating 0")
def min_rating_zero(world: World) -> None:
    c = s.ctx(world)
    c.resps = [s.search(c.keyword, token=bs.buyer_token(world), min_rating=0)]


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


@then("both calls succeed and return only the listing with rating_avg 4.5")
def accepted_only_top_rated(world: World) -> None:
    top = s.listing(world, "A").id
    for resp in s.ctx(world).resps:
        assert resp.status_code == 200, (resp.status_code, resp.text)
        assert s.hit_ids(resp) == [top], resp.text


@then(
    "the ratings facet is 4 with count 1 and 3 with count 2 while the categories facet is "
    "not empty"
)
def ratings_facet_filled(world: World) -> None:
    resp = s.ctx(world).resps[0]
    assert bs.facet_pairs(resp, "ratings") == [("4", 1), ("3", 2)], resp.text
    assert bs.facet_pairs(resp, "categories"), resp.text
