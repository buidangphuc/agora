"""Binds domain/seller_erasure.feature (change domain-seller-erasure, initiative trust-legal).

A UserErased envelope is produced to the real identity.events topic; every effect is read through the
gateway, the broker (listing.events, identity.events.team-domain.dlq, consumer-group lag) and, where no
RPC reaches it, listing_db (counts, and seeding of states no RPC can produce).
"""

from __future__ import annotations

import time
import uuid

from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.support import erasure_events_support as ev
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import seller_erasure_support as se
from tests.e2e.support import srm_support as s
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

scenarios("domain/seller_erasure.feature")

INDEX_S = 90.0
EFFECT_S = 60.0


def _x(world: World) -> dict:
    x = world.state.extra.setdefault("sel", {})
    x.setdefault("sellers", {})
    x.setdefault("listings", {})
    x.setdefault("owned", {})
    x.setdefault("events", {})
    x.setdefault("since_ms", int(time.time() * 1000) - 5000)
    return x


def _seller(world: World, name: str) -> dict:
    return _x(world)["sellers"][name]


def _lid(world: World, name: str) -> str:
    return _x(world)["listings"][name][0]


def _title(world: World, name: str) -> str:
    return _x(world)["listings"][name][1]


def _new_seller(world: World, name: str) -> dict:
    x = _x(world)
    user = se.new_seller(f"sel_{name.lower()}")
    x["sellers"][name] = user
    x["owned"][name] = []
    return user


def _add(world: World, seller: str, label: str, variants: int = 0) -> str:
    x = _x(world)
    lid, title = se.create_listing(_seller(world, seller), variants=variants)
    x["listings"][label] = (lid, title)
    x["owned"][seller].append(lid)

    def cleanup() -> None:
        try:
            se.psql(f"DELETE FROM listings WHERE id::text = {se.lit(lid)}")
        except AssertionError:
            pass

    world.add_cleanup(cleanup)
    return lid


def _findable(world: World, label: str) -> None:
    lid, title = _x(world)["listings"][label]
    s.eventually(lambda: lid in se.search_hits(title), "listing in search", timeout_s=INDEX_S)


def _publish(world: World, name: str, **kw) -> str:
    eid = ev.publish_user_erased(_seller(world, name)["id"], **kw)
    _x(world)["events"].setdefault(name, []).append(eid)
    return eid


# ── Givens ───────────────────────────────────────────────────────────────
@given(
    parsers.parse(
        'an erasure seller "{name}" with listings "P1" and "P2" published, "R" pending_review, '
        '"D" draft, "H" held, "B" banned and "V" published with 2 variants'
    )
)
def seller_all_statuses(world: World, name: str) -> None:
    _new_seller(world, name)
    for label in ("P1", "P2", "R", "D", "H", "B"):
        _add(world, name, label)
    _add(world, name, "V", variants=2)
    for label, status in (("R", "pending_review"), ("D", "draft"), ("H", "held"), ("B", "banned")):
        se.set_status(_lid(world, label), status)
    assert se.count_listings(_seller(world, name)["id"]) == 7, "seeded listings missing"
    assert se.count_variants(_seller(world, name)["id"]) == 2, "seeded variants missing"


@given(
    parsers.parse('an erasure seller "{name}" with a published listing "{label}" that is findable')
)
def seller_findable(world: World, name: str, label: str) -> None:
    _new_seller(world, name)
    _add(world, name, label)
    _findable(world, label)


@given(parsers.parse('an erasure seller "{name}" with a published listing "{label}"'))
def seller_listing(world: World, name: str, label: str) -> None:
    _new_seller(world, name)
    _add(world, name, label)
    assert se.get_listing(_lid(world, label)).status_code == 200


@given(parsers.parse('an erasure seller "{name}" with no listings'))
def seller_none(world: World, name: str) -> None:
    _new_seller(world, name)
    assert se.count_listings(_seller(world, name)["id"]) == 0


@given(
    parsers.parse(
        'an erasure seller "{name}" with a published listing "{label}" cloned to {n:d} listings'
    )
)
def seller_cloned(world: World, name: str, label: str, n: int) -> None:
    _new_seller(world, name)
    _add(world, name, label)
    se.clone_listings(_lid(world, label), n - 1)
    assert se.count_listings(_seller(world, name)["id"]) == n
    _x(world)["owned"][name] = se.seller_listing_ids(_seller(world, name)["id"])


@given(parsers.parse('the erasure seller "{name}" has a storefront and {n:d} bundles'))
def seller_shop(world: World, name: str, n: int) -> None:
    x = _x(world)
    seller = _seller(world, name)
    slug = f"sel-{uuid.uuid4().hex[:10]}"
    se.upsert_storefront(seller, slug)
    for i in range(n):
        se.create_bundle(seller, x["owned"][name][:1], f"Bundle {i}")
    x.setdefault("slugs", {})[name] = slug
    assert se.storefront_code(slug) == 200, "storefront not readable before the erasure"
    assert len(se.bundles_of(seller["id"])) == n, "bundles not listed before the erasure"


# ── Whens ────────────────────────────────────────────────────────────────
@when(parsers.parse('a UserErased event for the erasure seller "{name}" is consumed'))
def when_erased(world: World, name: str) -> None:
    _publish(world, name, roles=("seller",))


@when(parsers.parse('a UserErased event with no roles for the erasure seller "{name}" is consumed'))
def when_erased_no_roles(world: World, name: str) -> None:
    _publish(world, name, roles=())


@when(parsers.parse('the same UserErased event for the erasure seller "{name}" is delivered again'))
def when_again(world: World, name: str) -> None:
    ev.wait_group_caught_up(se.ERASURE_GROUP)
    eid = _x(world)["events"][name][0]
    ev.publish_user_erased(_seller(world, name)["id"], event_id=eid, roles=("seller",))
    _x(world)["events"][name].append(eid)


@when(
    parsers.parse(
        'a UserErased event with a new event id for the erasure seller "{name}" is consumed'
    )
)
def when_new_id(world: World, name: str) -> None:
    ev.wait_group_caught_up(se.ERASURE_GROUP)
    _publish(world, name, roles=("seller",))


@when(
    parsers.parse(
        'seller "{who}" calls DeleteListing on "{label}" and an anonymous caller calls DeleteListing on "{label2}"'
    )
)
def when_foreign_delete(world: World, who: str, label: str, label2: str) -> None:
    x = _x(world)
    x["denied"] = [
        s.post(
            se.LISTING + "DeleteListing", {"id": _lid(world, label)}, _seller(world, who)["token"]
        ),
        s.post(se.LISTING + "DeleteListing", {"id": _lid(world, label2)}),
    ]


@when(
    "an undecodable record, a UserErased with an empty user_id and a UserErased without erased_at are produced"
)
def when_malformed(world: World) -> None:
    x = _x(world)
    x["marker"] = f"e2e-garbage-{uuid.uuid4().hex}"
    ev.publish_garbage(x["marker"])
    x["empty_event"] = ev.publish_user_erased("")
    x["no_erased_at"] = ev.new_event_id()
    owner = _seller(world, "A")["id"]
    stack.produce(
        ev.TOPIC,
        key=owner.encode(),
        value=se.encode_user_erased_without_erased_at(owner, x["no_erased_at"]),
    )


@when(parsers.parse('the admin suspends the erasure seller "{name}"'))
def when_suspend(world: World, name: str) -> None:
    se.produce_status_changed_via_admin(world, _seller(world, name)["id"], "suspended")


# ── Thens ────────────────────────────────────────────────────────────────
@then(parsers.parse('{n:d} listings and {m:d} variants remain for the erasure seller "{name}"'))
def then_none_remain(world: World, n: int, m: int, name: str) -> None:
    sid = _seller(world, name)["id"]
    ev.wait_for(
        lambda: (se.count_listings(sid), se.count_variants(sid)),
        lambda v: v == (n, m),
        f"listings and variants of {name} after UserErased",
        timeout=EFFECT_S,
    )


@then(parsers.parse('exactly 1 DELETED event exists for each of the {n:d} listings of "{name}"'))
def then_one_deleted_each(world: World, n: int, name: str) -> None:
    x = _x(world)
    expected = set(x["owned"][name])
    assert len(expected) == n, f"expected {n} listings, tracked {len(expected)}"
    ev.wait_group_caught_up(se.ERASURE_GROUP)
    got = se.wait_deleted(sorted(expected), x["since_ms"])
    dupes = {i: len(got[i]) for i in expected if len(got[i]) != 1}
    assert not dupes, f"listings with a DELETED event count other than 1: {dupes}"


@then(parsers.parse('within {sec:d} seconds a search for the title of "{label}" returns 0 results'))
def then_not_searchable(world: World, sec: int, label: str) -> None:
    lid, title = _x(world)["listings"][label]
    s.eventually(
        lambda: lid not in se.search_hits(title), "listing gone from search", timeout_s=float(sec)
    )
    assert se.search_hits(title) == [], "search still returns results for the erased title"


@then(parsers.parse('GetListing for "{label}" returns not_found'))
def then_get_not_found(world: World, label: str) -> None:
    lid = _lid(world, label)
    ev.wait_for(
        lambda: se.get_listing(lid).status_code,
        lambda c: c == 404,
        f"GetListing {lid} should be not_found",
        timeout=EFFECT_S,
    )
    assert se.get_listing(lid).json().get("code") == "not_found"


@then(
    parsers.parse('the DELETED event of "{label}" carries its id and title and principal "{who}"')
)
def then_event_shape(world: World, label: str, who: str) -> None:
    lid, title = _x(world)["listings"][label]
    got = se.wait_deleted([lid], _x(world)["since_ms"])
    env = got[lid][0]
    assert env["listing_id"] == lid
    assert title.encode() in env["raw"], "event carries no pre-delete snapshot (title missing)"
    assert who.encode() in env["principal"], f"principal is {env['principal']!r}, not {who}"


@then(
    parsers.parse(
        'the listing "{label}" of seller "{name}" is still published and has no DELETED event'
    )
)
def then_still_published(world: World, label: str, name: str) -> None:
    ev.wait_group_caught_up(se.ERASURE_GROUP)
    lid = _lid(world, label)
    resp = se.get_listing(lid)
    assert (
        resp.status_code == 200 and resp.json()["listing"]["status"] == "LISTING_STATUS_PUBLISHED"
    ), resp.text
    assert lid not in se.deleted_envelopes(
        _x(world)["since_ms"]
    ), "DELETED event written for another seller"


@then("the calls are denied with permission_denied and unauthenticated")
def then_denied(world: World) -> None:
    codes = [ue.code_of(r) for r in _x(world)["denied"]]
    assert codes == ["permission_denied", "unauthenticated"], codes


@then(parsers.parse('the listing "{label}" still exists'))
def then_exists(world: World, label: str) -> None:
    assert se.get_listing(_lid(world, label)).status_code == 200


@then(parsers.parse('the listing "{label}" of seller "{name}" still exists'))
def then_exists_owner(world: World, label: str, name: str) -> None:
    ev.wait_group_caught_up(se.ERASURE_GROUP)
    resp = se.get_listing(_lid(world, label), _seller(world, name)["token"])
    assert resp.status_code == 200, f"listing of {name} gone: {resp.status_code} {resp.text}"


@then(parsers.parse('the erasure consumer completes the events of "{name}" without a DLQ record'))
@then("the erasure consumer completes both deliveries without a DLQ record")
def then_no_dlq(world: World, name: str = "A") -> None:
    ev.wait_group_caught_up(se.ERASURE_GROUP)
    for eid in _x(world)["events"].get(name, []):
        ev.assert_dlq_empty(se.ERASURE_DLQ, eid.encode())
    ev.assert_dlq_empty(se.ERASURE_DLQ, _seller(world, name)["id"].encode())


@then(parsers.parse('no DELETED event names a listing of "{name}"'))
def then_no_deleted(world: World, name: str) -> None:
    ev.wait_group_caught_up(se.ERASURE_GROUP)
    got = se.deleted_envelopes(_x(world)["since_ms"])
    assert not set(got) & set(_x(world)["owned"][name])


@then(parsers.parse('GetStorefront for the former slug of "{name}" returns not_found'))
def then_storefront_gone(world: World, name: str) -> None:
    slug = _x(world)["slugs"][name]
    ev.wait_for(
        lambda: se.storefront_code(slug),
        lambda c: c == 404,
        "storefront still readable",
        timeout=EFFECT_S,
    )


@then(parsers.parse('{n:d} bundles exist for the erasure seller "{name}"'))
def then_bundles(world: World, n: int, name: str) -> None:
    sid = _seller(world, name)["id"]
    ev.wait_for(
        lambda: len(se.bundles_of(sid)), lambda c: c == n, f"bundles of {name}", timeout=EFFECT_S
    )


@then(parsers.parse('the storefront of "{name}" and its {n:d} bundles are unchanged'))
def then_shop_unchanged(world: World, name: str, n: int) -> None:
    ev.wait_group_caught_up(se.ERASURE_GROUP)
    assert se.storefront_code(_x(world)["slugs"][name]) == 200
    assert len(se.bundles_of(_seller(world, name)["id"])) == n


@then("all three records appear on identity.events.team-domain.dlq")
def then_three_in_dlq(world: World) -> None:
    x = _x(world)
    for what, needle in (
        ("undecodable record", x["marker"]),
        ("empty user_id event", x["empty_event"]),
        ("event without erased_at", x["no_erased_at"]),
    ):
        assert ev.wait_dlq_record(
            se.ERASURE_DLQ, needle.encode()
        ), f"{what} not on {se.ERASURE_DLQ}"
    ev.wait_group_caught_up(se.ERASURE_GROUP)
    assert se.count_listings(_seller(world, "A")["id"]) >= 1, "a malformed event deleted listings"


@then(parsers.parse('within {sec:d} seconds the listing "{label}" is absent from search'))
def then_absent(world: World, sec: int, label: str) -> None:
    lid, title = _x(world)["listings"][label]
    s.eventually(
        lambda: lid not in se.search_hits(title),
        "held listing gone from search",
        timeout_s=float(sec),
    )


@then(
    parsers.parse('the erasure group has no lag and the DLQ holds nothing for the seller "{name}"')
)
def then_group_idle(world: World, name: str) -> None:
    ev.wait_group_caught_up(se.ERASURE_GROUP)
    ev.assert_dlq_empty(se.ERASURE_DLQ, _seller(world, name)["id"].encode())


@then(
    parsers.parse(
        'the team-domain logs of the erasure mention the erasure and contain neither the user id nor any listing id of "{name}"'
    )
)
def then_logs(world: World, name: str) -> None:
    ev.wait_group_caught_up(se.ERASURE_GROUP)
    se.wait_deleted(_x(world)["owned"][name], _x(world)["since_ms"])
    logs = se.domain_logs(int(time.time() - _x(world)["since_ms"] / 1000) + 30)
    lines = [ln for ln in logs.splitlines() if "erasure" in ln.lower() or "erased" in ln.lower()]
    assert lines, "no erasure log line found in the team-domain logs"
    secrets = [_seller(world, name)["id"], *_x(world)["owned"][name]]
    leaked = [sec for sec in secrets if sec in logs]
    assert not leaked, f"raw identifiers in team-domain logs: {leaked}"
