"""ListMyListings filter (domain-my-listings-filter) through the gateway. Listings are seeded
through the real API (rejected ones via the admin moderation path); nothing is inserted by SQL."""

from __future__ import annotations

import re
import subprocess
import time
from concurrent.futures import ThreadPoolExecutor

import httpx
import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.support.oic_order_support import (
    LISTING,
    Actor,
    OicWorld,
    code_of,
    login_admin,
    ok,
    post,
    register,
)

FLAGGED = "Replica screen-review-marker"
ACTORS = {"seller": "seller", "buyer": "buyer", "other seller": "other"}


@pytest.fixture
def mlf():
    w = OicWorld()
    w.data.update(ids={"published": [], "draft": [], "rejected": []}, titles={}, calls=[])
    yield w


def _post_retry(w: OicWorld, actor: Actor, method: str, body: dict) -> httpx.Response:
    resp = post(w, actor, LISTING, method, body)
    for attempt in range(6):
        if resp.status_code not in (429, 502, 503, 504):
            break
        time.sleep(0.5 * (attempt + 1))
        resp = post(w, actor, LISTING, method, body)
    return resp


def _create(w: OicWorld, owner: Actor, title: str, status: str) -> str:
    resp = _post_retry(
        w, owner, "CreateListing",
        {"listing": {
            "title": title, "categoryId": "cat-electronics", "price": 100000, "stock": 5,
            "status": status, "currency": "VND", "description": "my listings filter e2e",
        }},
    )  # fmt: skip
    listing_id = (ok(resp).get("listing") or {}).get("id", "")
    assert listing_id, resp.text
    return listing_id


def _seed(w: OicWorld, owner: Actor, kind: str, titles: list[str]) -> list[str]:
    status = "LISTING_STATUS_DRAFT" if kind == "draft" else "LISTING_STATUS_PUBLISHED"
    if kind == "rejected":
        titles = [f"{t} {FLAGGED}" for t in titles]
    with ThreadPoolExecutor(max_workers=8) as pool:
        ids = list(pool.map(lambda t: _create(w, owner, t, status), titles))
    if kind == "rejected":
        admin = w.actors.get("admin") or login_admin(w)
        for lid in ids:
            got = ok(post(w, owner, LISTING, "GetListing", {"id": lid})).get("listing") or {}
            assert got.get("status") == "LISTING_STATUS_PENDING_REVIEW", got
            resp = post(
                w, admin, LISTING, "ModerateListing",
                {"listingId": lid, "decision": "MODERATION_DECISION_REJECT", "reasonCode": "counterfeit"},
            )  # fmt: skip
            ok(resp)
    for lid, t in zip(ids, titles, strict=True):
        w.data["titles"][lid] = t
    return ids


def _owner(w: OicWorld, name: str = "seller") -> Actor:
    return w.actors.get(name) or register(w, name, "seller")


@given(parsers.parse("a seller owns {p:d} published, {d:d} draft and {r:d} rejected listings"))
def seller_mixed(mlf: OicWorld, p: int, d: int, r: int) -> None:
    owner = _owner(mlf)
    for kind, n in (("published", p), ("draft", d), ("rejected", r)):
        mlf.data["ids"][kind] += _seed(mlf, owner, kind, [f"Item {kind} {i}" for i in range(n)])


@given(
    parsers.parse("another seller owns {p:d} published, {d:d} draft and {r:d} rejected listings")
)
def other_mixed(mlf: OicWorld, p: int, d: int, r: int) -> None:
    owner = _owner(mlf, "other")
    for kind, n in (("published", p), ("draft", d), ("rejected", r)):
        _seed(mlf, owner, kind, [f"Foreign {kind} {i}" for i in range(n)])


@given("another seller owns no listing")
def other_none(mlf: OicWorld) -> None:
    _owner(mlf, "other")


@given("a buyer is signed in")
def buyer_in(mlf: OicWorld) -> None:
    register(mlf, "buyer", "buyer")


@given(parsers.parse("a seller owns published listings titled {titles}"))
def seller_titled(mlf: OicWorld, titles: str) -> None:
    mlf.data["ids"]["published"] += _seed(
        mlf, _owner(mlf), "published", re.findall(r'"([^"]+)"', titles)
    )


@given(parsers.parse('the seller owns a published and a draft listing both titled "{title}"'))
def seller_pub_and_draft(mlf: OicWorld, title: str) -> None:
    owner = _owner(mlf)
    mlf.data["ids"]["published"] += _seed(mlf, owner, "published", [title])
    mlf.data["ids"]["draft"] += _seed(mlf, owner, "draft", [title])


@given(
    parsers.parse(
        'a seller owns {n:d} published listings of which every second title contains "{word}"'
    )
)
def seller_bulk_titled(mlf: OicWorld, n: int, word: str) -> None:
    titles = [f"Áo thun {i}" if i % 2 else f"Quần jean {i}" for i in range(n)]
    mlf.data["ids"]["published"] += _seed(mlf, _owner(mlf), "published", titles)


def _statuses(spec: str) -> list[str]:
    out = []
    for s in (x.strip() for x in spec.split(",") if x.strip()):
        out.append(s if s.startswith("LISTING_STATUS_") else f"LISTING_STATUS_{s}")
    return out


def _list(w: OicWorld, actor: Actor, statuses=None, query=None, page_size=None, cursor=None):
    body: dict = {}
    page: dict = {}
    if page_size is not None:
        page["pageSize"] = page_size
    if cursor:
        page["cursor"] = cursor
    if page:
        body["page"] = page
    if statuses:
        body["statuses"] = statuses
    if query is not None:
        body["query"] = query
    w.data.setdefault("t0", int(time.time()) - 1)
    start = time.perf_counter()
    resp = post(w, actor, LISTING, "ListMyListings", body)
    elapsed = (time.perf_counter() - start) * 1000
    w.responses["last"] = resp
    w.data["calls"].append(resp)
    w.data.setdefault("durations", []).append(elapsed)
    return resp


def _actor(w: OicWorld, who: str) -> Actor:
    return w.actors[ACTORS[who]]


@when(parsers.parse('the {who} lists their listings with statuses "{spec}"'))
def list_statuses(mlf: OicWorld, who: str, spec: str) -> None:
    _list(mlf, _actor(mlf, who), statuses=_statuses(spec))


@when("the seller lists their listings without a filter")
def list_plain(mlf: OicWorld) -> None:
    _list(mlf, _actor(mlf, "seller"))


@when(parsers.parse('the seller lists their listings with query "{query}"'))
def list_query(mlf: OicWorld, query: str) -> None:
    _list(mlf, _actor(mlf, "seller"), query=query)


@when(
    parsers.parse(
        'the seller lists their listings filtered by query "{query}" and statuses "{spec}"'
    )
)
def list_query_statuses(mlf: OicWorld, query: str, spec: str) -> None:
    _list(mlf, _actor(mlf, "seller"), statuses=_statuses(spec), query=query)


@when(parsers.parse("the seller lists their listings with a query of {n:d} runes"))
def list_runes(mlf: OicWorld, n: int) -> None:
    _list(mlf, _actor(mlf, "seller"), query="á" * n)


@when(parsers.parse("the seller lists their listings with page size {n:d}"))
def list_page_size(mlf: OicWorld, n: int) -> None:
    _list(mlf, _actor(mlf, "seller"), page_size=n)


@when(
    parsers.parse(
        'the seller pages through listings with statuses "{spec}" at page size {n:d} until the cursor is empty'
    )
)
def page_through(mlf: OicWorld, spec: str, n: int) -> None:
    pages, cursor = [], ""
    for _ in range(50):
        body = ok(
            _list(mlf, _actor(mlf, "seller"), statuses=_statuses(spec), page_size=n, cursor=cursor)
        )
        pages.append(body)
        cursor = (body.get("page") or {}).get("nextCursor", "")
        if not cursor:
            break
    mlf.data["pages"] = pages


@when(
    parsers.parse(
        'the seller lists listings {times:d} times with statuses "{spec}", query "{query}" and page size {n:d}'
    )
)
def list_repeatedly(mlf: OicWorld, times: int, spec: str, query: str, n: int) -> None:
    mlf.data["durations"] = []
    mlf.data["calls"] = []
    for _ in range(times):
        _list(mlf, _actor(mlf, "seller"), statuses=_statuses(spec), query=query, page_size=n)


@when(parsers.parse('the seller obtains a next cursor for statuses "{spec}" at page size {n:d}'))
def obtain_cursor(mlf: OicWorld, spec: str, n: int) -> None:
    body = ok(_list(mlf, _actor(mlf, "seller"), statuses=_statuses(spec), page_size=n))
    mlf.data["cursor"] = (body.get("page") or {}).get("nextCursor", "")
    mlf.data["cursor_id"] = (body.get("listings") or [{}])[-1].get("id", "")
    assert mlf.data["cursor"] and mlf.data["cursor_id"], body


@when(parsers.parse('the seller reuses that cursor with statuses "{spec}"'))
def reuse_cursor(mlf: OicWorld, spec: str) -> None:
    _list(
        mlf,
        _actor(mlf, "seller"),
        statuses=_statuses(spec),
        page_size=100,
        cursor=mlf.data["cursor"],
    )


def _page(w: OicWorld, resp: httpx.Response | None = None) -> tuple[list[dict], int, str]:
    body = ok(resp or w.responses["last"])
    page = body.get("page") or {}
    return body.get("listings") or [], int(page.get("total", 0) or 0), page.get("nextCursor", "")


def _ids(listings: list[dict]) -> list[str]:
    return [item["id"] for item in listings]


@then(parsers.parse("the response holds exactly the {groups} listings and page total is {total:d}"))
def holds_groups(mlf: OicWorld, groups: str, total: int) -> None:
    listings, got_total, _ = _page(mlf)
    want = [i for g in groups.split(" and ") for i in mlf.data["ids"][g.strip()]]
    assert sorted(_ids(listings)) == sorted(want), (groups, listings)
    assert got_total == total, got_total


@then(
    parsers.parse(
        "the response holds all {n:d} own listings ordered by id with page total {total:d}"
    )
)
def holds_all(mlf: OicWorld, n: int, total: int) -> None:
    listings, got_total, _ = _page(mlf)
    want = [i for ids in mlf.data["ids"].values() for i in ids]
    assert len(want) == n
    assert _ids(listings) == sorted(want), listings
    assert got_total == total, got_total


@then(parsers.parse("the response holds the listings titled {titles} and page total is {total:d}"))
def holds_titled(mlf: OicWorld, titles: str, total: int) -> None:
    listings, got_total, _ = _page(mlf)
    assert sorted(i["title"] for i in listings) == sorted(
        re.findall(r'"([^"]+)"', titles)
    ), listings
    assert got_total == total, got_total


@then("the response holds 0 listings, page total is 0 and the next cursor is empty")
def holds_none(mlf: OicWorld) -> None:
    listings, total, cursor = _page(mlf)
    assert listings == [] and total == 0 and cursor == "", (listings, total, cursor)


@then(parsers.parse("the call fails with {code}"))
def call_fails(mlf: OicWorld, code: str) -> None:
    resp = mlf.responses["last"]
    assert resp.status_code != 200, f"expected {code}, call succeeded: {resp.text[:300]}"
    assert code_of(resp).upper() == code, f"expected {code}, got {code_of(resp)}: {resp.text[:300]}"


@then("no listing is returned")
def none_returned(mlf: OicWorld) -> None:
    resp = mlf.responses["last"]
    assert resp.status_code != 200 and not (resp.json().get("listings") or []), resp.text[:300]


@then("the first call succeeds with 0 listings and the second fails with INVALID_ARGUMENT")
def first_ok_second_bad(mlf: OicWorld) -> None:
    first, second = mlf.data["calls"][-2:]
    listings, _, _ = _page(mlf, first)
    assert listings == [], first.text[:300]
    assert second.status_code != 200 and code_of(second).upper() == "INVALID_ARGUMENT", second.text[
        :300
    ]


@then(parsers.parse("the last two responses are identical and hold {n:d} listings"))
def last_two_identical(mlf: OicWorld, n: int) -> None:
    a, b = (_page(mlf, r) for r in mlf.data["calls"][-2:])
    assert a == b, (a, b)
    assert len(a[0]) == n and a[1] == n, a


@then(parsers.parse('no team-domain log line since the call contains "{needle}"'))
def not_logged(mlf: OicWorld, needle: str) -> None:
    name = subprocess.run(
        ["docker", "ps", "--filter", "name=team-domain-svc", "--format", "{{.Names}}"],
        capture_output=True, text=True, timeout=30, check=True,
    ).stdout.split()  # fmt: skip
    assert name, "team-domain-svc container not running"
    out = subprocess.run(
        ["docker", "logs", "--since", str(mlf.data["t0"]), name[0]],
        capture_output=True, text=True, timeout=60, check=True,
    )  # fmt: skip
    text = out.stdout + out.stderr
    assert text.strip(), "no team-domain log output at all since the call (cannot prove absence)"
    assert needle not in text, f"query text leaked into team-domain logs: {needle}"


@then(
    parsers.parse(
        "the pages hold {a:d}, {b:d} and {c:d} listings with no repeated id, no draft and every page total {total:d}"
    )
)
def pages_cover(mlf: OicWorld, a: int, b: int, c: int, total: int) -> None:
    pages = mlf.data["pages"]
    assert [len(p.get("listings") or []) for p in pages] == [a, b, c], [
        len(p.get("listings") or []) for p in pages
    ]
    ids = [i for p in pages for i in _ids(p.get("listings") or [])]
    assert len(ids) == len(set(ids)), "an id repeats across pages"
    assert not set(ids) & set(mlf.data["ids"]["draft"]), "a draft listing leaked into the pages"
    assert sorted(ids) == sorted(mlf.data["ids"]["published"])
    for p in pages:
        assert int((p.get("page") or {}).get("total", 0) or 0) == total, p.get("page")


@then(parsers.parse("the response holds {n:d} listings and a non-empty next cursor"))
def holds_n_with_cursor(mlf: OicWorld, n: int) -> None:
    listings, _, cursor = _page(mlf)
    assert len(listings) == n and cursor, (len(listings), cursor)


@then(parsers.parse('every response holds only published listings whose title contains "{word}"'))
def all_match(mlf: OicWorld, word: str) -> None:
    for resp in mlf.data["calls"]:
        listings, total, _ = _page(mlf, resp)
        assert listings and total == 1000, (len(listings), total)
        for item in listings:
            assert item.get("status") == "LISTING_STATUS_PUBLISHED", item
            assert word.lower() in item["title"].lower(), item


@then(parsers.parse("the 95th percentile of the call durations is at most {ms:d} ms"))
def p95(mlf: OicWorld, ms: int) -> None:
    d = sorted(mlf.data["durations"])
    value = d[max(0, int(len(d) * 0.95 + 0.999999) - 1)]
    assert value <= ms, f"p95 {value:.1f} ms > {ms} ms"


@then("the response holds only own published listings with id greater than the cursor id")
def foreign_cursor(mlf: OicWorld) -> None:
    listings, _, _ = _page(mlf)
    published = mlf.data["ids"]["published"]
    want = sorted(i for i in published if i > mlf.data["cursor_id"])
    assert listings, "expected the seller's published rows after the cursor"
    assert sorted(_ids(listings)) == want, (_ids(listings), want)
    assert all(i["status"] == "LISTING_STATUS_PUBLISHED" for i in listings), listings
