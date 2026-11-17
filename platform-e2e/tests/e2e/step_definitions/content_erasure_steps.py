"""Steps for the engagement, chat and notification `user_erasure.feature` files (change content-user-erasure).

A `UserErased` envelope is produced to the real `identity.events` topic; every observation is through
public gateway RPCs or the broker (DLQ topics, consumer-group lag). The only direct database reads are
engagement_db fields that no RPC exposes (hide_reason, hidden_by, hidden_at, reports.reporter_id,
review_helpful_votes, follows.seller_id); seeding a moderator id into hidden_by is the one write, because
the shared seed admin must not be erased.
"""

from __future__ import annotations

import os
import time
import uuid
from collections.abc import Callable
from typing import Any

from pytest_bdd import given, parsers, then, when

from tests.e2e.support import erasure_events_support as ev
from tests.e2e.support import oic_order_support as s
from tests.e2e.support import order_lifecycle_support as ol
from tests.e2e.support import order_work_support as nw
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import review_order_support as ros

ENG = "/platform.engagement.v1.EngagementService"
CHAT = "/platform.chat.v1.ChatService"
NOTIF = "/platform.notification.v1.NotificationService"
MASK_NAME = "Người dùng đã xóa"
MASK_TEXT = "Tin nhắn đã bị xóa"
SETTLE_S = 8.0
GROUPS = {
    "engagement": ev.ENGAGEMENT_GROUP,
    "chat": ev.CHAT_GROUP,
    "notification": ev.NOTIFICATION_GROUP,
}
DLQS = {
    "engagement": ev.ENGAGEMENT_DLQ,
    "chat": ev.CHAT_DLQ,
    "notification": ev.NOTIFICATION_DLQ,
}


def _d(oic) -> dict:
    return oic.data.setdefault("cerase", {})


def _sub(oic, key: str) -> dict:
    return _d(oic).setdefault(key, {})


def _a(oic, name: str) -> s.Actor:
    return oic.actors[name]


def _uid(oic, name: str) -> str:
    return _a(oic, name).user_id


def _lid(oic, listing: str) -> str:
    return oic.listings[listing]["id"]


def _call(oic, actor, service: str, method: str, body: dict | None = None) -> dict:
    return s.ok(s.post(oic, actor, service, method, body or {}))


def _buyer(oic, name: str) -> s.Actor:
    return oic.actors.get(name) or s.register(oic, name, "buyer")


def _psql(sql: str) -> str:
    return ros.engagement_psql(sql)


def _lit(v: str) -> str:
    return ros.sql_lit(v)


def _intact(oic, name: str, probe: Callable[[], bool]) -> None:
    _sub(oic, "intact")[name] = probe


# ── shared actors ────────────────────────────────────────────────────────
@given(parsers.parse('a seller "{name}" with a listing "{listing}"'))
def given_seller_listing(oic, name: str, listing: str) -> None:
    seller = oic.actors.get(name) or s.register(oic, name, "seller")
    s.create_listing(oic, listing, seller, 200, price=100_000)


@given(parsers.parse('a seller "{name}" with listings "{l1}" and "{l2}"'))
def given_seller_two(oic, name: str, l1: str, l2: str) -> None:
    seller = oic.actors.get(name) or s.register(oic, name, "seller")
    s.create_listing(oic, l1, seller, 200, price=100_000)
    s.create_listing(oic, l2, seller, 200, price=100_000)


@given(parsers.parse('a seller "{name}" with 5 listings "V1" to "V5"'))
def given_seller_five(oic, name: str) -> None:
    seller = oic.actors.get(name) or s.register(oic, name, "seller")
    for i in range(1, 6):
        s.create_listing(oic, f"V{i}", seller, 200, price=100_000)


@given(parsers.parse('a seller listing "{listing}" owned by "{name}"'))
def given_listing_owned(oic, listing: str, name: str) -> None:
    s.create_listing(oic, listing, _a(oic, name), 200, price=100_000)


@given(parsers.re(r'a buyer "(?P<name>[^"]+)"'))
def given_buyer(oic, name: str) -> None:
    _buyer(oic, name)


# ── UserErased events ────────────────────────────────────────────────────
@when(parsers.parse('a UserErased event for "{name}" is consumed'))
def when_event(oic, name: str) -> None:
    _sub(oic, "events")[name] = ev.publish_user_erased(_uid(oic, name))


@when(parsers.parse('a UserErased event for the seller "{name}" is consumed'))
def when_event_seller(oic, name: str) -> None:
    _sub(oic, "events")[name] = ev.publish_user_erased(_uid(oic, name), roles=("seller",))


@when(parsers.parse('a UserErased event for the moderator "{name}" is consumed'))
def when_event_moderator(oic, name: str) -> None:
    _sub(oic, "events")[name] = ev.publish_user_erased(
        _sub(oic, "moderator")[name], roles=("admin",)
    )


@when(
    parsers.parse(
        'a UserErased record for "{name}" is produced to a topic other than identity.events'
    )
)
def when_wrong_topic(oic, name: str) -> None:
    topic = f"e2e.content-erasure.{uuid.uuid4().hex[:8]}"
    stack.produce(
        topic,
        key=_uid(oic, name).encode(),
        value=ev.encode_user_erased(_uid(oic, name), ev.new_event_id()),
    )
    _d(oic)["wrong_topic"] = topic


@when(
    parsers.parse(
        'the same UserErased event for "{name}" and then one with a new event id are consumed'
    )
)
def when_redelivery(oic, name: str) -> None:
    d = _d(oic)
    uid = _uid(oic, name)
    _sub(oic, "settled")[name] = _sub(oic, "state_fn")[name]()
    ev.publish_user_erased(uid, event_id=d["events"][name])
    d["second_event"] = ev.publish_user_erased(uid)


@then(
    parsers.parse(
        'the {service} erasure consumer completes both without error and "{name}" is unchanged'
    )
)
def then_redelivery_noop(oic, service: str, name: str) -> None:
    d = _d(oic)
    ev.wait_group_caught_up(GROUPS[service])
    settled = d["settled"][name]
    assert _state(oic, service, name) == settled, f"{service} state of {name} changed on redelivery"
    ev.assert_dlq_empty(DLQS[service], d["events"][name].encode())
    ev.assert_dlq_empty(DLQS[service], d["second_event"].encode())


@when("a UserErased event with an empty user_id and a record that does not decode are produced")
def when_malformed(oic) -> None:
    d = _d(oic)
    d["bad_event_id"] = ev.publish_user_erased("")
    d["bad_marker"] = f"e2e-garbage-{uuid.uuid4().hex}"
    ev.publish_garbage(d["bad_marker"])


@then(
    parsers.parse(
        'both records are written to the {service} DLQ and nothing is applied to "{name}"'
    )
)
def then_dlq(oic, service: str, name: str) -> None:
    d = _d(oic)
    topic = DLQS[service]
    assert ev.wait_dlq_record(
        topic, d["bad_event_id"].encode()
    ), f"empty-user_id event not in {topic}"
    assert ev.wait_dlq_record(topic, d["bad_marker"].encode()), f"undecodable record not in {topic}"
    ev.wait_group_caught_up(GROUPS[service])
    assert _sub(oic, "intact")[name](), f"data of {name} changed after a malformed event"


@then(parsers.parse('nothing of "{name}" is erased or changed'))
def then_nothing_changed(oic, name: str) -> None:
    time.sleep(SETTLE_S)
    assert _sub(oic, "intact")[name](), f"data of {name} changed"


# ── state snapshots used by the replay scenario ──────────────────────────
def _state(oic, service: str, name: str) -> Any:
    return _sub(oic, "state_fn")[name]()


def _remember_settled(oic, name: str, fn: Callable[[], Any]) -> None:
    _sub(oic, "state_fn")[name] = fn
    _sub(oic, "settled")[name] = fn()


# ═════════════════════════ engagement ════════════════════════════════════
def _reviews(oic, listing: str) -> list[dict]:
    return ros.list_reviews(_lid(oic, listing)).get("reviews", [])


def _review_row(oic, name: str, listing: str) -> dict:
    rid = _sub(oic, "review")[name]
    for r in _reviews(oic, listing):
        if r.get("id") == rid:
            return r
    return {}


@given(
    parsers.parse(
        'a buyer "{name}" with a verified {rating:d}-star review "{comment}" on "{listing}"'
    )
)
def given_review(oic, name: str, rating: int, comment: str, listing: str) -> None:
    buyer = _buyer(oic, name)
    seller = oic.listings[listing]["seller"]
    r = ros.seed_verified_review(oic, buyer, seller, _lid(oic, listing), rating, comment)
    _sub(oic, "review")[name] = r["id"]
    _sub(oic, "review_listing")[name] = listing
    uid = buyer.user_id

    def intact() -> bool:
        row = _review_row(oic, name, listing)
        return bool(row) and row.get("userId") == uid and row.get("userName") != MASK_NAME

    _intact(oic, name, intact)
    _remember_settled(oic, name, lambda: (_review_row(oic, name, listing), _favs(oic, name)))


def _favs(oic, name: str) -> list[str]:
    return _call(oic, _a(oic, name), ENG, "ListFavorites", {"page": {"pageSize": 100}}).get(
        "listingIds", []
    )


@given(parsers.parse('"{name}" favourited "{listing}"'))
def given_favourite(oic, name: str, listing: str) -> None:
    _call(oic, _a(oic, name), ENG, "AddFavorite", {"listingId": _lid(oic, listing)})
    prior = _sub(oic, "intact").get(name)

    def intact() -> bool:
        return _lid(oic, listing) in _favs(oic, name) and (prior() if prior else True)

    _intact(oic, name, intact)


@then(
    parsers.parse(
        'the review of "{name}" on "{listing}" reads rating {rating:d}, comment "{comment}" and author name "Người dùng đã xóa" without the id of "{who}"'
    )
)
def then_review_anonymised(
    oic, name: str, listing: str, rating: int, comment: str, who: str
) -> None:
    uid = _uid(oic, who)
    row = ev.wait_for(
        lambda: _review_row(oic, name, listing),
        lambda r: bool(r) and r.get("userName") == MASK_NAME,
        f"review of {name} was not anonymised",
    )
    assert row.get("rating") == rating, row
    assert row.get("comment") == comment, row
    assert row.get("userId") != uid, f"review still carries the erased id: {row}"


# Q&A
@given(parsers.parse('a buyer "{name}" who asked "{text}" on "{listing}"'))
def given_question(oic, name: str, text: str, listing: str) -> None:
    buyer = _buyer(oic, name)
    q = _call(
        oic, buyer, ENG, "AskQuestion", {"listingId": _lid(oic, listing), "questionText": text}
    )
    _sub(oic, "question")[text] = q["question"]["id"]
    _d(oic)["qa_listing"] = listing


@given(
    parsers.parse(
        'a buyer "{asker}" asked "{text}" on "{listing}" and "{answerer}" answered it with "{answer}"'
    )
)
def given_question_answered(
    oic, asker: str, text: str, listing: str, answerer: str, answer: str
) -> None:
    buyer = _buyer(oic, asker)
    q = _call(
        oic, buyer, ENG, "AskQuestion", {"listingId": _lid(oic, listing), "questionText": text}
    )
    qid = q["question"]["id"]
    _call(
        oic,
        _a(oic, answerer),
        ENG,
        "AnswerQuestion",
        {"questionId": qid, "answerText": answer, "isShopReply": False},
    )
    _sub(oic, "question")[text] = qid
    _d(oic)["qa_listing"] = listing


def _questions(oic, listing: str) -> list[dict]:
    return _call(
        oic,
        None,
        ENG,
        "ListQuestionsByListing",
        {"listingId": _lid(oic, listing), "page": {"pageSize": 100}},
    ).get("questions", [])


def _qa_rows(oic, listing: str) -> list[dict]:
    rows: list[dict] = []
    for q in _questions(oic, listing):
        rows.append(q)
        rows.extend(q.get("answers", []))
    return rows


@then(
    parsers.parse(
        'the questions of "{listing}" still list "{q1}", "{q2}" and "{a1}" and no row carries the id of "{name}"'
    )
)
def then_qa(oic, listing: str, q1: str, q2: str, a1: str, name: str) -> None:
    uid = _uid(oic, name)

    def clean() -> bool:
        return all(r.get("userId") != uid for r in _qa_rows(oic, listing))

    ev.wait_for(lambda: clean(), bool, f"a Q&A row still carries the id of {name}")
    texts = {r.get("questionText") or r.get("answerText") for r in _qa_rows(oic, listing)}
    for t in (q1, q2, a1):
        assert t in texts, f"{t!r} no longer listed: {texts}"


# rating summary
@given(parsers.parse('the rating summary of "{listing}" is recorded'))
def given_summary(oic, listing: str) -> None:
    _d(oic)["summary"] = ros.listing_summary(_lid(oic, listing))
    assert int(_d(oic)["summary"].get("reviewCount", 0)) == 3, _d(oic)["summary"]


@then(
    parsers.parse('the review count of "{listing}" is still 3 and its average rating is unchanged')
)
def then_summary(oic, listing: str) -> None:
    now = ros.listing_summary(_lid(oic, listing))
    before = _d(oic)["summary"]
    assert int(now.get("reviewCount", 0)) == 3, now
    assert now.get("averageRating") == before.get("averageRating"), (before, now)


# moderation
@given(parsers.parse('the admin hides the review of "{name}" with the reason "{reason}"'))
def given_hide(oic, name: str, reason: str) -> None:
    admin = oic.actors.get("admin") or s.login_admin(oic)
    _call(oic, admin, ENG, "HideReview", {"reviewId": _sub(oic, "review")[name], "reason": reason})


def _mod_row(oic, name: str) -> list[str]:
    rid = _sub(oic, "review")[name]
    out = _psql(
        "SELECT hide_reason, hidden_by, (hidden_at IS NOT NULL) FROM reviews "
        f"WHERE id = {_lit(rid)}"
    )
    return out.split("|") if out else []


@then(
    parsers.parse(
        'the hide reason of the review of "{name}" is empty, it is still hidden and hidden_at is still set'
    )
)
def then_hide_reason_cleared(oic, name: str) -> None:
    row = ev.wait_for(
        lambda: _mod_row(oic, name), lambda r: bool(r) and r[0] == "", "hide_reason was not cleared"
    )
    assert row[2] == "t", f"hidden_at was cleared: {row}"
    listing = _sub(oic, "review_listing")[name]
    assert not _review_row(oic, name, listing), "the hidden review is listed to buyers again"


@given(parsers.parse('the reviews of "{a}" and "{b}" record moderator "{mod}" as hidden_by'))
def given_hidden_by(oic, a: str, b: str, mod: str) -> None:
    mid = f"e2e-mod-{uuid.uuid4().hex[:10]}"
    _sub(oic, "moderator")[mod] = mid
    for n in (a, b):
        _psql(
            f"UPDATE reviews SET hidden_by = {_lit(mid)} WHERE id = {_lit(_sub(oic, 'review')[n])}"
        )
        assert _mod_row(oic, n)[1] == mid


@then(
    parsers.parse(
        'hidden_by is empty on the reviews of "{a}" and "{b}", their hide reason reads "{reason}" and both are still hidden'
    )
)
def then_hidden_by_cleared(oic, a: str, b: str, reason: str) -> None:
    for n in (a, b):
        row = ev.wait_for(
            lambda n=n: _mod_row(oic, n),
            lambda r: bool(r) and r[1] == "",
            f"hidden_by of {n} not cleared",
        )
        assert row[0] == reason, f"hide_reason of {n} changed: {row}"
        assert row[2] == "t", f"review of {n} is no longer hidden: {row}"


# isolation
@given(parsers.parse('"{name}" asked "{text}" on "{listing}" and favourited "{l2}"'))
def given_b_content(oic, name: str, text: str, listing: str, l2: str) -> None:
    buyer = _a(oic, name)
    q = _call(
        oic, buyer, ENG, "AskQuestion", {"listingId": _lid(oic, listing), "questionText": text}
    )
    _sub(oic, "question")[f"{name}:{text}"] = q["question"]["id"]
    _call(oic, buyer, ENG, "AddFavorite", {"listingId": _lid(oic, l2)})
    _d(oic)["b_listing"] = listing


def _content(oic, name: str) -> dict:
    listing = _d(oic)["b_listing"]
    mine = [q for q in _questions(oic, listing) if q.get("userId") == _uid(oic, name)]
    return {
        "review": _review_row(oic, name, _sub(oic, "review_listing")[name]),
        "questions": mine,
        "favourites": _favs(oic, name),
    }


@given(parsers.parse('the content of "{name}" is recorded'))
def given_content_recorded(oic, name: str) -> None:
    _sub(oic, "content")[name] = _content(oic, name)
    assert _sub(oic, "content")[name]["review"] and _sub(oic, "content")[name]["questions"]


@then(parsers.parse('the content of "{name}" is unchanged'))
def then_content_unchanged(oic, name: str) -> None:
    assert _content(oic, name) == _sub(oic, "content")[name], f"content of {name} changed"


# wrong-topic
@when(parsers.parse('"{x}" calls engagement RPCs naming "{name}" and the review of "{name2}"'))
def when_x_calls_engagement(oic, x: str, name: str, name2: str) -> None:
    actor = _a(oic, x)
    s.post(oic, actor, ENG, "UnfollowSeller", {"sellerId": _uid(oic, name)})
    s.post(oic, actor, ENG, "HideReview", {"reviewId": _sub(oic, "review")[name2], "reason": "x"})


# deletes
@given(
    parsers.parse(
        'a buyer "{name}" with 2 favourites, 1 follow of "{seller}", 1 report filed on "{listing}" and 1 helpful vote on the review of "{author}"'
    )
)
def given_activity(oic, name: str, seller: str, listing: str, author: str) -> None:
    buyer = _buyer(oic, name)
    for lk in ("L", "L2"):
        _call(oic, buyer, ENG, "AddFavorite", {"listingId": _lid(oic, lk)})
    _call(oic, buyer, ENG, "FollowSeller", {"sellerId": _uid(oic, seller)})
    _call(
        oic,
        buyer,
        ENG,
        "CreateReport",
        {
            "targetType": "REPORT_TARGET_TYPE_LISTING",
            "targetId": _lid(oic, listing),
            "reasonCode": "other",
        },
    )
    _call(oic, buyer, ENG, "MarkReviewHelpful", {"reviewId": _sub(oic, "review")[author]})
    assert _activity_counts(oic, name) == (2, 1, 1, 1), _activity_counts(oic, name)


def _activity_counts(oic, name: str) -> tuple[int, int, int, int]:
    uid = _uid(oic, name)
    favs = len(_favs(oic, name))
    follows = int(_psql(f"SELECT count(*) FROM follows WHERE user_id = {_lit(uid)}") or 0)
    reports = int(_psql(f"SELECT count(*) FROM reports WHERE reporter_id = {_lit(uid)}") or 0)
    votes = int(
        _psql(f"SELECT count(*) FROM review_helpful_votes WHERE user_id = {_lit(uid)}") or 0
    )
    return favs, follows, reports, votes


@then(
    parsers.parse('zero favourites, follows, filed reports and helpful votes remain for "{name}"')
)
def then_activity_deleted(oic, name: str) -> None:
    ev.wait_for(
        lambda: _activity_counts(oic, name),
        lambda c: c == (0, 0, 0, 0),
        f"activity of {name} not deleted",
    )


@given(parsers.parse('buyers "{a}", "{b}" and "{c}" who follow "{s1}" and "{s2}"'))
def given_followers(oic, a: str, b: str, c: str, s1: str, s2: str) -> None:
    for n in (a, b, c):
        buyer = _buyer(oic, n)
        for sel in (s1, s2):
            _call(oic, buyer, ENG, "FollowSeller", {"sellerId": _uid(oic, sel)})
    _d(oic)["followers"] = (a, b, c)
    _d(oic)["follow_other"] = s2


def _followed(oic, n: str) -> list[str]:
    return _call(oic, _a(oic, n), ENG, "ListFollowedSellers", {"page": {"pageSize": 100}}).get(
        "sellerIds", []
    )


@then(
    parsers.parse(
        'zero follows with seller "{seller}" remain and each of "{a}", "{b}" and "{c}" still follows only "{other}"'
    )
)
def then_followers_gone(oic, seller: str, a: str, b: str, c: str, other: str) -> None:
    sid = _uid(oic, seller)
    ev.wait_for(
        lambda: int(_psql(f"SELECT count(*) FROM follows WHERE seller_id = {_lit(sid)}") or 0),
        lambda n: n == 0,
        f"followers of {seller} not deleted",
    )
    for n in (a, b, c):
        assert _followed(oic, n) == [_uid(oic, other)], f"follows of {n}: {_followed(oic, n)}"


@given(
    parsers.parse('a buyer "{name}" with a collection of 2 items, 5 viewed listings and a check-in')
)
def given_personal(oic, name: str) -> None:
    buyer = _buyer(oic, name)
    col = _call(oic, buyer, ENG, "CreateCollection", {"name": "Yêu thích"})["collection"]["id"]
    for lk in ("V1", "V2"):
        _call(oic, buyer, ENG, "AddToCollection", {"collectionId": col, "listingId": _lid(oic, lk)})
    for i in range(1, 6):
        _call(oic, buyer, ENG, "RecordView", {"listingId": _lid(oic, f"V{i}")})
    _call(oic, buyer, ENG, "CheckIn", {})
    assert _personal(oic, name) == (1, 5, True), _personal(oic, name)


def _personal(oic, name: str) -> tuple[int, int, bool]:
    b = _a(oic, name)
    cols = len(_call(oic, b, ENG, "ListCollections", {}).get("collections", []))
    views = len(
        _call(oic, b, ENG, "GetRecentlyViewed", {"page": {"pageSize": 100}}).get("listingIds", [])
    )
    loy = _call(oic, b, ENG, "GetLoyalty", {})
    return (
        cols,
        views,
        int(loy.get("coinBalance", 0) or 0) > 0 or int(loy.get("streak", 0) or 0) > 0,
    )


@then(parsers.parse('"{name}" has no collections, no viewed listings and an empty loyalty account'))
def then_personal_gone(oic, name: str) -> None:
    ev.wait_for(
        lambda: _personal(oic, name),
        lambda c: c == (0, 0, False),
        f"personal data of {name} not deleted",
    )


# reports about content
@given(parsers.parse('a buyer "{name}" who reported "{listing}"'))
def given_reporter(oic, name: str, listing: str) -> None:
    buyer = _buyer(oic, name)
    _call(
        oic,
        buyer,
        ENG,
        "CreateReport",
        {
            "targetType": "REPORT_TARGET_TYPE_LISTING",
            "targetId": _lid(oic, listing),
            "reasonCode": "other",
        },
    )


@then(parsers.parse('the report of "{name}" on "{listing}" still exists with status "{status}"'))
def then_report_open(oic, name: str, listing: str, status: str) -> None:
    admin = oic.actors.get("admin") or s.login_admin(oic)
    reports = _call(
        oic, admin, ENG, "ListReports", {"targetId": _lid(oic, listing), "page": {"pageSize": 100}}
    ).get("reports", [])
    mine = [r for r in reports if r.get("reporterId") == _uid(oic, name)]
    assert len(mine) == 1, f"report of {name} gone or duplicated: {reports}"
    assert mine[0].get("status") == status, mine[0]


# one hour
@given(parsers.parse('a buyer "{name}" holding 1000 reviews on "{listing}"'))
def given_thousand(oic, name: str, listing: str) -> None:
    buyer = _buyer(oic, name)
    seller = oic.listings[listing]["seller"]
    for chunk in range(10):
        ros.seed_verified_reviews_sql(
            _lid(oic, listing),
            seller.user_id,
            100,
            user_id=buyer.user_id,
            comment_prefix=f"cue{chunk}",
        )
    assert _count_rows(oic, name) == 1000


def _count_rows(oic, name: str) -> int:
    return int(_psql(f"SELECT count(*) FROM reviews WHERE user_id = {_lit(_uid(oic, name))}") or 0)


@then(parsers.parse('all 1000 rows of "{name}" are anonymised within 1 hour'))
def then_thousand(oic, name: str) -> None:
    bound = float(os.getenv("ERASURE_BOUND_S", "3600"))
    start = time.monotonic()
    ev.wait_for(
        lambda: _count_rows(oic, name),
        lambda n: n == 0,
        "rows still keyed by the erased user",
        timeout=bound,
    )
    assert time.monotonic() - start < bound


# ═════════════════════════ chat ══════════════════════════════════════════
def _thread(oic, buyer: s.Actor, seller: s.Actor, listing: str) -> str:
    t = _call(
        oic,
        buyer,
        CHAT,
        "GetOrCreateThread",
        {"sellerId": seller.user_id, "listingId": _lid(oic, listing)},
    )
    return t["thread"]["id"]


def _send(oic, actor: s.Actor, tid: str, text: str, **extra) -> dict:
    return _call(oic, actor, CHAT, "SendMessage", {"threadId": tid, "content": text, **extra})[
        "message"
    ]


def _messages(oic, actor: s.Actor, tid: str) -> list[dict]:
    return _call(
        oic, actor, CHAT, "GetThreadMessages", {"threadId": tid, "page": {"pageSize": 100}}
    ).get("messages", [])


def _threads(oic, actor: s.Actor) -> list[dict]:
    return _call(oic, actor, CHAT, "ListThreads", {"page": {"pageSize": 100}}).get("threads", [])


def _chat_probe(oic, name: str, seller: str, tid: str, texts: list[str]) -> None:
    def intact() -> bool:
        got = [
            m.get("content")
            for m in _messages(oic, _a(oic, seller), tid)
            if m.get("senderId") == _uid(oic, name)
        ]
        return got == texts

    _intact(oic, name, intact)
    _remember_settled(oic, name, lambda: _messages(oic, _a(oic, seller), tid))


@given(
    parsers.parse(
        'a buyer "{name}" who sent 3 messages in a thread with "{seller}" on "{listing}" where "{s2}" sent 2'
    )
)
def given_thread(oic, name: str, seller: str, listing: str, s2: str) -> None:
    buyer = _buyer(oic, name)
    sel = _a(oic, seller)
    tid = _thread(oic, buyer, sel, listing)
    mine: list[str] = []
    theirs: list[str] = []
    for i in range(1, 4):
        t = f"{name}-tin-{i}-{uuid.uuid4().hex[:4]}"
        _send(oic, buyer, tid, t)
        mine.append(t)
        if i <= 2:
            t2 = f"{seller}-tra-loi-{i}-{uuid.uuid4().hex[:4]}"
            _send(oic, sel, tid, t2)
            theirs.append(t2)
    ch = _sub(oic, "chat")
    ch[name] = {"tid": tid, "mine": mine, "theirs": theirs, "seller": seller}
    _chat_probe(oic, name, seller, tid, mine)


@then(
    parsers.parse(
        'the 3 messages of "{name}" read "Tin nhắn đã bị xóa" with sender name "Người dùng đã xóa", the 2 messages of "{seller}" are unchanged and the thread still lists for "{s2}"'
    )
)
def then_messages_replaced(oic, name: str, seller: str, s2: str) -> None:
    info = _sub(oic, "chat")[name]
    uid = _uid(oic, name)
    sel = _a(oic, seller)

    def mine() -> list[dict]:
        return [m for m in _messages(oic, sel, info["tid"]) if m.get("senderId") == uid]

    got = ev.wait_for(
        mine,
        lambda ms: len(ms) == 3 and all(m.get("content") == MASK_TEXT for m in ms),
        "messages of the erased user were not replaced",
    )
    assert all(m.get("senderName") == MASK_NAME for m in got), got
    theirs = [
        m.get("content")
        for m in _messages(oic, sel, info["tid"])
        if m.get("senderId") == sel.user_id
    ]
    assert theirs == info["theirs"], f"counterparty messages changed: {theirs}"
    assert any(
        t.get("id") == info["tid"] for t in _threads(oic, sel)
    ), "thread no longer lists for the seller"


@given(
    parsers.parse(
        'a buyer "{name}" who sent an image message with a payload in a thread with "{seller}" on "{listing}"'
    )
)
def given_rich(oic, name: str, seller: str, listing: str) -> None:
    buyer = _buyer(oic, name)
    sel = _a(oic, seller)
    tid = _thread(oic, buyer, sel, listing)
    m = _send(
        oic,
        buyer,
        tid,
        "xem cái này",
        messageType="MESSAGE_TYPE_LISTING_CARD",
        listingId=_lid(oic, listing),
        payload='{"image":"https://x/y.png"}',
    )
    assert m.get("payload"), m
    _sub(oic, "chat")[name] = {"tid": tid, "msg": m["id"], "seller": seller}


@then(
    parsers.parse(
        'the image message of "{name}" has an empty payload and reads "Tin nhắn đã bị xóa"'
    )
)
def then_rich(oic, name: str) -> None:
    info = _sub(oic, "chat")[name]
    sel = _a(oic, info["seller"])

    def row() -> dict:
        return next((m for m in _messages(oic, sel, info["tid"]) if m.get("id") == info["msg"]), {})

    got = ev.wait_for(row, lambda m: m.get("content") == MASK_TEXT, "message text was not replaced")
    assert not got.get("payload"), got


@given(
    parsers.parse(
        'a buyer "{name}" who sent the last message of a thread with "{seller}" on "{listing}"'
    )
)
def given_last_by_user(oic, name: str, seller: str, listing: str) -> None:
    buyer = _buyer(oic, name)
    sel = _a(oic, seller)
    tid = _thread(oic, buyer, sel, listing)
    _send(oic, sel, tid, "chào bạn")
    _send(oic, buyer, tid, f"{name}-cuoi-{uuid.uuid4().hex[:4]}")
    _sub(oic, "preview")[listing] = tid


@given(
    parsers.parse(
        'a buyer "{name}" whose thread with "{seller}" on "{listing}" ends with a message from "{s2}"'
    )
)
def given_last_by_seller(oic, name: str, seller: str, listing: str, s2: str) -> None:
    buyer = _buyer(oic, name)
    sel = _a(oic, seller)
    tid = _thread(oic, buyer, sel, listing)
    _send(oic, buyer, tid, f"{name}-dau-{uuid.uuid4().hex[:4]}")
    _send(oic, sel, tid, f"{seller}-cuoi-{uuid.uuid4().hex[:4]}")
    _sub(oic, "preview")[listing] = tid
    before = next(t for t in _threads(oic, sel) if t["id"] == tid)
    _sub(oic, "preview_before")[listing] = before.get("lastMessageText")
    _intact(oic, name, lambda: True)


@then(
    parsers.parse(
        'the preview of the thread on "{l1}" reads "Tin nhắn đã bị xóa" and the preview of the thread on "{l2}" is unchanged'
    )
)
def then_preview(oic, l1: str, l2: str) -> None:
    sel = _a(oic, "S")
    t1, t2 = _sub(oic, "preview")[l1], _sub(oic, "preview")[l2]

    def text(tid: str) -> str | None:
        return next((t.get("lastMessageText") for t in _threads(oic, sel) if t["id"] == tid), None)

    ev.wait_for(
        lambda: text(t1), lambda t: t == MASK_TEXT, "thread preview still shows the erased text"
    )
    assert (
        text(t2) == _sub(oic, "preview_before")[l2]
    ), "preview of the counterparty's last message changed"


@given(parsers.parse('the thread of "{name}" is recorded'))
def given_thread_recorded(oic, name: str) -> None:
    info = _sub(oic, "chat")[name]
    sel = _a(oic, info["seller"])
    info["before_msgs"] = _messages(oic, sel, info["tid"])
    info["before_thread"] = next(t for t in _threads(oic, sel) if t["id"] == info["tid"])


@then(parsers.parse('the messages and thread of "{name}" are unchanged'))
def then_thread_unchanged(oic, name: str) -> None:
    info = _sub(oic, "chat")[name]
    sel = _a(oic, info["seller"])
    assert (
        _messages(oic, sel, info["tid"]) == info["before_msgs"]
    ), "messages of the other user changed"
    now = next(t for t in _threads(oic, sel) if t["id"] == info["tid"])
    assert now == info["before_thread"], "thread of the other user changed"


@when(parsers.parse('"{x}" calls chat RPCs naming "{name}" and the thread of "{name2}"'))
def when_x_calls_chat(oic, x: str, name: str, name2: str) -> None:
    actor = _a(oic, x)
    tid = _sub(oic, "chat")[name2]["tid"]
    s.post(oic, actor, CHAT, "SendMessage", {"threadId": tid, "content": "xen vao"})
    s.post(oic, actor, CHAT, "GetThreadMessages", {"threadId": tid})
    s.post(oic, actor, CHAT, "GetOrCreateThread", {"sellerId": _uid(oic, name)})


# ═════════════════════════ notification ═════════════════════════════════
def _feed(oic, name: str) -> list[dict]:
    return nw.feed(oic, _a(oic, name))


def _subs(oic, name: str) -> list[dict]:
    return _call(oic, _a(oic, name), NOTIF, "ListAlertSubscriptions", {}).get("subscriptions", [])


def _prefs(oic, name: str) -> dict:
    return _call(oic, _a(oic, name), NOTIF, "GetNotificationPrefs", {}).get("prefs", {}) or {}


def _is_default_prefs(p: dict) -> bool:
    return (
        p.get("digestFreq", "DIGEST_FREQUENCY_OFF") == "DIGEST_FREQUENCY_OFF"
        and p.get("typeEnabled", {}).get("NOTIFICATION_TYPE_ORDER") is not False
    )


def _notif_snapshot(oic, name: str) -> dict:
    return {
        "feed": sorted(n.get("id") for n in _feed(oic, name)),
        "subs": sorted(x.get("id") for x in _subs(oic, name)),
        "prefs": _prefs(oic, name),
    }


@given(
    parsers.parse(
        'a buyer "{name}" with notifications, 2 alert subscriptions on "{listing}" and a preferences row'
    )
)
def given_notif_user(oic, name: str, listing: str) -> None:
    buyer = _buyer(oic, name)
    ol.place(oic, buyer, listing, 1, "PAYMENT_METHOD_MOCK_MOMO")
    ev.wait_for(
        lambda: _feed(oic, name), bool, f"{name} got no notification from the order", timeout=45.0
    )
    for kind in ("ALERT_TYPE_PRICE_DROP", "ALERT_TYPE_BACK_IN_STOCK"):
        _call(oic, buyer, NOTIF, "SubscribeAlert", {"listingId": _lid(oic, listing), "type": kind})
    _call(
        oic,
        buyer,
        NOTIF,
        "UpdateNotificationPrefs",
        {
            "prefs": {
                "typeEnabled": {"NOTIFICATION_TYPE_ORDER": False},
                "digestFreq": "DIGEST_FREQUENCY_DAILY",
            }
        },
    )
    assert len(_subs(oic, name)) == 2
    assert not _is_default_prefs(_prefs(oic, name)), _prefs(oic, name)
    _sub(oic, "notif")[name] = _notif_snapshot(oic, name)

    def intact() -> bool:
        return (
            len(_feed(oic, name)) >= 1
            and len(_subs(oic, name)) == 2
            and not _is_default_prefs(_prefs(oic, name))
        )

    _intact(oic, name, intact)
    _sub(oic, "state_fn")[name] = lambda: _notif_snapshot(oic, name)
    _sub(oic, "settled")[name] = None


@then(
    parsers.parse('"{name}" has no notifications, no alert subscriptions and default preferences')
)
def then_notif_gone(oic, name: str) -> None:
    ev.wait_for(
        lambda: (
            len(_feed(oic, name)),
            len(_subs(oic, name)),
            _is_default_prefs(_prefs(oic, name)),
        ),
        lambda c: c == (0, 0, True),
        f"notification data of {name} not deleted",
    )
    _sub(oic, "settled")[name] = _notif_snapshot(oic, name)


@given(parsers.parse('the notification data of "{name}" is recorded'))
def given_notif_recorded(oic, name: str) -> None:
    _sub(oic, "notif")[name] = _notif_snapshot(oic, name)


@then(parsers.parse('the notification data of "{name}" is unchanged'))
def then_notif_unchanged(oic, name: str) -> None:
    assert (
        _notif_snapshot(oic, name) == _sub(oic, "notif")[name]
    ), f"notification data of {name} changed"


@when(
    parsers.parse('"{x}" calls notification RPCs naming "{name}" and a notification of "{name2}"')
)
def when_x_calls_notification(oic, x: str, name: str, name2: str) -> None:
    actor = _a(oic, x)
    nid = _feed(oic, name2)[0]["id"]
    s.post(oic, actor, NOTIF, "MarkAsRead", {"id": nid})
    s.post(oic, actor, NOTIF, "UnsubscribeAlert", {"subscriptionId": _subs(oic, name2)[0]["id"]})
