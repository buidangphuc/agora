"""Steps for search/search_best_selling_rating.feature (change search-bestseller-rating).

Black box: ListingSalesChanged and ListingRatingChanged are produced to order.events and
engagement.events, the projection is observed through SearchListings at the gateway and, for
parked events, on the `search-indexer.<topic>.dlq` topics. Waiting for the indexer uses the committed offset of its
consumer group on the topic of the event, never a sleep.
"""

from __future__ import annotations

import json
import struct
import time
import uuid
from dataclasses import dataclass
from typing import Any

from pytest_bdd import given, parsers, then, when

from tests.e2e.flows import srm_events_flow as ev
from tests.e2e.flows.oic_inv_flow import register
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

ORDER_TOPIC = "order.events"
ENGAGEMENT_TOPIC = "engagement.events"
SALES_TYPE = "platform.order.v1.ListingSalesChanged"
RATING_TYPE = "platform.engagement.v1.ListingRatingChanged"
CONSUME_S = 45.0
DLQ_S = 60.0
BEST_SELLING = "SORT_BY_BEST_SELLING"
SEMANTIC = "SEARCH_MODE_SEMANTIC"
HYBRID = "SEARCH_MODE_HYBRID"
_BASE_NS = time.time_ns() - 7_200 * 1_000_000_000


def at(k: int) -> int:
    return _BASE_NS + k * 60 * 1_000_000_000


@dataclass
class Fact:
    kind: str
    topic: str
    key: str
    raw: bytes
    event_id: str
    partition: int
    offset: int


def _envelope(type_name: str, payload: bytes, event_id: str, occurred_ns: int) -> bytes:
    return (
        ev._str(1, event_id)
        + ev._str(2, type_name)
        + ev._bytes(3, ev.timestamp(occurred_ns))
        + ev._bytes(7, payload)
    )


def _facts(world: World) -> list[Fact]:
    return s.ctx(world).extra.setdefault("facts", [])


def _record(world: World, kind: str, topic: str, key: str, raw: bytes, event_id: str) -> Fact:
    partition, offset = ev.publish(key, raw, topic=topic)
    fact = Fact(kind, topic, key, raw, event_id, partition, offset)
    _facts(world).append(fact)
    s.ctx(world).extra[f"last:{kind}"] = fact
    return fact


def send_sales(world: World, listing_id: str, sold: int, changed_ns: int | None) -> Fact:
    event_id = str(uuid.uuid4())
    payload = (ev._str(1, listing_id) if listing_id else b"") + ev._int(2, sold)
    if changed_ns is not None:
        payload += ev._bytes(3, ev.timestamp(changed_ns))
    raw = _envelope(SALES_TYPE, payload, event_id, time.time_ns())
    return _record(world, "sales", ORDER_TOPIC, listing_id, raw, event_id)


def send_rating(
    world: World, listing_id: str, avg: float, count: int, changed_ns: int | None
) -> Fact:
    event_id = str(uuid.uuid4())
    payload = (ev._str(1, listing_id) if listing_id else b"") + ev._varint((2 << 3) | 1)
    payload += struct.pack("<d", float(avg)) + ev._int(3, count)
    if changed_ns is not None:
        payload += ev._bytes(4, ev.timestamp(changed_ns))
    raw = _envelope(RATING_TYPE, payload, event_id, time.time_ns())
    return _record(world, "rating", ENGAGEMENT_TOPIC, listing_id, raw, event_id)


def committed_offset(topic: str, partition: int) -> int:
    from confluent_kafka import Consumer, TopicPartition

    consumer = Consumer(
        {
            "bootstrap.servers": ev.brokers(),
            "group.id": ev.indexer_group(),
            "enable.auto.commit": False,
        }
    )
    try:
        return consumer.committed([TopicPartition(topic, partition)], timeout=10)[0].offset
    finally:
        consumer.close()


def wait_consumed(fact: Fact, timeout_s: float = CONSUME_S) -> None:
    deadline = time.monotonic() + timeout_s
    last = -1
    while time.monotonic() < deadline:
        last = committed_offset(fact.topic, fact.partition)
        if last > fact.offset:
            return
        time.sleep(0.5)
    raise AssertionError(
        f"the {ev.indexer_group()} group did not consume {fact.topic}[{fact.partition}]"
        f"@{fact.offset} within {timeout_s:.0f}s (committed offset {last})"
    )


def wait_all_consumed(world: World) -> None:
    for fact in _facts(world):
        wait_consumed(fact)


def topic_has(topic: str, needle: bytes, since_ms: int) -> bool:
    from confluent_kafka import OFFSET_BEGINNING, Consumer, TopicPartition

    consumer = Consumer(
        {
            "bootstrap.servers": ev.brokers(),
            "group.id": f"e2e-sbr-{uuid.uuid4().hex}",
            "enable.auto.commit": False,
        }
    )
    try:
        meta = consumer.list_topics(topic, timeout=10).topics.get(topic)
        if meta is None or meta.error is not None:
            return False
        starts, ends = [], {}
        for partition in meta.partitions:
            low, high = consumer.get_watermark_offsets(TopicPartition(topic, partition), 10)
            if high <= low:
                continue
            resolved = consumer.offsets_for_times(
                [TopicPartition(topic, partition, since_ms)], timeout=10
            )[0]
            if resolved.offset < 0:
                continue
            starts.append(resolved)
            ends[partition] = high
        if not starts:
            return False
        consumer.assign(starts)
        deadline = time.monotonic() + 15.0
        while ends and time.monotonic() < deadline:
            msg = consumer.poll(0.5)
            if msg is None or msg.error():
                continue
            if needle in (msg.value() or b""):
                return True
            if msg.offset() + 1 >= ends.get(msg.partition(), 0):
                ends.pop(msg.partition(), None)
        return False
    finally:
        consumer.close()


def on_dlq(world: World, fact: Fact) -> bool:
    return topic_has(
        "search-indexer." + fact.topic + ".dlq", fact.event_id.encode(), s.ctx(world).started_ms
    )


def buyer_token(world: World) -> str:
    c = s.ctx(world)
    if "buyer_token" not in c.extra:
        c.extra["buyer_token"] = register(world, "buyer").token
    return c.extra["buyer_token"]


def num(hit: dict[str, Any] | None, key: str) -> float:
    return float((hit or {}).get(key, 0))


def hit_for(world: World, label: str, query: str | None = None, token: str | None = None):
    c = s.ctx(world)
    return s.hit_of(s.search(query or c.keyword, token=token), s.listing(world, label).id)


def wait_hit(world: World, label: str, **expected: float) -> None:
    def ok() -> bool:
        hit = hit_for(world, label)
        return hit is not None and all(abs(num(hit, k) - v) < 1e-9 for k, v in expected.items())

    s.eventually(ok, f"the hit of {label} to carry {expected}", s.VISIBLE_S)


def ids_of(world: World, labels: str) -> list[str]:
    return [s.listing(world, x.strip()).id for x in labels.split(",")]


def create_indexed(world: World, label: str, **kw: Any) -> s.Listed:
    listed = s.create_listing(world, label, stock=5, **kw)
    s.wait_indexed(world, label)
    return listed


def ghost(world: World, label: str) -> s.Listed:
    c = s.ctx(world)
    listed = s.Listed(str(uuid.uuid4()), f"{c.keyword} {label}")
    c.listings[label] = listed
    return listed


def publish_created(
    world: World, label: str, occurred_ns: int | None = None, title: str | None = None
) -> tuple[int, int]:
    listed = s.listing(world, label)
    seller_id = s.subject(s.seller_of(world).token)
    msg = ev.listing_msg(listed.id, title=title or listed.title, seller_id=seller_id, stock=5)
    pos = ev.publish(
        listed.id, ev.listing_changed(ev.CREATED, msg, occurred_ns or time.time_ns())
    )
    ev.wait_consumed(*pos, s.CONSUME_S)
    return pos


def rated_listings(world: World) -> None:
    for label in ("A", "B", "C", "D"):
        s.create_listing(world, label, stock=5)
    for label in ("A", "B", "C", "D"):
        s.wait_indexed(world, label)
    for label, avg, count in (("A", 4.5, 2), ("B", 3.5, 1), ("C", 2.0, 1)):
        send_rating(world, s.listing(world, label).id, avg, count, at(1))
    wait_hit(world, "A", ratingAvg=4.5, reviewCount=2)
    wait_hit(world, "B", ratingAvg=3.5, reviewCount=1)
    wait_hit(world, "C", ratingAvg=2.0, reviewCount=1)


def search_all(world: World, **kw: Any) -> list:
    token = buyer_token(world)
    return [s.search(s.ctx(world).keyword, token=token, **kw)]


def facet_pairs(resp, name: str) -> list[tuple[str, int]]:
    buckets = s.ok_json(resp).get("facets", {}).get(name, [])
    return [(b["key"], int(b.get("count", 0))) for b in buckets]


@given(parsers.parse('a published listing "{label}" that is indexed'))
def published_indexed(world: World, label: str) -> None:
    create_indexed(world, label)
    hit = hit_for(world, label)
    assert num(hit, "soldCount") == 0 and num(hit, "reviewCount") == 0, hit


@given(parsers.parse('the listing "{label}" has sold_count {n:d} from a sales event at T{k:d}'))
def has_sold_count(world: World, label: str, n: int, k: int) -> None:
    send_sales(world, s.listing(world, label).id, n, at(k))
    wait_hit(world, label, soldCount=n)


@given(
    parsers.parse(
        'the listing "{label}" has rating_avg {avg:g} and review_count {n:d} from a rating event at T{k:d}'
    )
)
def has_rating(world: World, label: str, avg: float, n: int, k: int) -> None:
    send_rating(world, s.listing(world, label).id, avg, n, at(k))
    wait_hit(world, label, ratingAvg=avg, reviewCount=n)


@given(
    parsers.parse(
        'a listing id "{label}" that has no search document and a control listing "{control}" that is indexed'
    )
)
def ghost_and_control(world: World, label: str, control: str) -> None:
    ghost(world, label)
    create_indexed(world, control)
    assert ev.os_doc(s.listing(world, label).id) is None


@given('published listings "A", "B" and "C" matching one keyword with sold counts 7, 0 and 3')
def abc_sold(world: World) -> None:
    for label in ("A", "B", "C"):
        s.create_listing(world, label, stock=5)
    for label in ("A", "B", "C"):
        s.wait_indexed(world, label)
    for label, sold in (("A", 7), ("B", 0), ("C", 3)):
        send_sales(world, s.listing(world, label).id, sold, at(1))
    wait_hit(world, "A", soldCount=7)
    wait_hit(world, "C", soldCount=3)
    wait_all_consumed(world)


@given(
    'listings "D", "E" and "F" with sold_count 2, where D was created at 10:00:00, E and F at '
    "10:05:00, and the id of E sorts before the id of F"
)
def tie_listings(world: World) -> None:
    ids = sorted(str(uuid.uuid4()) for _ in range(3))
    c = s.ctx(world)
    for label, lid in zip(("D", "E", "F"), ids):
        c.listings[label] = s.Listed(lid, f"{c.keyword} {label}")
    base = time.time_ns() - 3_600 * 1_000_000_000
    publish_created(world, "D", base)
    publish_created(world, "E", base + 300 * 1_000_000_000)
    publish_created(world, "F", base + 300 * 1_000_000_000)
    for label in ("D", "E", "F"):
        s.wait_indexed(world, label)
        send_sales(world, s.listing(world, label).id, 2, time.time_ns())
    for label in ("D", "E", "F"):
        wait_hit(world, label, soldCount=2)


@given(
    'a published listing "N" that does not match the keyword and has sold_count 50'
)
def unrelated_listing(world: World) -> None:
    title = "zsbrother" + uuid.uuid4().hex[:10]
    resp = s.post(
        s.LISTING + "CreateListing",
        {
            "listing": {
                "title": title,
                "categoryId": "cat-laptop",
                "price": 100_000,
                "stock": 5,
                "status": "LISTING_STATUS_PUBLISHED",
                "currency": "VND",
                "description": "sbr e2e",
            }
        },
        s.seller_of(world).token,
    )
    assert resp.status_code == 200, resp.text
    listed = s.Listed(resp.json()["listing"]["id"], title)
    s.ctx(world).listings["N"] = listed
    s.wait_indexed(world, "N", title)
    send_sales(world, listed.id, 50, at(1))
    s.eventually(
        lambda: num(s.hit_of(s.search(title), listed.id), "soldCount") == 50,
        "the sold count of N",
        s.VISIBLE_S,
    )


@given(
    'a draft listing "D" owned by seller S with sold_count 100 and a published listing "P" '
    "with sold_count 1"
)
def draft_and_published(world: World) -> None:
    c = s.ctx(world)
    s.create_listing(world, "D", stock=5, status="LISTING_STATUS_DRAFT")
    s.create_listing(world, "P", stock=5)
    s.wait_indexed(world, "P")
    token = s.seller_of(world).token
    sid = s.subject(token)
    send_sales(world, s.listing(world, "D").id, 100, at(1))
    send_sales(world, s.listing(world, "P").id, 1, at(1))

    def owner_sees() -> bool:
        resp = s.search(c.keyword, token=token, filters={"status": "draft", "seller_id": sid})
        hit = s.hit_of(resp, s.listing(world, "D").id)
        return hit is not None and num(hit, "soldCount") == 100

    s.eventually(owner_sees, "the owner's draft view to carry the draft's sold_count 100", 60.0)
    wait_hit(world, "P", soldCount=1)


@given(
    'published listings "A", "B", "C" and "D" matching one keyword with rating_avg 4.5, 3.5, 2 '
    "and no review"
)
def rated_abcd(world: World) -> None:
    rated_listings(world)


@given(
    parsers.parse(
        'a published listing "{label}" with rating_avg {avg:g} and review_count {n:d}'
    )
)
def listing_with_rating(world: World, label: str, avg: float, n: int) -> None:
    create_indexed(world, label)
    fact = send_rating(world, s.listing(world, label).id, avg, n, at(1))
    if n:
        wait_hit(world, label, ratingAvg=avg, reviewCount=n)
    else:
        wait_consumed(fact)


@given(parsers.parse('a saved search for the keyword with min_rating {n:d}'))
def saved_with_min_rating(world: World, n: int) -> None:
    c = s.ctx(world)
    saved = s.ok_json(
        s.save_search(buyer_token(world), c.keyword, json.dumps({"min_rating": str(n)}))
    )
    c.saved_id = saved["savedSearch"]["id"]


@given(
    parsers.parse(
        'a published listing "{label}" with sold_count {sold:d}, rating_avg {avg:g} and review_count {n:d}'
    )
)
def listing_with_all(world: World, label: str, sold: int, avg: float, n: int) -> None:
    create_indexed(world, label)
    lid = s.listing(world, label).id
    send_sales(world, lid, sold, at(1))
    send_rating(world, lid, avg, n, at(1))
    wait_hit(world, label, soldCount=sold, ratingAvg=avg, reviewCount=n)


@given(
    'a published listing "L" that no event has touched and a published listing "M" with '
    "sold_count 1"
)
def untouched_and_sold(world: World) -> None:
    create_indexed(world, "L")
    create_indexed(world, "M")
    send_sales(world, s.listing(world, "M").id, 1, at(1))
    wait_hit(world, "M", soldCount=1)


@given('a published listing "L" and a control listing "C" that are indexed')
def published_and_control(world: World) -> None:
    create_indexed(world, "L")
    create_indexed(world, "C")


@given('"L" is deleted through the gateway')
def listing_deleted(world: World) -> None:
    s.delete_and_wait_consumed(world, "L")


@when(
    parsers.parse(
        'a ListingSalesChanged for "{label}" with sold_count {n:d} and changed_at T{k:d} is published to order.events'
    )
)
def publish_sales(world: World, label: str, n: int, k: int) -> None:
    send_sales(world, s.listing(world, label).id, n, at(k))


@when(
    parsers.parse(
        'a ListingRatingChanged for "{label}" with average_rating {avg:g}, review_count {n:d} and changed_at T{k:d} is published to engagement.events'
    )
)
def publish_rating(world: World, label: str, avg: float, n: int, k: int) -> None:
    send_rating(world, s.listing(world, label).id, avg, n, at(k))


@when(
    parsers.parse(
        'a ListingSalesChanged for "{label}" with sold_count {n:d} and changed_at T{k:d} is published and the T{j:d} event is delivered again'
    )
)
def publish_older_and_redeliver(world: World, label: str, n: int, k: int, j: int) -> None:
    first = _facts(world)[0]
    send_sales(world, s.listing(world, label).id, n, at(k))
    _record(world, first.kind, first.topic, first.key, first.raw, first.event_id)


@when(
    'a ListingChanged CREATED for "G" is published to listing.events'
)
def create_ghost(world: World) -> None:
    publish_created(world, "G")


@when(
    'a ListingChanged UPDATED for "L" with a new title is published to listing.events'
)
def update_title(world: World) -> None:
    c = s.ctx(world)
    listed = s.listing(world, "L")
    c.extra["new_title"] = f"{c.keyword} renamed"
    msg = ev.listing_msg(
        listed.id,
        title=c.extra["new_title"],
        seller_id=s.subject(s.seller_of(world).token),
        stock=5,
    )
    pos = ev.publish(listed.id, ev.listing_changed(ev.UPDATED, msg, time.time_ns()))
    ev.wait_consumed(*pos, s.CONSUME_S)


@when(
    'a ListingSalesChanged with sold_count -1 for "L" is published to order.events and a '
    "ListingRatingChanged with average_rating 7 for \"L\" is published to engagement.events"
)
def publish_malformed(world: World) -> None:
    lid = s.listing(world, "L").id
    send_sales(world, lid, -1, at(2))
    send_rating(world, lid, 7.0, 1, at(2))


@when(
    'a ListingSalesChanged with an empty listing_id and a ListingRatingChanged for "L" with no '
    "changed_at are published"
)
def publish_empty_and_no_changed_at(world: World) -> None:
    send_sales(world, "", 3, at(2))
    send_rating(world, s.listing(world, "L").id, 1.0, 9, None)


@when(
    'a ListingSalesChanged for "L" with sold_count 2 and a ListingRatingChanged for "L" with '
    "average_rating 4 and review_count 1, both with changed_at earlier than the delete, are "
    "published"
)
def publish_before_delete(world: World) -> None:
    deleted = s.ctx(world).extra["delete:L"]
    earlier = deleted.occurred_at_ns - 5_000_000_000
    lid = s.listing(world, "L").id
    send_sales(world, lid, 2, earlier)
    send_rating(world, lid, 4.0, 1, earlier)


@when(
    'a ListingChanged CREATED for "L" with occurred_at after the delete is published to '
    "listing.events"
)
def create_after_delete(world: World) -> None:
    deleted = s.ctx(world).extra["delete:L"]
    publish_created(world, "L", max(time.time_ns(), deleted.occurred_at_ns + 1_000_000_000))


@when("a buyer calls SearchListings for the keyword with SORT_BY_BEST_SELLING")
def search_best_selling(world: World) -> None:
    s.ctx(world).resps = search_all(world, sort_by=BEST_SELLING)


@when(
    "a buyer calls SearchListings for the keyword with SORT_BY_BEST_SELLING and "
    "SEARCH_MODE_SEMANTIC, then with SEARCH_MODE_HYBRID"
)
def search_best_selling_modes(world: World) -> None:
    c = s.ctx(world)
    token = buyer_token(world)
    c.resps = [
        s.search(c.keyword, token=token, sort_by=BEST_SELLING, mode=SEMANTIC),
        s.search(c.keyword, token=token, sort_by=BEST_SELLING, mode=HYBRID),
    ]


@when(
    parsers.parse(
        "a buyer calls SearchListings for the keyword with min_rating {a:d}, then with min_rating {b:d}"
    )
)
def search_two_ratings(world: World, a: int, b: int) -> None:
    s.ctx(world).resps = search_all(world, min_rating=a) + search_all(world, min_rating=b)


@when(parsers.parse("a buyer calls SearchListings for the keyword with min_rating {a:d}"))
def search_one_rating(world: World, a: int) -> None:
    s.ctx(world).resps = search_all(world, min_rating=a)


@when(
    "a buyer calls SearchListings with min_rating 4 and SEARCH_MODE_SEMANTIC, then with "
    "SEARCH_MODE_HYBRID, and runs the saved search"
)
def search_rating_modes_and_saved(world: World) -> None:
    c = s.ctx(world)
    token = buyer_token(world)
    c.resps = [
        s.search(c.keyword, token=token, min_rating=4, mode=SEMANTIC),
        s.search(c.keyword, token=token, min_rating=4, mode=HYBRID),
        s.run_saved(token, c.saved_id),
    ]


@when(
    "a buyer calls SearchListings through the gateway with min_rating 6, then with min_rating -1"
)
def search_out_of_range(world: World) -> None:
    c = s.ctx(world)
    token = buyer_token(world)
    c.resps = [
        s.search(c.keyword, token=token, min_rating=6),
        s.search(c.keyword, token=token, min_rating=-1),
    ]


@when(parsers.parse('a buyer calls SearchListings for the title of "{label}"'))
def search_title(world: World, label: str) -> None:
    c = s.ctx(world)
    c.resps = [s.search(s.listing(world, label).title, token=buyer_token(world))]


@then(parsers.parse('within 30 seconds the hit of "{label}" carries sold_count {n:d}'))
def hit_sold(world: World, label: str, n: int) -> None:
    wait_hit(world, label, soldCount=n)


@then(
    parsers.parse(
        'within 30 seconds the hit of "{label}" carries rating_avg {avg:g} and review_count {n:d}'
    )
)
def hit_rating(world: World, label: str, avg: float, n: int) -> None:
    wait_hit(world, label, ratingAvg=avg, reviewCount=n)


@then(
    parsers.parse(
        'within 30 seconds the hit of "{label}" carries sold_count {n:d} and review_count {m:d}'
    )
)
def hit_sold_and_reviews(world: World, label: str, n: int, m: int) -> None:
    wait_hit(world, label, soldCount=n, reviewCount=m)


@then(
    parsers.parse(
        'after the indexer has consumed the published events, the hit of "{label}" still carries sold_count {n:d}'
    )
)
def hit_still_sold(world: World, label: str, n: int) -> None:
    wait_all_consumed(world)
    assert num(hit_for(world, label), "soldCount") == n


@then(
    parsers.parse(
        'after the indexer has consumed the published events, a keyword search returns no hit for "{label}" and counts only "{control}"'
    )
)
def only_control(world: World, label: str, control: str) -> None:
    wait_all_consumed(world)
    resp = s.search(s.ctx(world).keyword)
    assert s.hit_ids(resp) == [s.listing(world, control).id], resp.text
    assert s.total_of(resp) == 1, resp.text


@then('within 30 seconds the read-model title of "L" is the new title')
def new_title(world: World) -> None:
    lid = s.listing(world, "L").id
    expected = s.ctx(world).extra["new_title"]
    s.eventually(
        lambda: (ev.os_doc(lid) or {}).get("title") == expected,
        "the read-model title of L to be the new title",
        s.VISIBLE_S,
    )


@then(
    "within 60 seconds the sales event is on search-indexer.order.events.dlq and the rating "
    "event is on search-indexer.engagement.events.dlq"
)
def both_parked(world: World) -> None:
    c = s.ctx(world)
    sales, rating = c.extra["last:sales"], c.extra["last:rating"]
    s.eventually(lambda: on_dlq(world, sales), "the sales event on search-indexer.order.events.dlq", DLQ_S)
    s.eventually(
        lambda: on_dlq(world, rating), "the rating event on search-indexer.engagement.events.dlq", DLQ_S
    )


@then(
    parsers.parse(
        'the hit of "{label}" still carries sold_count {n:d} and rating_avg {avg:g}'
    )
)
def hit_unchanged_both(world: World, label: str, n: int, avg: float) -> None:
    hit = hit_for(world, label)
    assert num(hit, "soldCount") == n and num(hit, "ratingAvg") == avg, hit


@then(parsers.parse('the hit of "{label}" still carries rating_avg {avg:g}'))
def hit_unchanged_rating(world: World, label: str, avg: float) -> None:
    assert num(hit_for(world, label), "ratingAvg") == avg


@then(
    'after the indexer has consumed the published events, "L" is absent from the hits, the '
    "total and the facets of a keyword search"
)
def absent_everywhere(world: World) -> None:
    wait_all_consumed(world)
    resp = s.search(s.ctx(world).keyword)
    lid, control = s.listing(world, "L").id, s.listing(world, "C").id
    assert lid not in s.hit_ids(resp) and control in s.hit_ids(resp), resp.text
    assert s.total_of(resp) == 1, resp.text
    assert sum(c for _, c in facet_pairs(resp, "categories")) == 1, resp.text


@then(parsers.parse("the hits are {labels} in that order"))
def hits_in_order(world: World, labels: str) -> None:
    expected = ids_of(world, labels)
    for resp in s.ctx(world).resps:
        assert s.hit_ids(resp) == expected, resp.text


@then('"D" is not among the hits and "P" is')
def draft_hidden(world: World) -> None:
    resp = s.ctx(world).resps[0]
    ids = s.hit_ids(resp)
    assert s.listing(world, "P").id in ids, resp.text
    assert s.listing(world, "D").id not in ids, resp.text


@then(
    parsers.parse(
        "the first response returns only {first} and the second returns only {second}"
    )
)
def first_and_second_only(world: World, first: str, second: str) -> None:
    resps = s.ctx(world).resps
    assert set(s.hit_ids(resps[0])) == set(ids_of(world, first)), resps[0].text
    assert set(s.hit_ids(resps[1])) == set(ids_of(world, second)), resps[1].text


@then(parsers.parse("the response returns only {labels}"))
def response_only(world: World, labels: str) -> None:
    resp = s.ctx(world).resps[0]
    assert set(s.hit_ids(resp)) == set(ids_of(world, labels)), resp.text


@then(parsers.parse("every response returns only {labels}"))
def every_response_only(world: World, labels: str) -> None:
    for resp in s.ctx(world).resps:
        assert set(s.hit_ids(resp)) == set(ids_of(world, labels)), resp.text


@then(parsers.parse('"{label}" is among the hits'))
def among_hits(world: World, label: str) -> None:
    resp = s.ctx(world).resps[0]
    assert s.listing(world, label).id in s.hit_ids(resp), resp.text


@then(parsers.parse('"{label}" is not among the hits and the total counts only {only}'))
def not_among_total(world: World, label: str, only: str) -> None:
    resp = s.ctx(world).resps[0]
    ids = s.hit_ids(resp)
    assert s.listing(world, label).id not in ids, resp.text
    assert set(ids) == set(ids_of(world, only)), resp.text
    assert s.total_of(resp) == len(ids_of(world, only)), resp.text


@then("both calls fail with invalid_argument")
def both_invalid(world: World) -> None:
    for resp in s.ctx(world).resps:
        assert resp.status_code == 400 and resp.json().get("code") == "invalid_argument", (
            resp.status_code,
            resp.text,
        )


@then("a call with min_rating 5 and a call with min_rating 0 are accepted")
def boundaries_accepted(world: World) -> None:
    for value in (5, 0):
        resp = search_all(world, min_rating=value)[0]
        assert resp.status_code == 200, (value, resp.status_code, resp.text)


@then(
    parsers.parse(
        "the ratings facet is 4 with count {a:d} and 3 with count {b:d}"
    )
)
def ratings_facet(world: World, a: int, b: int) -> None:
    resp = s.ctx(world).resps[0]
    assert facet_pairs(resp, "ratings") == [("4", a), ("3", b)], resp.text


@then(parsers.parse("the categories facet counts {n:d} listing"))
def categories_facet(world: World, n: int) -> None:
    resp = s.ctx(world).resps[0]
    assert sum(c for _, c in facet_pairs(resp, "categories")) == n, resp.text


@then(
    parsers.parse(
        'the hit of "{label}" has sold_count {sold:d}, rating_avg {avg:g} and review_count {n:d}'
    )
)
def hit_has_all(world: World, label: str, sold: int, avg: float, n: int) -> None:
    hit = s.hit_of(s.ctx(world).resps[0], s.listing(world, label).id)
    assert hit is not None, s.ctx(world).resps[0].text
    assert (num(hit, "soldCount"), num(hit, "ratingAvg"), num(hit, "reviewCount")) == (
        sold,
        avg,
        n,
    ), hit


@then(
    'the hit of "L" has sold_count 0, rating_avg 0 and review_count 0 while the hit of "M" has '
    "sold_count 1"
)
def zeros_next_to_values(world: World) -> None:
    resp = s.ctx(world).resps[0]
    first = s.hit_of(resp, s.listing(world, "L").id)
    other = s.hit_of(resp, s.listing(world, "M").id)
    assert first is not None and other is not None, resp.text
    assert (num(first, "soldCount"), num(first, "ratingAvg"), num(first, "reviewCount")) == (
        0,
        0,
        0,
    ), first
    assert num(other, "soldCount") == 1, other
