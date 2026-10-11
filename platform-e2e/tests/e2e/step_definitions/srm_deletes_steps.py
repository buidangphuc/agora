"""Steps for search/search_read_model_deletes.feature (change port-search-read-model-correctness)."""

from __future__ import annotations

import json
import time

from pytest_bdd import given, then, when

from tests.e2e.flows import srm_events_flow as ev
from tests.e2e.flows.oic_inv_flow import register
from tests.e2e.support import oic_inv_stack as stack
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

VICTIM = "victim"
CONTROL = "control"
OTHER = "other"


def _gone(world: World, label: str = VICTIM) -> None:
    """The listing is absent from a keyword search while its control sibling is still present
    (the control proves the read path is live, so the absence means something)."""
    c = s.ctx(world)
    resp = s.search(s.listing(world, label).title)
    ids = s.hit_ids(resp)
    assert s.listing(world, label).id not in ids, f"deleted listing returned: {resp.text}"
    control = s.search(s.listing(world, CONTROL).title)
    assert s.listing(world, CONTROL).id in s.hit_ids(
        control
    ), f"control listing missing, the absence proves nothing: {control.text}"
    assert c.keyword


# ── givens ───────────────────────────────────────────────────────────────
@given("a seller's published listing that appears in search next to a control listing")
def victim_and_control(world: World) -> None:
    s.create_listing(world, VICTIM, stock=5)
    s.create_listing(world, CONTROL, stock=5)
    s.wait_indexed(world, VICTIM)
    s.wait_indexed(world, CONTROL)


@given("that listing is deleted through the gateway")
def victim_deleted(world: World) -> None:
    s.delete_and_wait_consumed(world, VICTIM)


@given(
    "a seller with two published listings that a seller_id search counts as total 2 and a sellers facet of 2"
)
def seller_with_two(world: World) -> None:
    seller = s.seller_of(world)
    s.create_listing(world, VICTIM, stock=5)
    s.create_listing(world, OTHER, stock=5)
    sid = s.subject(seller.token)

    def two() -> bool:
        body = s.ok_json(s.search("", filters={"seller_id": sid}))
        sellers = {
            b["key"]: int(b.get("count", 0)) for b in body.get("facets", {}).get("sellers", [])
        }
        return s.total_of(s.search("", filters={"seller_id": sid})) == 2 and sellers.get(sid) == 2

    s.eventually(two, "the seller_id search to count 2 listings and a sellers facet of 2", 90.0)


@given("a seller's draft listing that appears in the owner's draft search next to a control draft")
def draft_and_control(world: World) -> None:
    s.create_listing(world, VICTIM, stock=5, status="LISTING_STATUS_DRAFT")
    s.create_listing(world, CONTROL, stock=5, status="LISTING_STATUS_DRAFT")
    token = s.seller_of(world).token
    sid = s.subject(token)

    def both() -> bool:
        ids = s.hit_ids(s.search("", token=token, filters={"status": "draft", "seller_id": sid}))
        return s.listing(world, VICTIM).id in ids and s.listing(world, CONTROL).id in ids

    s.eventually(both, "both drafts to appear in the owner's draft search", 90.0)


@given("the search indexer runs with a short tombstone TTL and purge interval")
def short_ttl(world: World) -> None:
    env = stack.container_env("team-search-indexer")
    ttl = stack.parse_go_duration(env.get("TOMBSTONE_TTL", ""))
    interval = stack.parse_go_duration(env.get("TOMBSTONE_PURGE_INTERVAL", ""))
    assert ttl and interval and ttl + interval <= 120, (
        "the short-tombstone overlay is not active on team-search-indexer "
        f"(TOMBSTONE_TTL={env.get('TOMBSTONE_TTL')!r}, "
        f"TOMBSTONE_PURGE_INTERVAL={env.get('TOMBSTONE_PURGE_INTERVAL')!r}); start the stack with "
        "`docker compose -f docker-compose.yaml -f "
        "platform-e2e/compose/search-tombstones.override.yaml up -d team-search-indexer`"
    )
    s.ctx(world).extra["purge_wait_s"] = ttl + interval + 8.0


# ── whens ────────────────────────────────────────────────────────────────
@when(
    "the seller deletes it through the gateway and a stale ListingChanged UPDATED published for it "
    "with an occurred_at before the delete is published to listing.events"
)
def delete_then_stale_update(world: World) -> None:
    delete = s.delete_and_wait_consumed(world, VICTIM)
    stale = delete.occurred_at_ns - 2_000_000_000
    value = ev.listing_changed(
        ev.UPDATED,
        s.craft_listing(world, VICTIM, title=s.listing(world, VICTIM).title, stock=5),
        stale,
    )
    s.ctx(world).event = ev.publish_and_wait(s.listing(world, VICTIM).id, value, s.CONSUME_S)


@when(
    "the original ListingChanged CREATED record for it is read from listing.events and "
    "published again unchanged"
)
def republish_create(world: World) -> None:
    c = s.ctx(world)
    created = s.real_event(world, VICTIM, ev.LISTING_CHANGED, ev.CREATED)
    c.event = ev.republish(created)
    ev.wait_consumed(*c.event, s.CONSUME_S)


@when(
    "a ListingStatusChanged with status PUBLISHED and an occurred_at after the delete is "
    "published to listing.events"
)
def newer_status_event(world: World) -> None:
    delete = s.ctx(world).extra[f"delete:{VICTIM}"]
    after = max(ev.now_ns(), delete.occurred_at_ns + 1_000_000_000)
    lid = s.listing(world, VICTIM).id
    s.ctx(world).event = ev.publish_and_wait(
        lid, ev.status_changed(lid, ev.PUBLISHED, after), s.CONSUME_S
    )


@when(
    "a valid ListingStockChanged with stock 5 and an occurred_at after the delete is "
    "published to listing.events"
)
def newer_stock_event(world: World) -> None:
    delete = s.ctx(world).extra[f"delete:{VICTIM}"]
    after = max(ev.now_ns(), delete.occurred_at_ns + 1_000_000_000)
    lid = s.listing(world, VICTIM).id
    s.ctx(world).event = ev.publish_and_wait(lid, ev.stock_changed(lid, 5, after), s.CONSUME_S)


@when("the seller deletes one of them through the gateway")
def delete_one_of_two(world: World) -> None:
    s.delete_and_wait_consumed(world, VICTIM)


@when("the seller deletes the draft through the gateway")
def delete_draft(world: World) -> None:
    s.delete_listing(world, VICTIM)


# ── thens ────────────────────────────────────────────────────────────────
@then(
    "after the search indexer has consumed past that event, neither SearchListings for its "
    "title nor Suggest for its title prefix returns it"
)
def not_searchable_nor_suggested(world: World) -> None:
    c = s.ctx(world)
    _gone(world)
    victim = s.listing(world, VICTIM)
    assert victim.title not in s.suggestions(c.keyword), "Suggest returned the deleted listing"


@then(
    "after the search indexer has consumed past the copy, SearchListings for its title does "
    "not return it"
)
def copy_consumed_not_returned(world: World) -> None:
    _gone(world)


@then(
    "after the search indexer has consumed past that event, SearchListings for its title does "
    "not return it"
)
def event_consumed_not_returned(world: World) -> None:
    _gone(world)


@then(
    "after the search indexer has consumed past that event, an in-stock search for its title "
    "does not return it and the record is not on listing.events.dlq"
)
def in_stock_not_returned_not_parked(world: World) -> None:
    c = s.ctx(world)
    victim = s.listing(world, VICTIM)
    resp = s.search(victim.title, filters={"in_stock": "true"})
    assert victim.id not in s.hit_ids(resp), f"deleted listing returned: {resp.text}"
    parked = [r for r in ev.dlq_records_for(victim.id, c.started_ms) if r.type == ev.STOCK_CHANGED]
    assert not parked, f"the stock event for a deleted listing was parked: {parked}"
    _gone(world)


@then(
    "within 30 seconds the same search returns total 1, only the remaining listing and a "
    "sellers facet of 1, and a saved search with that filter run by a buyer returns only the "
    "remaining listing"
)
def totals_and_facets_after_delete(world: World) -> None:
    sid = s.subject(s.seller_of(world).token)
    remaining = s.listing(world, OTHER).id
    gone = s.listing(world, VICTIM).id

    def settled() -> bool:
        resp = s.search("", filters={"seller_id": sid})
        body = s.ok_json(resp)
        sellers = {
            b["key"]: int(b.get("count", 0)) for b in body.get("facets", {}).get("sellers", [])
        }
        return s.hit_ids(resp) == [remaining] and s.total_of(resp) == 1 and sellers.get(sid) == 1

    s.eventually(settled, "total 1, one hit and a sellers facet of 1 after the delete")
    buyer = register(world, "buyer")
    saved = s.ok_json(s.save_search(buyer.token, "", json.dumps({"seller_id": sid})))
    run = s.run_saved(buyer.token, saved["savedSearch"]["id"])
    ids = s.hit_ids(run)
    assert gone not in ids and remaining in ids, f"saved search returned {ids}: {run.text}"


@then(
    "within 30 seconds that owner search no longer returns it, and a search with status "
    "deleted fails with invalid_argument"
)
def draft_gone(world: World) -> None:
    token = s.seller_of(world).token
    sid = s.subject(token)
    victim, control = s.listing(world, VICTIM).id, s.listing(world, CONTROL).id

    def gone() -> bool:
        ids = s.hit_ids(s.search("", token=token, filters={"status": "draft", "seller_id": sid}))
        return victim not in ids and control in ids

    s.eventually(gone, "the deleted draft to leave the owner's draft view (control draft kept)")
    resp = s.search("", token=token, filters={"status": "deleted", "seller_id": sid})
    assert resp.status_code == 400 and resp.json().get("code") == "invalid_argument", resp.text


@then(
    "the read-model holds a tombstone right after the delete is consumed, no document once the "
    "TTL plus one purge interval has passed, and the other listing still appears in search"
)
def tombstone_then_purged(world: World) -> None:
    c = s.ctx(world)
    lid = s.listing(world, VICTIM).id
    doc = ev.os_doc(lid)
    assert doc is not None and doc.get("status") == "deleted", f"no tombstone after delete: {doc}"
    deadline = time.monotonic() + c.extra["purge_wait_s"]
    s.eventually(
        lambda: ev.os_doc(lid) is None,
        f"the tombstone of {lid} to be purged",
        max(deadline - time.monotonic(), 1.0),
    )
    other = s.listing(world, OTHER)
    assert other.id in s.hit_ids(s.search(other.title)), "the other listing left search"
    assert ev.os_doc(other.id) is not None
